# Tasks — Service Order Listing and Detail

Each task references the `design.md` section it implements.

## 1. Spec traceability check

- [x] `requirements.md` and `design.md` written and internally consistent; the one genuine
      ambiguity (route prefix, `requirements.md` §7) resolved with the requester before
      writing any code.

## 2. Domain layer (`design.md` §1.6)

- [x] Add `ServiceOrderHistory` struct to `model.go`.
- [x] Add the package-level list of the six known status strings (for filter validation),
      kept separate from the `StatusRecebida`/`StatusEmDiagnostico`/
      `StatusAguardandoAprovacao` transition constants.
- [x] Extend `customerRef` with `Document`/`Phone`; extend `vehicleRef` with
      `Brand`/`Model`/`Year`/`Color` (`repository.go`).

## 3. Persistence (`design.md` §1.7)

- [x] Extend `findCustomerByID`/`findCustomerByDocument`/`scanCustomerRef` to select the two
      new `customerRef` columns.
- [x] Extend `findVehicleByID`/`findVehicleByPlate`/`scanVehicleRef` to select the four new
      `vehicleRef` columns.
- [x] Add `findServiceOrderByCode` to `serviceOrderLookups` + `PostgresServiceOrderRepository`.
- [x] Add `findRequestedServices` (join `service_order_requested_services`/`services`).
- [x] Add `findHistoryByServiceOrderID` (`service_order_history`, ordered oldest first).
- [x] Add `listServiceOrders` (filtered, paginated, joined `service_orders`/`customers`/
      `vehicles` query with a window `COUNT(*) OVER()` for `total`, same pattern
      `product.PostgresProductRepository.List` already uses).

## 4. Application layer (`design.md` §1.8)

- [x] `ListFilter`, `ServiceOrderListItem`, `ServiceOrderDetail` types (`query_service.go`).
- [x] `ServiceOrderService.List`.
- [x] `ServiceOrderService.GetDetail` / `GetDetailByCode`, sharing `buildDetail`; quote
      resolved to `nil` on `ErrQuoteNotFound`, any other repository error still propagated.

## 5. API layer (`design.md` §1.9)

- [x] `query_dto.go`: `listItemResponse`, `ListResponse`, `customerDetail`, `vehicleDetail`,
      `historyEntryResponse`, `DetailResponse`, and their `toListResponse`/`toDetailResponse`
      builders.
- [x] `handler.go`: `list` and `get` methods (`get` tries a UUID first, falling back to a
      sequential code — `design.md` §1.2/§1.9); query-string filter parsing/validation for
      `list` (`code`, `status`, `document`, `licensePlate`, `createdFrom`, `createdTo`);
      pagination helper (`defaultPage`/`defaultPageSize`/`maxPageSize`, `parseIntParam`,
      same per-feature duplication `customer`/`vehicle`/`product` already use — none of this
      package's existing files have it yet, since `service-order` had no listing before this
      feature).

## 6. Wiring (`design.md` §1.2)

- [x] Register `GET /api/v1/service-orders` and `GET /api/v1/service-orders/{id}` in
      `RegisterRoutes`, wrapped in `requireAuth`. A separate `GET .../code/{code}` route was
      attempted first and dropped after it failed the check below — `{id}` now accepts
      either a UUID or a code (`design.md` §1.2, `requirements.md` §7).
- [x] Verify the `ServeMux` registration actually succeeds (call `RegisterRoutes` against a
      real `*http.ServeMux`, no database needed) rather than assuming — `specs/architecture.md`
      decision 18's lesson. This is exactly what caught the conflict above.

## 7. Tests (`design.md` §3)

- [x] Extend `fake_repository_test.go` with the four new `serviceOrderLookups` methods.
- [x] `query_service_test.go`: filters (individually and combined), pagination, sort order,
      detail assembly (quote present/absent), not-found by id/code.
- [x] `internal/handlers_test/service_order_test.go`: pagination, every filter, combined
      filters, detail by id/code, `404` unknown id/code, `401` without a token on both
      routes, full-lifecycle detail (with quote + multi-entry history) vs. `RECEBIDA`-only
      detail (`quote` absent, single history entry).

## 8. Documentation (`design.md` §4)

- [x] `docs/openapi.yaml`: `service-orders` tag, two new paths, filter/pagination
      parameters, list/detail response schemas, `bearerAuth` requirement; update
      `info.description` to stop saying `ServiceOrder` isn't exposed via HTTP.

## 9. Verification

- [x] `go build ./...`, `go vet ./...`, `go test ./...` all pass.
- [x] Every `requirements.md` acceptance criterion (§6) re-checked against the implemented
      behavior.
- [x] No change outside this feature's scope (no route/behavior change to
      `POST /api/v1/service-orders`'s own auth status, no touching `product`/`servicecatalog`/
      `customer`/`vehicle` code).
