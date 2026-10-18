# Tasks — Service Order Quote Sending, Approval, and Rejection

Each task references the `design.md` section it implements. All tasks below are complete.

1. **Schema** (design.md §1.5, §2) — `docs/schema.sql`: add `CANCELED` to
   `service_order_status`; add `quote_sent` to `history_event`; add `version`/`sent_at`/
   `sent_version` + their `CHECK` constraints to `quotes`. `docs/entities.md`: document the
   new status, event, and `Quote` fields. `docs/seed.sql`: backfill `version`/`sent_at`/
   `sent_version` for existing seed quotes.
2. **Domain model** (design.md §1.3) — `model.go`: add `StatusCanceled`; delete
   `markAwaitingApproval`; add `sendQuote`/`approveQuote`/`rejectQuote`; add `Quote.Version`/
   `SentAt`/`SentVersion`.
3. **Notification port** (design.md §1.4) — new `quote_notifier.go`: `QuoteNotifier`
   interface, `NoOpQuoteNotifier`; wire into `ServiceOrderService` (`service.go`) and every
   call site of `NewServiceOrderService` (`cmd/api/main.go`, both test packages).
4. **Errors** (design.md §1.7) — `errors.go`: add `ErrTrackingTokenInvalid`.
5. **Repository interfaces** (design.md §1.5) — `repository.go`: add
   `findServiceOrderByCodeWithTrackingToken` to `serviceOrderLookups`; add `SendQuote`/
   `DecideQuote` to `ServiceOrderRepository`.
6. **Repository implementation** (design.md §1.5) — `repository.go`: implement
   `findServiceOrderByCodeWithTrackingToken`. `quote_repository.go`: update `SaveQuote` (no
   order transition, increment `version`, return new columns); implement `SendQuote`,
   `DecideQuote`; update `FindQuoteByServiceOrderID`'s scan for the new columns.
7. **Erratum applied to `ComposeQuote`** (design.md §1.2) — `quote_service.go`: replace the
   `markAwaitingApproval()` call with a direct `RECEIVED` check.
8. **Application layer** (design.md §1.6) — `quote_service.go`: add `SendQuote`,
   `ApproveQuote`, `RejectQuote`, `decideQuote`.
9. **DTOs** (design.md §1.7) — `quote_dto.go`: extend `QuoteResponse`; add
   `quoteDecisionResponse`/`toQuoteDecisionResponse`.
10. **HTTP layer** (design.md §1.7) — `handler.go`: `trackingTokenHeader` constant, three new
    routes/handlers. `httpsupport.go`: map `ErrTrackingTokenInvalid` to 401.
11. **Test doubles** (design.md §3) — `fake_repository_test.go`: implement `SendQuote`,
    `DecideQuote`, `findServiceOrderByCodeWithTrackingToken`; increment `Version` in the fake
    `SaveQuote`.
12. **Unit tests** (design.md §3) — rewrite `quote_model_test.go`'s
    `markAwaitingApproval` tests as `sendQuote`/`approveQuote`/`rejectQuote` tests; update
    `TestComposeQuoteSuccess`; add `SendQuote`/`ApproveQuote`/`RejectQuote` unit tests to
    `quote_service_test.go`.
13. **Integration tests** (design.md §3) — new
    `internal/handlers_test/service_order_quote_decision_test.go`; update the two existing
    assertions in `service_order_test.go` affected by the erratum (task 7).
14. **Spec sync** (design.md §2, this folder) — this `requirements.md`/`design.md`/`tasks.md`,
    plus an erratum note in `specs/service-order-diagnosis-quote/design.md` §1.6 and a
    resolved-gap note in `specs/service-order-execution/requirements.md` §2.1.
15. **Verification** — `go build ./...`, `go vet ./...`, and `go test ./...` pass, both
    without `DATABASE_URL` (integration tests self-skip) and against a freshly recreated local
    Postgres volume (`docker compose down -v && docker compose up -d`, reseeded) so the
    schema change is exercised for real.

## Fase 2 — external-system notification channel (`design.md` §5, added 2026-10-12)

- [ ] `quote_service.go`: refactor `decideQuote` to take an already-resolved `*ServiceOrder`
      instead of resolving it itself; update `ApproveQuote`/`RejectQuote` to resolve first,
      then call it (`design.md` §5.2).
- [ ] `quote_service.go`: add `DecideQuoteExternal(ctx, serviceOrderID, decision)`, resolving
      by id and calling the refactored `decideQuote` (`design.md` §5.2).
- [ ] Register the new route against a real `*http.ServeMux` (`RegisterRoutes` call, no
      database needed) and confirm no pattern-conflict panic — this project has twice found
      `ServeMux` route conflicts this way (`specs/architecture.md` decision 18,
      `specs/service-order-query/design.md` §1.2), cheap insurance before writing the
      handler for real.
- [ ] `handler.go`: register `POST /api/v1/service-orders/{id}/quote/decision`;
      `decideQuoteExternal` handler — constant-time secret check
      (`X-External-Notification-Secret`), decode/validate `{"decision": ...}`
      (`design.md` §5.1, §5.3).
- [ ] `httpsupport.go`: map missing/invalid secret to `401 INVALID_EXTERNAL_SECRET`
      (`design.md` §5.3).
- [ ] `cmd/api/main.go`: read `EXTERNAL_NOTIFICATION_SECRET` from env, `log.Fatalf` if unset,
      pass it into the service constructor (`design.md` §5.4).
- [ ] `.env.example`: add `EXTERNAL_NOTIFICATION_SECRET=` (`design.md` §5.4).
- [ ] Unit tests: `DecideQuoteExternal` success (approve/reject), repeated-decision conflict,
      unknown order id — reusing the existing fake repository (`design.md` §5.2).
- [ ] Integration tests: valid-secret approve/reject (DB status + history assertions),
      missing/wrong secret → `401` with zero side effects, invalid `decision` value → `400`,
      and a regression check that the two existing tracking-token endpoints are unaffected
      (AC20).
- [ ] `docs/openapi.yaml`: document the new endpoint, its header, and its two response
      statuses.
- [ ] Re-run `go build ./...`, `go vet ./...`, `go test ./...`; re-check AC15–AC20 in
      `requirements.md` §9.1.
