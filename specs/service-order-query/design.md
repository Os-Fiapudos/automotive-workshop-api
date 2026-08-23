# Design — Service Order Listing and Detail

## 1. Architecture decisions

### 1.1 Same package, not a new feature

This feature adds three read-only handlers to `internal/features/service-order/` (Go
package `serviceorder`), the same package `service-order-opening` and
`service-order-diagnosis-quote` already extend, rather than introducing a new
`internal/features/<name>/` package. It reads the exact same `ServiceOrder`/`Quote`
aggregate those two features write, uses the same administrative-JWT auth model
`service-order-diagnosis-quote` already applies to its own routes, and needs the same
package-private `customerRef`/`vehicleRef`/`serviceRef` projections and
`serviceOrderLookups` interface those features already declare — splitting it into its own
package would just re-declare all of that. This mirrors `service-order-diagnosis-quote`'s
own §1.1 rationale.

New files added to the package, following the existing `quote_*.go` split-by-use-case-group
convention (`dto.go`/`service.go`/`repository.go` for opening, `quote_dto.go`/
`quote_service.go`/`quote_repository.go` for diagnosis/quote):
- `query_dto.go` — request/response shapes for listing and detail.
- `query_service.go` — `ListFilter`, `ServiceOrderListItem`, `ServiceOrderDetail`, and the
  `ServiceOrderService.List`/`GetDetail`/`GetDetailByCode` methods.
- `query_repository.go` — the Postgres queries backing them.
- `handler.go` gets three new methods (`list`, `get`, `getByCode`) and three new
  registrations in `RegisterRoutes` — handler methods for every use case in this package
  already live in the single `handler.go`, not one file per use case (diagnosis/quote's
  handlers are there too), so this feature follows the same layout rather than adding a
  `query_handler.go`.

### 1.2 Route prefix and paths

Confirmed with the requester (`requirements.md` §7): reuse the existing
`/api/v1/service-orders` prefix instead of the ticket's suggested `/api/v1/ordens-servico`.

```
GET /api/v1/service-orders        → list (filters + pagination as query params)
GET /api/v1/service-orders/{id}   → detail — {id} accepts either the order's UUID or its
                                     sequential code (see below for why this is one route)
```

The ticket suggested two separate detail routes, `.../{id}` and `.../codigo/{codigo}`. A
literal `GET /api/v1/service-orders/code/{code}` was implemented first and then dropped
after actually registering it on a real `*http.ServeMux` (no database needed for that check)
panicked:

```
pattern "GET /api/v1/service-orders/code/{code}" conflicts with
pattern "GET /api/v1/service-orders/{id}/quote": both match "/api/v1/service-orders/code/quote",
but neither is more specific than the other.
```

This is not fixable by picking a different literal word: any `<literal>/{wildcard}` pattern
at that same depth conflicts with the pre-existing `{id}/quote` and `{id}/diagnosis`
patterns (both `{wildcard}/<literal>`), because the new pattern's wildcard slot can always
be filled with `"quote"`/`"diagnosis"` and the existing patterns' wildcard slot can always be
filled with whatever literal the new pattern chose — Go 1.22's `http.ServeMux` correctly
refuses to guess which one wins. This is a harder version of the conflict
`specs/architecture.md` decision 18 already documents for `vehicle`'s
`GET /api/v1/vehicles/plate/{plate}` vs. `GET /api/v1/vehicles/{id}`: that pair works only
because `vehicle` has no `{id}/<literal>` sibling route for it to collide with. `service-order`
does (`{id}/diagnosis`, `{id}/quote`), so the same trick isn't available here, at this depth,
under this prefix.

**Resolution**: one route, `GET /api/v1/service-orders/{id}`, whose handler tries
`uuid.Parse` first and, on failure, `strconv.ParseInt` — a UUID and a bare integer can never
be mistaken for each other, so this is unambiguous for the caller and requires no new
`ServeMux` pattern at all (`requirements.md` §7 records the discovery; `GetDetail`/
`GetDetailByCode`, §1.8 below, both still exist as separate service-layer methods — only the
HTTP-layer routing merged, not the application logic). Registering the final route set was
re-verified the same way (a real `ServeMux` + `RegisterRoutes` call) to confirm no conflict
remains — the lesson decision 18 already recorded: `go build`/`go vet`/`go test` do not catch
`ServeMux` pattern conflicts, only an actual registration call does.

