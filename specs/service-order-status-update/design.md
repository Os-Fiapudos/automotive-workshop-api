# Design — Service Order Status Update via External Tool

Satisfies: `requirements.md` (all sections), with §7's decisions applied.

`specs/service-order-architecture-refactor/` has not landed, so this is implemented against
the current flat package (`internal/features/service-order/`), next to the existing
`StartDiagnosis`/`FinalizeOrder`/`DeliverOrder` methods.

## 1. Use case — dispatch (`status_update_service.go`)

```go
func (service *ServiceOrderService) UpdateStatusExternal(ctx context.Context, id uuid.UUID, targetStatus Status, notes string) (*StatusUpdateResult, error)
```

- `notes` is trimmed; more than 500 characters (runes) → `ErrInvalidStatusUpdateNotes`.
- Dispatch:

  | `targetStatus` | delegates to |
  | --- | --- |
  | `IN_DIAGNOSIS` | `startDiagnosis(ctx, id, notes)` |
  | `COMPLETED` | `finalizeOrder(ctx, id, notes)` |
  | `DELIVERED` | `deliverOrder(ctx, id, notes)` |
  | anything else | `ErrUnsupportedExternalStatus` |

- The public `StartDiagnosis`/`FinalizeOrder`/`DeliverOrder` become thin wrappers that call
  the same unexported core with `notes = ""`. There is still exactly one implementation of
  each transition (lookup, status guard, `ensureRequiredExecutionsComplete`, transactional
  write) — BR1/BR3 hold by construction.
- Each unexported core returns `(order, previousStatus, error)`; `StatusUpdateResult`
  carries the updated order plus that `PreviousStatus` (captured from the loaded order before
  the transition), for the reduced response (BR8).

No new validation of transition legality and no new persistence logic is added here.

## 2. Persistence — notes in the history entry

The repository methods `StartDiagnosis`, `FinalizeOrder` and `DeliverOrder` gain a
`notes string` parameter. Each still inserts its history row in the same transaction as the
status `UPDATE`; the `description` becomes `historyDescription(<fixed text>, notes)`:

```go
func historyDescription(base, notes string) string {
    if notes == "" {
        return base
    }
    return base + " Notes: " + notes
}
```

No schema change: `service_order_history.description` is already `TEXT NOT NULL`. The
administrative endpoints always pass `""`, so their descriptions are byte-for-byte unchanged
(AC10). The value is bound as a query parameter (no SQL concatenation).

## 3. Authentication

Same shape as `specs/service-order-quote-decision/design.md` §5.1/§5.3/§5.4, with its own,
separate secret:

- Header: `X-External-Status-Secret`.
- `ServiceOrderService.WithExternalStatusUpdateSecret(secret)` +
  `AuthenticateExternalStatusUpdate(presented) bool` — both values SHA-256-hashed, then
  `crypto/subtle.ConstantTimeCompare` (shared `secretMatches` helper with the
  quote-decision check). An empty configured secret never authenticates anyone.
- `cmd/api/main.go` reads `EXTERNAL_STATUS_UPDATE_SECRET` and `log.Fatalf`s if it is unset
  **or equal to `EXTERNAL_NOTIFICATION_SECRET`** (BR4).
- The route is registered **without** `requireAuth` (no JWT needed); the handler checks the
  secret first.

## 4. API layer (`handler.go`, `status_update_dto.go`)

```go
mux.HandleFunc("POST /api/v1/service-orders/{id}/status", handler.updateStatusExternal)
```

Handler order (each step stops on failure, none touches the order before step 1 passes):

1. Secret check → `401 INVALID_EXTERNAL_SECRET`.
2. `{id}` parse → `404` for a non-UUID (same as the other `{id}` routes).
3. Decode `ExternalStatusUpdateRequest{Status string; Notes *string}` → `400` on bad JSON.
4. `Validate()` → `400 VALIDATION_ERROR` with `details` for a status outside the contract or
   notes over 500 characters (structural checks — no order lookup yet).
5. `UpdateStatusExternal`; errors through the existing `writeServiceError` mapping
   (`ErrServiceOrderNotFound` → 404, `ErrInvalidStatusTransition` → 409
   `INVALID_STATUS_TRANSITION`, `ErrExecutionsNotCompleted` → 409
   `EXECUTIONS_NOT_COMPLETED`), plus two new entries: `ErrUnsupportedExternalStatus` and
   `ErrInvalidStatusUpdateNotes` → `400 VALIDATION_ERROR` (defense in depth — step 4 already
   catches them).

