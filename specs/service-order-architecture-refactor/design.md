# Design — `service-order` Clean Architecture Refactor

Satisfies: `requirements.md` (all sections). Structural only — no acceptance criterion from
any other `service-order-*` spec changes meaning; this document maps existing files to a
new layout and defines the new port interfaces, nothing else.

## 1. Target package layout

```
internal/features/service-order/
├── doc.go                         → updated package comment describing the layering
├── domain/
│   ├── doc.go
│   ├── service_order.go           → ServiceOrder, Status, transition methods (was model.go)
│   ├── quote.go                   → Quote, QuoteItem, QuoteStatus (was part of model.go)
│   ├── history.go                 → ServiceOrderHistory (was part of model.go, query feature)
│   ├── execution.go               → Execution (was execution_model.go)
│   ├── stock_usage.go             → StockMovement-related domain types (was stockusage_model.go)
│   ├── errors.go                  → domain sentinel errors (ErrInvalidStatusTransition, ...)
│   └── *_test.go                  → pure domain unit tests (was model_test.go, quote_model_test.go, ...)
├── usecase/
│   ├── doc.go
│   ├── ports.go                   → every port interface this layer defines (§2)
│   ├── create.go                  → CreateService (was service.go's ServiceOrderService.Create)
│   ├── diagnosis.go               → StartDiagnosis
│   ├── quote_compose.go            → ComposeQuote, GetQuote (was quote_service.go)
│   ├── quote_decision.go           → SendQuote, ApproveQuote, RejectQuote, DecideQuoteExternal
│   ├── execution.go                → StartExecution, FinishExecution
│   ├── finalize.go                 → FinalizeOrder, DeliverOrder
│   ├── query.go                    → List, GetDetail, GetDetailByCode (was query_service.go)
│   ├── stock_usage.go              → RegisterStockUsage, ListStockMovements, ReverseStockMovement
│   ├── metrics.go                  → AverageExecutionTime (was metrics_service.go)
│   └── *_test.go                   → use-case unit tests against fakes implementing the ports
├── adapter/
│   ├── postgres/
│   │   ├── doc.go
│   │   ├── repository.go           → PostgresServiceOrderRepository + shared pool/tx helpers
│   │   ├── lookups.go               → customer/vehicle/product/service read models + queries
│   │   ├── quote_repository.go      → SaveQuote, SendQuote, DecideQuote
│   │   ├── query_repository.go      → listServiceOrders, detail/history queries
│   │   ├── execution_repository.go
│   │   ├── stockusage_repository.go
│   │   ├── metrics_repository.go
│   │   └── *_test.go (integration, if any live here rather than internal/handlers_test/)
│   └── httpapi/
│       ├── doc.go
│       ├── router.go                → RegisterRoutes(mux, deps) — the only exported entrypoint
│       ├── create_handler.go        → POST /service-orders
│       ├── diagnosis_handler.go      → POST /{id}/diagnosis
│       ├── quote_handler.go          → PUT/GET .../quote, POST .../quote/send
│       ├── quote_decision_handler.go → the three decision routes (tracking-token + external)
│       ├── execution_handler.go      → the two execution routes
│       ├── finalize_handler.go       → finalize, deliver
│       ├── query_handler.go          → list, get
│       ├── stockusage_handler.go     → the three stock-movement routes
│       ├── metrics_handler.go        → average-execution-time
│       ├── dto.go, quote_dto.go, query_dto.go, execution_dto.go, metrics_dto.go, stockusage_dto.go
│       ├── errors.go                 → writeServiceError: domain/use-case error → apierror mapping
│       └── httpsupport.go            → JSON decode/encode, trackingTokenHeader, parseIntParam
└── quote_notifier.go → stays at the usecase layer's port list (§2); NoOpQuoteNotifier moves
                         to adapter/ (it's an infrastructure stub, not a domain/use-case type)
```

Each of `domain`, `usecase`, `adapter/postgres`, `adapter/httpapi` is its own Go package
(`package domain`, `package usecase`, `package postgres`, `package httpapi`) under
`internal/features/service-order/`. `httpapi`, not `http`, avoids shadowing the standard
library package at every import site in this feature. `cmd/api/main.go` imports only
`usecase` (to construct the use-case structs) and `adapter/postgres`/`adapter/httpapi` (to
construct the repository and call `RegisterRoutes`) — it never imports `domain` directly,
same as today.

This is a rename-and-move of existing files, not a rewrite: every struct, method, and SQL
statement keeps its current logic; only its package (and, for `handler.go`, its file
grouping) changes. The one real code change is the port-interface split (§2) and removing
`apierror` imports from anything that moves into `domain`/`usecase` (§3).