Both routes are wrapped in `requireAuth`, like this package's existing `/diagnosis` and
`/quote` routes — `requirements.md` BR7 requires a valid JWT on every route this feature
adds; it does not touch `POST /api/v1/service-orders`'s own, separately-open, unauthenticated
status.

### 1.3 Filters

Query parameters on `GET /api/v1/service-orders`, all optional and combined with `AND`:

| Param | Maps to | Matching |
| --- | --- | --- |
| `code` | `service_orders.code` | exact (parsed as int64; a non-numeric value is a `400 VALIDATION_ERROR`, not silently ignored — it can never match a real order, but a client sending garbage should learn that, unlike `page`/`pageSize`, which have a sane default to fall back to) |
| `status` | `service_orders.status` | exact, validated against the six values `docs/entities.md`'s `ServiceOrderStatus` enum documents (`RECEBIDA`, `EM_DIAGNOSTICO`, `AGUARDANDO_APROVACAO`, `EM_EXECUCAO`, `FINALIZADA`, `ENTREGUE`) — not just the three this package currently has `Status` constants for (§1.6 below), since the other three are real, documented values a caller may reasonably filter for even though no feature transitions an order into them yet |
| `document` | `customers.document` | exact, normalized the same way `service-order-opening`'s own `resolveCustomer` already normalizes an inbound customer document (`internal/shared/document.Normalize`), so a formatted or unformatted CPF/CNPJ both work |
| `licensePlate` | `vehicles.license_plate` | exact, trimmed and upper-cased before matching (plates are stored normalized already by `vehicle`; this feature cannot reuse `vehicle`'s own `plate.Normalize` without importing another feature's package — `CLAUDE.md` §9.2 — so it applies this same minimal, dependency-free normalization instead) |
| `createdFrom` / `createdTo` | `service_orders.created_at` | inclusive range bound, each parsed as RFC3339 (`time.RFC3339`) — the same format Go's `encoding/json` already uses for every `time.Time` field this project returns, so a client filtering by a value it just read back from a previous response needs no reformatting; an unparseable value is a `400 VALIDATION_ERROR` |

No filter is required to have a name resembling the ticket's Portuguese suggestions
("Código da OS", "Placa", ...) — `CLAUDE.md` decision 8 already established that this
project's field/parameter names are in English even when the originating ticket is written
in Portuguese, and `customer`'s/`vehicle`'s own query parameters (`document`, `page`,
`pageSize`) already set that precedent for filters specifically.

`ListFilter` (service-layer input, `query_service.go`) carries these already-parsed/validated
values; structural parsing/validation (is `code` a number, is `status` one of the six known
values, is `createdFrom`/`createdTo` valid RFC3339) happens in the handler, the same split
`CreateRequest.Validate()`/`ComposeQuoteRequest.Validate()` already use in this package —
business-rule validation that needs repository access (there is none here; every filter
either matches or doesn't) stays in the service layer only to keep the split consistent, not
because there is a business rule to check.

### 1.4 Sort order and pagination

`ORDER BY service_orders.created_at DESC, service_orders.code DESC` — "most recent first"
(BR3) is `created_at`, and `code DESC` is a deterministic tiebreaker for orders created in
the same instant (matters once pagination is involved: an unstable tiebreak can duplicate or
skip a row across pages). `page`/`pageSize` reuse the exact convention already duplicated
per-feature in `customer`, `vehicle`, and `product` (`defaultPage = 1`, `defaultPageSize =
20`, `maxPageSize = 100`, an out-of-range value is clamped rather than rejected) — this
project has no shared pagination helper (`specs/architecture.md` §3 notes each feature
duplicates its own `parseIntParam`), so this feature duplicates it too rather than
introducing a `internal/shared/` package a single new feature doesn't justify on its own
(`CLAUDE.md` §9.3).

### 1.5 Response envelope

The list envelope is `{"data": [...], "page", "pageSize", "total", "totalPages"}` — the
shape `customer`, `vehicle`, and `product` (the project's three other *paginated* listings)
already use, not the catalog's `{"items": [...]}` shape (`CLAUDE.md` §8 flags this
envelope split as still unresolved). The catalog's listing is unpaginated by design (its own
`design.md` scoped pagination out); every listing that *is* paginated in this codebase
already agrees on `data`/`page`/`pageSize`/`total`/`totalPages`, so this feature follows the
majority, actually-paginated precedent instead of picking a third shape.

