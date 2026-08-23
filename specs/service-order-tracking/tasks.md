# Service Order Tracking — Tasks

Ordered implementation checklist, each item traceable to a `design.md` section.

- [x] T1 — `docs/entities.md`: add `ServiceOrderTrackingToken` entity; note the new
      `trackingToken` field on `ServiceOrder`'s creation response. (design.md §3, §5)
- [x] T2 — `docs/schema.sql`: add `service_order_tracking_tokens` table + unique index on
      `token_hash`. (design.md §3)
- [x] T3 — `internal/shared/trackingtoken`: `Generate`/`Hash` + unit tests. (design.md §2)
- [x] T4 — `internal/shared/apierror`: add `Unauthorized(code, message string) *Error`.
      (design.md §8)
- [x] T5 — `internal/features/service-order`: change `ServiceOrderRepository.Create` to
      also issue and persist a tracking token in the same transaction; thread
      `CreateResult.TrackingToken` through to `Response.TrackingToken`; update
      `fakeRepository`/tests. (design.md §5)
- [x] T6 — `specs/service-order-opening/design.md`: cross-reference note for the new
      response field. (design.md §5)
- [x] T7 — New feature `internal/features/service-order-tracking` (package
      `servicetracking`): model, dto, errors, repository, service, handler, doc.go.
      (design.md §4, §6, §7, §8)
- [x] T8 — `cmd/api/main.go`: wire the repository/service and register
      `GET /api/v1/acompanhamento/{codigo}`, unwrapped by `requireAuth`. (design.md §8)
- [x] T9 — Unit tests for `servicetracking.TrackingService.Get` (fake repository): AC1–AC6.
      (design.md §10)
- [x] T10 — Integration test `internal/handlers_test/service_order_tracking_test.go`:
      AC1–AC5, AC7, AC8, AC10. (design.md §10)
- [x] T11 — `go build ./...`, `go vet ./...`, `go test ./...` all pass. (CLAUDE.md §6/§7)
- [x] T12 — Review against `requirements.md`'s acceptance criteria (§5) before calling the
      feature done.
