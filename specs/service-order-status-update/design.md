# Design — Service Order Status Update via External Tool

Satisfies: `requirements.md` (all sections). Assumes `requirements.md` §7's working scope
(`IN_DIAGNOSIS`/`COMPLETED`/`DELIVERED`) is confirmed; if a different status set is chosen
instead, only §1's dispatch table changes, nothing else in this document.

Assumes `specs/service-order-architecture-refactor/` has landed (ports/use cases under
`usecase/`, handlers under `adapter/httpapi/`) — if this feature is implemented before that
refactor, the same logic applies against the current flat package (`service.go`'s existing
`StartDiagnosis`/`FinalizeOrder`/`DeliverOrder` methods) instead; §1's dispatch table is
identical either way.

## 1. Dispatch table (`usecase/status_update.go` or, pre-refactor, `service.go`)

```go
func (s *ServiceOrderService) UpdateStatusExternal(ctx context.Context, id uuid.UUID, targetStatus string, notes string) (*domain.ServiceOrder, error) {
    switch targetStatus {
    case string(domain.StatusInDiagnosis):
        return s.StartDiagnosis(ctx, id)
    case string(domain.StatusCompleted):
        return s.FinalizeOrder(ctx, id)
    case string(domain.StatusDelivered):
        return s.DeliverOrder(ctx, id)
    default:
        return nil, ErrUnsupportedExternalStatus
    }
}
```

`notes`, if present, is passed through to the underlying use case only where that use case
already accepts a notes/description override for its history entry (none of the three do
today — each writes a fixed description, e.g. `"Diagnosis started."`); if the underlying
signatures stay fixed-description, `notes` is accepted on the request but not yet plumbed
through, with a one-line comment pointing here, rather than changing three existing use
cases' signatures as an unrequested side effect of this feature (`CLAUDE.md` §17).

This function performs **no new validation and no new persistence** — it is purely a
`string → existing use-case method` dispatch. Every precondition (status guard, transaction,
history write) is the one the dispatched method already has; `requirements.md` BR1/BR3 are
satisfied by construction (there is no second implementation to drift from the first).

## 2. Authentication

Same shape as `specs/service-order-quote-decision/design.md` §5.1/§5.3/§5.4, with its own,
separate secret:

- Header: `X-External-Status-Secret`.
- Config: `EXTERNAL_STATUS_UPDATE_SECRET` env var, required at startup (`log.Fatalf` if
  unset, same as `JWT_SECRET`/`EXTERNAL_NOTIFICATION_SECRET`), compared with
  `crypto/subtle.ConstantTimeCompare`.
- Kept as a **separate** secret from quote-decision's `EXTERNAL_NOTIFICATION_SECRET`
  (`requirements.md` BR4) — the two represent different external integrations (a
  quote-approval relay vs. a status-update tool) that may be different systems in practice
  and should be revocable independently; sharing one secret would couple their rotation.

## 3. API layer

```go
mux.Handle("POST /api/v1/service-orders/{id}/status", wrap(handler.updateStatusExternal))
```

Request: `{"status": string, "notes"?: string}`. Handler validates `status` is one of the
three supported values (`400 VALIDATION_ERROR` otherwise, before even checking the secret
vs. after — checking the secret *first* is still correct here, unlike leaking "order
exists" information: this is a structural body-shape check, not information about a specific
order, so no information-disclosure concern applies to ordering it after the secret check);
calls `UpdateStatusExternal`; maps `ErrUnsupportedExternalStatus` to `400
VALIDATION_ERROR`, every other error through the already-existing `writeServiceError`
mapping (`specs/service-order-execution/design.md`'s/`specs/service-order-opening/design.md`'s
existing table — `ErrInvalidStatusTransition` → 409, `ErrServiceOrderNotFound` → 404,
already mapped, nothing new to add there).

Response: the full `ServiceOrder` (same shape `GET /{id}` already returns), `200 OK` — an
administrative/internal response shape is fine here since the caller is a trusted internal
tool, not the customer (unlike the quote-decision external channel, which deliberately
returns the reduced public DTO because its decision still ultimately originates from the
customer).

## 4. Configuration

`.env.example` gains `EXTERNAL_STATUS_UPDATE_SECRET=`. `cmd/api/main.go` reads it and fails
fast if unset, same pattern as `EXTERNAL_NOTIFICATION_SECRET`
(`specs/service-order-quote-decision/design.md` §5.4). In Kubernetes
(`specs/kubernetes-deployment/`), a second key in the same `Secret` object that already
holds `EXTERNAL_NOTIFICATION_SECRET`/`JWT_SECRET`/`DATABASE_URL`.

## 5. Testing strategy

- **Unit**: `UpdateStatusExternal` dispatch for each of the three supported statuses
  (delegates correctly, returns `ErrUnsupportedExternalStatus` for anything else, including
  `AWAITING_APPROVAL`/`IN_PROGRESS`/`CANCELED`/`RECEIVED`), using the same fakes
  `StartDiagnosis`/`FinalizeOrder`/`DeliverOrder`'s own existing unit tests already use.
- **Integration**: one success case per supported status (asserting the same DB state and
  history entry the existing dedicated endpoint's own integration test already asserts),
  missing/wrong secret → `401` with zero side effects, unsupported status → `400`,
  currently-illegal transition (e.g. `DELIVERED` requested on a still-`RECEIVED` order) →
  same status/code `POST /{id}/deliver` already returns for that case, and a regression
  check that the three existing endpoints are unaffected.

## 6. Traceability

| Requirement | Design section |
| --- | --- |
| BR1, BR3 (no new transition logic, dispatch only) | §1 |
| BR2 (status named explicitly) | §3 |
| BR4 (separate external secret) | §2, §4 |
| BR5 (existing history write reused) | §1 |
| BR6 (existing rejection semantics reused) | §1, §3 |
| AC1–AC3 | §1, §3 |
| AC4, AC5 | §3 |
| AC6 | §2 |
| AC7, AC8 | §1 |