### 1.6 Domain/model additions

`model.go` gains:
- `ServiceOrderHistory`, mirroring `docs/entities.md`'s `ServiceOrderHistory` entity
  (`ID`, `ServiceOrderID`, `OccurredAt`, `Event`, `Description`, `PreviousStatus`,
  `NewStatus`) — read-only, this feature never writes a history row, only reads the ones
  `service-order-opening`/`service-order-diagnosis-quote` already write.
- A package-level list of the six known `Status` string values (for filter validation, §1.3)
  — kept separate from the existing `StatusRecebida`/`StatusEmDiagnostico`/
  `StatusAguardandoAprovacao` constants, which specifically mean "a status this package's own
  transition methods (`startDiagnosis`, `markAwaitingApproval`) know how to reach or leave,"
  not "every status value that exists." Adding `StatusEmExecucao`/`StatusFinalizada`/
  `StatusEntregue` constants without also giving them a transition method would be
  half-finished, unused code — this feature only needs the *strings* to validate a filter
  against, so that's all it adds.

`customerRef`/`vehicleRef` (declared in `repository.go`, already used by every use case in
this package) gain extra fields the detail view needs and the create flow never asked for:
`Document`/`Phone` on `customerRef`, `Brand`/`Model`/`Year`/`Color` on `vehicleRef`. The
existing `findCustomerByID`/`findCustomerByDocument`/`findVehicleByID`/`findVehicleByPlate`
queries are extended to select these extra columns too, rather than adding parallel
"detail" variants of the same four queries — the extra columns come from the same row at
no meaningful cost, and every existing caller of these methods (the create flow) simply
leaves the new fields unused, same pattern `serviceRef`'s `Description`/`Price` already
follow (populated by `findServiceByID`, left zero by `findServicesByIDs`).

### 1.7 Repository interface additions

Added to `serviceOrderLookups` (`repository.go`), the read-only boundary this package
already declares:

```go
findServiceOrderByCode(ctx context.Context, code int64) (*ServiceOrder, error)
findRequestedServices(ctx context.Context, serviceOrderID uuid.UUID) ([]*serviceRef, error)
findHistoryByServiceOrderID(ctx context.Context, serviceOrderID uuid.UUID) ([]*ServiceOrderHistory, error)
listServiceOrders(ctx context.Context, filter ListFilter, page, pageSize int) ([]*ServiceOrderListItem, int, error)
```

`findRequestedServices` replaces the create flow's two-step "load ids from
`order.RequestedServiceIDs`, then `findServicesByIDs`" (that field is only ever populated by
`NewServiceOrder` at construction time — an order loaded back from the database never has
it) with a single join query against `service_order_requested_services`. `ServiceOrderDetail`
(service layer) uses this for both `GetDetail`/`GetDetailByCode`.

`ServiceOrderRepository`'s own write-only surface (`Create`, `StartDiagnosis`, `SaveQuote`)
is untouched — every method this feature adds is a read, so it belongs on
`serviceOrderLookups`, the same split `service-order-diagnosis-quote`'s own read methods
(`findServiceOrderByID`, `findActiveProductByID`, `findServiceByID`) already established.

### 1.8 Service layer

