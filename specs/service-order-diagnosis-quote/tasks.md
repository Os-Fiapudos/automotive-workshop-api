# Tasks — Service Order Diagnosis and Quote Composition

Each task references the `design.md` section it implements. Implement in order — later
tasks depend on earlier ones.

## 1. Schema and docs (`design.md` §2)

- [ ] `docs/schema.sql`: add `applied_description`/`applied_total_price` to
      `quote_products`; add `applied_description`/`quantity`/`applied_total_price` to
      `quote_services`; add `diagnosis_started`/`quote_composed` to the `history_event`
      enum literal. Update `COMMENT ON COLUMN` for every changed/added column.
- [ ] `docs/entities.md`: `Quote` item shape (description/total per item),
      `ServiceOrderHistory.event` enum values.
- [ ] `docs/seed.sql`: fill in the new columns for existing `quote_products`/
      `quote_services` rows (orders 1, 2, 3, 5).

## 2. Spec traceability check

- [ ] Re-read `requirements.md` and `design.md` once schema changes are drafted; confirm
      no acceptance-criteria item is left unaddressed.

## 3. Domain layer (`design.md` §1.2, §1.3)

- [ ] `model.go`: add `StatusEmDiagnostico`/`StatusAguardandoAprovacao`,
      `startDiagnosis()`/`markAwaitingApproval()` methods, `QuoteItemKind`, `QuoteItem`,
      `Quote`, `QuoteItemInput`, `validateQuoteItems`, `calculateTotal`.
- [ ] `quote_model_test.go`: transition rules, item validation, total calculation
      (including a many-items case proving no float drift).

## 4. Errors (`design.md` §1.4)

- [ ] `errors.go`: add `ErrServiceOrderNotFound`, `ErrInvalidStatusTransition`,
      `ErrDiagnosisNotStarted`, `ErrQuoteAlreadyDecided`, `ErrQuoteNotFound`,
      `ErrEmptyQuote`, `ErrInvalidQuantity`, `ErrProductNotFound`, `ErrProductInactive`.
      (`ErrServiceNotFound` already exists as `ErrRequestedServiceNotFound` — decide
      whether to reuse it or add a quote-scoped alias; prefer reuse, same underlying
      "service id not in catalog" condition.)

## 5. Persistence (`design.md` §1.5, §1.6, §2.1)

- [ ] `quote_repository.go`: extend `serviceOrderLookups` (`findServiceOrderByID`,
      `findActiveProductByID`) and `ServiceOrderRepository` (`StartDiagnosis`,
      `SaveQuote`, `FindQuoteByServiceOrderID`) on `PostgresServiceOrderRepository`,
      following the existing transactional `Create`'s shape.

## 6. Application layer (`design.md` §1.4)

- [ ] `quote_service.go`: `StartDiagnosis`, `ComposeQuote`, `GetQuote` on
      `ServiceOrderService`.
- [ ] `quote_service_test.go`: extend `fake_repository_test.go` with the new fake methods
      (including a way to seed a decided quote for the "already decided" test) and cover
      every case in `design.md` §3's unit-test bullet.

## 7. API layer (`design.md` §1.7)

- [ ] `quote_dto.go`: `ComposeQuoteRequest`/item DTO (+ `Validate()`), `QuoteResponse` (+
      item summaries), `toQuoteResponse`.
- [ ] `httpsupport.go`: extend `writeServiceError` with the mapping table in `design.md`
      §1.7.
- [ ] `handler.go`: `startDiagnosis`, `composeQuote`, `getQuote` handlers; update
      `RegisterRoutes` to accept `requireAuth` and wrap the three new routes (existing
      `POST /api/v1/service-orders` stays unwrapped).

## 8. Wiring (`design.md` §1.7)

- [ ] `cmd/api/main.go`: change the `serviceorder.RegisterRoutes(...)` call to pass
      `requireAuth` as the third argument.

## 9. Integration tests (`design.md` §3)

- [ ] `internal/handlers_test/`: full flow, error cases from `design.md` §1.7's table,
      snapshot-immutability-after-catalog-change, stock-untouched, history rows,
      server-side total enforcement.

## 10. Verification

- [ ] `go build ./...`, `go vet ./...`, `go test ./...` pass locally.
- [ ] `docker compose down -v && docker compose up -d`, reseed, manual `curl` against the
      three new endpoints for a success case and each rejection case using
      `docs/seed.sql` fixtures (order 4, `EM_DIAGNOSTICO`, has no quote yet — good manual
      target for `PUT .../quote`; order 5, `RECEBIDA`, good target for
      `POST .../diagnosis` and for the "diagnosis not started" rejection on
      `PUT .../quote`).
- [ ] `DATABASE_URL=... go test ./...` (integration tests actually exercised, not
      skipped).
- [ ] Re-check every acceptance-criteria checkbox in `requirements.md` §6 against the
      implementation before calling the feature done.
