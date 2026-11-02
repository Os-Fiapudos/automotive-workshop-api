# Tasks — Service Order Status Update via External Tool

Each task references the `design.md` section it implements.

## 1. Spec confirmation

- [x] Confirm `requirements.md` §7 (statuses, notes, response, tool) — resolved
      2026-10-19, `Status: Approved for implementation`.

## 2. Use-case layer (`design.md` §1, §2)

- [x] Add `ErrUnsupportedExternalStatus` and `ErrInvalidStatusUpdateNotes`.
- [x] Extract unexported `startDiagnosis`/`finalizeOrder`/`deliverOrder` cores taking
      `notes`; public methods delegate with `""`.
- [x] Add `UpdateStatusExternal(ctx, id, targetStatus, notes)` dispatch returning
      `StatusUpdateResult` (order + previous status).
- [x] Repository `StartDiagnosis`/`FinalizeOrder`/`DeliverOrder` take `notes`, persisted
      via `historyDescription` in the same transaction.

## 3. API layer (`design.md` §3, §4)

- [x] `WithExternalStatusUpdateSecret` / `AuthenticateExternalStatusUpdate`
      (shared `secretMatches`).
- [x] `POST /api/v1/service-orders/{id}/status` handler: secret → id → body → validate →
      dispatch; reduced response.
- [x] Map the two new errors to `400 VALIDATION_ERROR`.
- [x] Real-mux registration test (no pattern conflict).
- [x] `cmd/api/main.go`: read `EXTERNAL_STATUS_UPDATE_SECRET`, fail fast if unset or equal
      to `EXTERNAL_NOTIFICATION_SECRET`.

## 4. Tool (`design.md` §5, §6)

- [x] `tools/n8n/workflows/service-order-status-update.json` (form + webhook → HTTP).
- [x] `docker-compose.yml` `n8n` service with auto-import/activation; `api` requires the
      new secret.
- [x] `.env.example`, `Makefile` (`run`).
- [x] `tools/n8n/README.md` demo script.

## 5. Tests (`design.md` §7)

- [x] Unit: dispatch per status, unsupported statuses, illegal transition without side
      effects, notes, authentication.
- [x] Handler unit: 401/400/404/409/200.
- [x] Integration: lifecycle via the endpoint, history with notes, zero side effects on
      401/400/409.

## 6. Documentation

- [x] `docs/openapi.yaml`: endpoint, header, three status values, notes, reduced response,
      error responses.
- [x] README, `CLAUDE.md`, `specs/architecture.md` mentions of the new secret/route.

## 7. Verification

- [x] `go build ./...`, `go vet ./...`, `go test ./...` pass (with and without
      `DATABASE_URL`).
- [x] Re-check AC1–AC11 in `requirements.md` §6 against the implementation.
- [x] End-to-end run of the n8n workflow against the compose stack.
