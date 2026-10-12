# Tasks — Service Order Status Update via External Tool

Each task references the `design.md` section it implements.

## 1. Spec confirmation

- [ ] Confirm `requirements.md` §7's status scope (`IN_DIAGNOSIS`/`COMPLETED`/`DELIVERED`)
      with whoever owns the Fase 2 requirement before starting implementation — `Status:
      Proposed` becomes `Approved for implementation` once confirmed.

## 2. Use-case layer (`design.md` §1)

- [ ] Add `ErrUnsupportedExternalStatus` to the feature's domain/use-case errors.
- [ ] Add `UpdateStatusExternal(ctx, id, targetStatus, notes)` dispatching to
      `StartDiagnosis`/`FinalizeOrder`/`DeliverOrder` (`design.md` §1) — no new persistence
      or validation logic.

## 3. API layer (`design.md` §2, §3)

- [ ] Register the new route against a real `*http.ServeMux` (`RegisterRoutes` call, no
      database needed) and confirm no pattern-conflict panic — the `{id}/status` segment is
      new and distinct from this package's other `{id}/<literal>` routes, but this project
      has twice found `ServeMux` conflicts only by actually registering routes
      (`specs/architecture.md` decision 18, `specs/service-order-query/design.md` §1.2), so
      the check is repeated here rather than assumed safe.
- [ ] `handler.go` (or `adapter/httpapi/status_update_handler.go` if
      `specs/service-order-architecture-refactor/` has already landed): register
      `POST /api/v1/service-orders/{id}/status`; constant-time secret check
      (`X-External-Status-Secret`); decode/validate `{"status", "notes"?}`; call
      `UpdateStatusExternal`; map `ErrUnsupportedExternalStatus` to `400
      VALIDATION_ERROR`.
- [ ] `cmd/api/main.go`: read `EXTERNAL_STATUS_UPDATE_SECRET`, `log.Fatalf` if unset, pass
      into the handler/service construction (`design.md` §4).
- [ ] `.env.example`: add `EXTERNAL_STATUS_UPDATE_SECRET=` (`design.md` §4).

## 4. Tests (`design.md` §5)

- [ ] Unit tests: dispatch for each supported status; `ErrUnsupportedExternalStatus` for
      every unsupported value.
- [ ] Integration tests: one success case per supported status (DB state + history entry
      parity with the existing dedicated endpoint), missing/wrong secret → `401`,
      unsupported status → `400`, illegal transition → same status/code as the existing
      dedicated endpoint, regression check on the three existing endpoints.

## 5. Documentation

- [ ] `docs/openapi.yaml`: document the new endpoint, its header, its three supported
      status values, and its response shape.

## 6. Verification

- [ ] `go build ./...`, `go vet ./...`, `go test ./...` pass.
- [ ] Re-check AC1–AC8 in `requirements.md` §6 against the implementation.