```go
type ListFilter struct {
    Code             *int64
    Status           *string
    CustomerDocument string     // already normalized by the handler
    LicensePlate     string     // already normalized by the handler
    CreatedFrom      *time.Time
    CreatedTo        *time.Time
}

type ServiceOrderListItem struct {
    Order    *ServiceOrder
    Customer *customerRef
    Vehicle  *vehicleRef
}

type ServiceOrderDetail struct {
    Order             *ServiceOrder
    Customer          *customerRef
    Vehicle           *vehicleRef
    RequestedServices []*serviceRef
    Quote             *Quote // nil when no quote has been composed yet
    History           []*ServiceOrderHistory
}

func (service *ServiceOrderService) List(ctx context.Context, filter ListFilter, page, pageSize int) ([]*ServiceOrderListItem, int, error)
func (service *ServiceOrderService) GetDetail(ctx context.Context, id uuid.UUID) (*ServiceOrderDetail, error)
func (service *ServiceOrderService) GetDetailByCode(ctx context.Context, code int64) (*ServiceOrderDetail, error)
```

`GetDetail`/`GetDetailByCode` share one unexported `buildDetail(ctx, order *ServiceOrder)
(*ServiceOrderDetail, error)` once the order itself is resolved (by id or by code) — it loads
customer, vehicle, requested services, and history unconditionally, and the quote
conditionally: `FindQuoteByServiceOrderID` returning `ErrQuoteNotFound` is translated to
`Quote: nil`, not propagated as an error (`requirements.md` AC8 — the quote is `null`/absent,
not a `404`, for an order that hasn't reached `AGUARDANDO_APROVACAO` yet).

### 1.9 API layer

`query_dto.go` response shapes:

```go
type listItemResponse struct {
    ID        string          `json:"id"`
    Code      int64           `json:"code"`
    Customer  customerSummary `json:"customer"`  // reused from dto.go
    Vehicle   vehicleSummary  `json:"vehicle"`    // reused from dto.go
    Status    string          `json:"status"`
    OpenedAt  time.Time       `json:"openedAt"`
    CreatedAt time.Time       `json:"createdAt"`
    UpdatedAt time.Time       `json:"updatedAt"`
}

type ListResponse struct {
    Data       []listItemResponse `json:"data"`
    Page       int                `json:"page"`
    PageSize   int                `json:"pageSize"`
    Total      int                `json:"total"`
    TotalPages int                `json:"totalPages"`
}

type customerDetail struct {
    ID       string `json:"id"`
    Code     int64  `json:"code"`
    Name     string `json:"name"`
    Document string `json:"document"`
    Phone    string `json:"phone"`
}

type vehicleDetail struct {
    ID           string `json:"id"`
    Code         int64  `json:"code"`
    LicensePlate string `json:"licensePlate"`
    Brand        string `json:"brand"`
    Model        string `json:"model"`
    Year         int    `json:"year"`
    Color        string `json:"color"`
}

type historyEntryResponse struct {
    ID             string    `json:"id"`
    OccurredAt     time.Time `json:"occurredAt"`
    Event          string    `json:"event"`
    Description    string    `json:"description"`
    PreviousStatus string    `json:"previousStatus"`
    NewStatus      string    `json:"newStatus"`
}

type DetailResponse struct {
    ID                string                 `json:"id"`
    Code              int64                  `json:"code"`
    Customer          customerDetail         `json:"customer"`
    Vehicle           vehicleDetail          `json:"vehicle"`
    Status            string                 `json:"status"`
    Notes             string                 `json:"notes"`
    RequestedServices []serviceSummary       `json:"requestedServices"` // reused from dto.go
    Quote             *QuoteResponse         `json:"quote,omitempty"`   // reused from quote_dto.go
    History           []historyEntryResponse `json:"history"`
    OpenedAt          time.Time              `json:"openedAt"`
    CreatedAt         time.Time              `json:"createdAt"`
    UpdatedAt         time.Time              `json:"updatedAt"`
}
```

Handlers (`handler.go`):
- `list`: parses `page`/`pageSize` (existing per-feature helper, §1.4) and the five filters
  (§1.3) from the query string, collecting `apierror.Detail`s for any malformed one and
  responding `apierror.Validation` if any exist (mirrors `CreateRequest.Validate()`'s
  handler-side call), otherwise calls `Service.List` and writes `toListResponse(...)`.
- `get`: `r.PathValue("id")` is tried as `uuid.Parse` first (→ `Service.GetDetail`), and, if
  that fails, as `strconv.ParseInt(..., 10, 64)` (→ `Service.GetDetailByCode`) — the merged
  route from §1.2. A value that is neither is treated as `404 NOT_FOUND` (`"service order not
  found"`), the same choice `create`/`startDiagnosis`/`composeQuote`/`getQuote` already make
  in this handler for a malformed id — it can never match a real order, same observable
  outcome as a well-formed but unknown one (this is the exact reasoning `parseUUIDs`'s own
  doc comment already states for the create flow, reapplied here rather than reusing
  `product`'s different `400`-on-malformed-UUID convention, since this feature extends
  `service-order`, not `product`).
- Both handlers call `writeServiceError` on a service-layer error — `ErrServiceOrderNotFound`
  already maps to `404 NOT_FOUND` there (no change needed to that switch).

## 2. Domain model additions

### 2.1 `docs/entities.md`

No entity/field changes — `ServiceOrder`, `Quote`, `QuoteItem`, `ServiceOrderHistory` are
already fully documented there (`service-order-diagnosis-quote` wrote them in). This feature
only reads what already exists.

### 2.2 `docs/schema.sql` / `docs/seed.sql`

No schema changes — every table this feature reads (`service_orders`, `customers`,
`vehicles`, `service_order_requested_services`, `services`, `quotes`, `quote_products`,
`quote_services`, `service_order_history`) already exists. No seed changes either.

## 3. Testing strategy

Same split as the rest of this package (`specs/architecture.md` §9): stdlib `testing` only,
hand-written fakes, no mocking library.

- **Unit** (`internal/features/service-order/query_service_test.go`): extends
  `fake_repository_test.go`'s `fakeRepository` with the four new `serviceOrderLookups`
  methods (§1.7), backed by the same in-memory slices/maps it already has. Covers: listing
  with each filter individually and combined, pagination math, sort order, detail assembly
  (customer/vehicle/requested services/history always populated, quote `nil` before
  composition and populated after), and not-found for an unknown id/code.
- **Integration** (`internal/handlers_test/service_order_test.go`, extending the existing
  `testServiceOrderServer` helper): drives the three real HTTP routes against Postgres —
  pagination across pages, every filter (including combined filters and the date range),
  detail by id and by code returning the same shape, `404` for an unknown id/code, `401`
  without a bearer token on all three routes, and a full lifecycle case (create → start
  diagnosis → compose quote) whose detail view is checked to show the composed quote and a
  multi-entry history, followed by a case that never leaves `RECEBIDA` whose detail is
  checked to show `quote: null`/absent and a single `creation` history entry.

## 4. Documentation

`docs/openapi.yaml` gains a `service-orders` tag and two new paths
(`/api/v1/service-orders`, `/api/v1/service-orders/{id}` — the latter documented as
accepting either a UUID or a sequential code, per §1.2), documenting every filter query
parameter, the pagination parameters, the `bearerAuth` requirement, and the list/detail
response schemas —
`requirements.md` AC11. The existing `/api/v1/customers`/`/api/v1/vehicles` paths and the
document's `info.description` (which currently says `ServiceOrder`/`Quote` are "documented in
docs/entities.md but not yet implemented/exposed via HTTP") are updated to stop claiming
service orders aren't exposed via HTTP, without otherwise touching content out of this
feature's scope (auth, service catalog, product remain undocumented there, unchanged).

## 5. Traceability

| Requirement | Design section |
| --- | --- |
| BR1 (pagination) | §1.4 |
| BR2 (filters) | §1.3 |
| BR3 (sort order) | §1.4 |
| BR4 (detail contents) | §1.6, §1.8, §1.9 |
| BR5 (values from quote snapshot) | §1.6 (no re-join to catalog pricing anywhere in this feature) |
| BR6 (inactive related records still shown) | §1.6, §1.8 (no `status`/`active` filter on any detail read) |
| BR7 (JWT on every route) | §1.2 |
| BR8 / AC9 (404 on unknown id/code) | §1.9 |
| AC1–AC7 (listing + filters + pagination) | §1.3, §1.4, §1.5 |
| AC8 (detail contents) | §1.8, §1.9 |
| AC10 (401 without token) | §1.2 |
| AC11 (OpenAPI) | §4 |