## 2. Port interfaces (`usecase/ports.go`)

Replaces the single `serviceOrderLookups` (20 methods) with one interface per real
consumer, each only as large as its one use case needs:

```go
type ServiceOrderRepository interface {
    Create(ctx context.Context, order *domain.ServiceOrder) error
    StartDiagnosis(ctx context.Context, id uuid.UUID) (*domain.ServiceOrder, error)
    // ... every existing write method, unchanged — this interface was already use-case-
    // shaped (requirements.md of service-order-opening §26 reference), it is not split.
}

type CustomerLookup interface {
    FindActiveCustomerByID(ctx context.Context, id uuid.UUID) (*CustomerRef, error)
    FindActiveCustomerByDocument(ctx context.Context, doc string) (*CustomerRef, error)
}

type VehicleLookup interface {
    FindActiveVehicleByID(ctx context.Context, id uuid.UUID) (*VehicleRef, error)
    FindActiveVehicleByPlate(ctx context.Context, plate string) (*VehicleRef, error)
}

type ServiceCatalogLookup interface {
    FindServiceByID(ctx context.Context, id uuid.UUID) (*ServiceRef, error)
    FindServicesByIDs(ctx context.Context, ids []uuid.UUID) ([]*ServiceRef, error)
    FindMissingServiceIDs(ctx context.Context, ids []uuid.UUID) ([]uuid.UUID, error)
}

type ProductLookup interface {
    FindActiveProductByID(ctx context.Context, id uuid.UUID) (*ProductRef, error)
}

type HistoryLookup interface {
    FindHistoryByServiceOrderID(ctx context.Context, id uuid.UUID) ([]*domain.ServiceOrderHistory, error)
}

type TrackingLookup interface {
    FindServiceOrderByCodeWithTrackingToken(ctx context.Context, code int64, tokenHash string) (*domain.ServiceOrder, error)
}

type QuoteNotifier interface {
    NotifyQuoteSent(ctx context.Context, order *domain.ServiceOrder, quote *domain.Quote) error
}
```

Each use-case constructor takes only the ports it actually calls — e.g.
`NewCreateService(repo ServiceOrderRepository, customers CustomerLookup, vehicles
VehicleLookup, catalog ServiceCatalogLookup, products ProductLookup)`, not one 20-method
bag. `PostgresServiceOrderRepository` (`adapter/postgres`) implements every one of these
interfaces on the same concrete type (it already does, implicitly, today — Go's structural
typing means no change to the Postgres implementation itself, only to how each use case
declares what it needs). `cmd/api/main.go` still constructs one
`PostgresServiceOrderRepository` and passes the same value into every use-case constructor
that needs it, the same wiring shape as today, just typed through the narrower interfaces at
each call site.

Fakes in `usecase/*_test.go` shrink correspondingly — a test for `CreateService` only needs
to implement `ServiceOrderRepository`'s `Create` plus `CustomerLookup`/`VehicleLookup`/
`ServiceCatalogLookup`/`ProductLookup`, not all 20 methods the old `fakeRepository` carried
regardless of which use case was under test.

## 3. Removing HTTP concerns from domain/use-case code

- `domain/*.go`: no file imports `apierror`, `net/http`, `encoding/json`, or `pgx`. Domain
  errors (`ErrInvalidStatusTransition`, etc.) are plain `errors.New`/sentinel values, as they
  already are today in `errors.go` — this file moves into `domain/errors.go` unchanged.
- `usecase/*.go`: returns domain errors (the same sentinels) or wraps them; never
  constructs an `apierror.Error` itself. This is almost entirely a move, not a rewrite — the
  current `service.go`/`quote_service.go`/etc. already return sentinel errors, not
  `apierror` values; the one real change is `dto.go`'s current `Validate() []apierror.Detail`
  shape (`CreateRequest`, `ComposeQuoteRequest`, etc.), which moves into
  `adapter/httpapi/dto.go` and keeps `apierror.Detail` there, since structural request
  validation (is this field present, is this a well-formed UUID) is correctly an HTTP-layer
  concern, not a domain one — only business-rule validation (does this customer exist, is it
  active) belongs in `usecase/`.
- `adapter/httpapi/errors.go`: the single `writeServiceError(w, err)` switch, mapping every
  domain/use-case sentinel error to its HTTP status + `apierror` code — consolidates the
  mapping logic that today is split across `httpsupport.go` and ad hoc checks in `handler.go`
  into one place, satisfying `requirements.md` BR5.

## 4. Cross-feature read models stay, relocated