Response `200`: `{"id", "code", "previousStatus", "status"}` (`statusUpdateResponse`).

`ServeMux` conflict check: `{id}/status` is a new literal segment under
`/api/v1/service-orders/{id}/`; a handler unit test registers every route on a real mux
(`RegisterRoutes`) so a pattern conflict would panic in CI (`specs/architecture.md`
decision 18).

## 5. Integration tool — n8n (`tools/n8n/`)

- `docker-compose.yml` gains an `n8n` service (`n8nio/n8n:2.43.8`, pinned), port
  `${N8N_PORT:-5678}`, with:
  - `EXTERNAL_STATUS_UPDATE_SECRET` and `WORKSHOP_API_URL=http://api:8080` in its
    environment, and `N8N_BLOCK_ENV_ACCESS_IN_NODE=false` so the workflow reads them via
    `$env` — the secret lives only in `.env`, never in the versioned workflow;
  - `./tools/n8n/workflows` mounted read-only, imported and published on startup
    (`n8n import:workflow` + `n8n publish:workflow` — n8n 2.x deprecated
    `update:workflow --active`), so the setup is reproducible with no clicks in the editor.
- Workflow `service-order-status-update.json`:
  **Form Trigger** (`/form/service-order-status`; fields: order id, target status dropdown
  with the three supported values, optional notes) → **HTTP Request** (`POST
  {{$env.WORKSHOP_API_URL}}/api/v1/service-orders/{id}/status`, header
  `X-External-Status-Secret: {{$env.EXTERNAL_STATUS_UPDATE_SECRET}}`, JSON body; "never
  error" + full response so rejections are shown, not swallowed) → **Form ending** page
  showing the HTTP status and response body. The form endpoint itself is the entry point
  for another system too (it accepts a multipart POST of the same fields), so no separate
  webhook trigger was added.
- `tools/n8n/README.md` is the demo script (roteiro): setup, triggering each status,
  checking the resulting history via `GET /api/v1/service-orders/{id}`, and the 409/400/401
  rejections.

## 6. Configuration

`.env.example` gains `EXTERNAL_STATUS_UPDATE_SECRET=` (different default from the
quote-decision one) and `N8N_PORT`. The `api` compose service requires it
(`${EXTERNAL_STATUS_UPDATE_SECRET:?...}`); `make run` passes it through. In Kubernetes
(`specs/kubernetes-deployment/`) it is one more key in the API `Secret`.

## 7. Testing strategy

- **Unit** (`status_update_service_test.go`, fake repository): one success per supported
  status (status, previous status, history entry with/without notes); every unsupported
  status → `ErrUnsupportedExternalStatus`; illegal transition → `ErrInvalidStatusTransition`
  with no state/history change; pending executions → `ErrExecutionsNotCompleted`; notes
  over 500 characters rejected, notes trimmed; secret authentication (match, mismatch,
  unconfigured, quote secret rejected).
- **Handler unit** (`status_update_handler_test.go`; the real-mux registration test stays in `handler_test.go`): 401 before lookup (missing, wrong, quote-decision
  secret, JWT-only), 400 for unsupported status/long notes/bad JSON, 404, 409, 200 shape.
- **Integration** (`internal/handlers_test/service_order_status_update_test.go`, real
  Postgres): full lifecycle `RECEIVED → IN_DIAGNOSIS → … → COMPLETED → DELIVERED` through
  this endpoint, history entries with notes; 401/400/409 with zero side effects (status and
  history count unchanged); `EXECUTIONS_NOT_COMPLETED` parity with `/finalize`.

## 8. Traceability

| Requirement | Design section |
| --- | --- |
| BR1, BR3 (no new transition logic, dispatch only) | §1 |
| BR2 (supported statuses) | §1, §4 |
| BR4 (separate external secret, checked first) | §3, §4, §6 |
| BR5, BR6 (history entry + notes) | §1, §2 |
| BR7 (rejections) | §4 |
| BR8 (reduced response) | §4 |
| AC1–AC3, AC7 | §1, §2 |
| AC4, AC5, AC8, AC9 | §4 |
| AC6 | §3, §4 |
| AC10 | §1, §2 |
| AC11 | §5 |