`CustomerRef`/`VehicleRef`/`ServiceRef`/`ProductRef` (today: `customerRef`/`vehicleRef`/
`serviceRef`/`productRef`, lower-case/package-private) move to `usecase/` as exported types
(the ports in §2 reference them, so they must be visible outside `adapter/postgres`), but
keep their exact fields and purpose: package-private-in-spirit read models populated by raw
SQL against other features' tables (`customers`, `vehicles`, `services`, `products`),
**not** imports of `customer.Customer`/`vehicle.Vehicle`/`servicecatalog.Service`/
`product.Product`. `CLAUDE.md` §9.2 ("no direct coupling between features") is unaffected —
this refactor does not change *how* `service-order` reads other features' data, only *which
package* the resulting Go types live in.

## 5. Staged migration order (keeps `go build`/`go vet`/`go test` green throughout)

1. Create `domain/` and move `model.go`, `execution_model.go`, `stockusage_model.go`,
   `errors.go`, and their `*_test.go` files into it verbatim (package rename only). Update
   every remaining file in the old flat package to `import ".../service-order/domain"` and
   qualify domain types (`domain.ServiceOrder`, `domain.Status`). Suite green.
2. Create `usecase/`, define the ports (§2), move `service.go`, `quote_service.go`,
   `query_service.go`, `execution_service.go`, `stockusage_service.go`, `metrics_service.go`
   and their `*_test.go` into it, updating each to depend on the new narrow ports instead of
   `serviceOrderLookups`. Fakes in `*_test.go` shrink accordingly (§2). Suite green.
3. Create `adapter/postgres/`, move `repository.go`, `quote_repository.go`,
   `query_repository.go`, `execution_repository.go`, `stockusage_repository.go`,
   `metrics_repository.go` into it; confirm `PostgresServiceOrderRepository` satisfies every
   port interface from step 2 (a compile-time `var _ usecase.ServiceOrderRepository =
   (*PostgresServiceOrderRepository)(nil)` assertion per port, in a new `assertions.go`).
   Suite green.
4. Create `adapter/httpapi/`, move `handler.go` (split into the per-use-case files in §1),
   every `*_dto.go`, and `httpsupport.go`; move `apierror` usage out of `usecase/` into
   `adapter/httpapi/errors.go` (§3). Suite green.
5. Update `cmd/api/main.go`'s wiring to the new import paths/constructors. Suite green.
6. Update `CLAUDE.md` §9.1 and `specs/architecture.md` (§6 below).

Each step is its own commit, matching `requirements.md` §6's "staged, by layer" decision —
a reviewer (or a future bisect) can see the domain extracted, then the use cases, then each
adapter, independently.

## 6. Documentation updates

- `CLAUDE.md` §9.1 gains a clause: "A feature that has explicitly adopted Clean Architecture
  or a Hexagonal layering (currently: `service-order` only, see
  `specs/service-order-architecture-refactor/`) may organize its internal files into
  `domain/`, `usecase/`, and `adapter/<name>/` sub-packages, as long as the whole feature
  still lives under one `internal/features/<name>/` folder and no cross-feature import is
  introduced. This is additive to, not a replacement for, the vertical-slice rule: the slice
  is still the feature folder; the sub-packages are internal to it."
- `specs/architecture.md` gains a description of this layering for `service-order`,
  distinguishing it from the flat layout every other implemented feature still uses, so a
  future reader does not treat the flat layout as broken or the layered one as the only
  correct shape.

## 7. Testing strategy

No new test *behavior* — every existing test moves with its code (domain tests with
`domain/`, use-case tests with `usecase/`, using the shrunk fakes from §2, integration tests
in `internal/handlers_test/` untouched since they only exercise the public HTTP surface,
which does not change). The one new check: a short `go vet`-adjacent task (`tasks.md`) that
greps `domain/*.go` and `usecase/*.go` for a forbidden import (`net/http`, `encoding/json`,
`internal/shared/apierror`, `github.com/jackc/pgx`) and fails if found — this is the
mechanical enforcement of `requirements.md` BR1/BR2/BR5, since the project has no layered-
architecture linter (`CLAUDE.md` §7).

## 8. Traceability

| Requirement | Design section |
| --- | --- |
| BR1 (domain has no infra imports) | §1, §3 |
| BR2 (use-case depends only on domain + ports) | §2, §3 |
| BR3 (adapters depend inward only) | §1 |
| BR4 (narrow, consumer-defined ports) | §2 |
| BR5 (HTTP concerns only in adapter/httpapi) | §3 |
| BR6 (still one vertical slice) | §6 |
| BR7 (no new cross-feature import) | §4 |
| BR8 (no behavior change) | §5 (staged, suite green at each step) |
