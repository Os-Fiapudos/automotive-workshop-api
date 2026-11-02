# Requirements — Service Order Status Update via External Tool

Status: **Approved for implementation** (2026-10-19 — §7's open questions resolved, see
§7 for the decisions; planning id F2-US05 / FP-30)
Feature folder: `internal/features/service-order/` (extends the existing package/aggregate
— same reasoning `specs/service-order-query/requirements.md` §1 already gives for not
creating a new feature package).

## 1. Context

Source: `SOAT - Fase 2 - Tech challenge.pdf` p.3, "Atualização de status da OS via alguma
ferramenta como email." `GAPS-FASE-2.md` §3.1 records that today every status transition is
a dedicated, administrative-JWT-protected action
(`/diagnosis`, `/quote/send`, the quote-decision endpoints, `/executions`, `/finalize`,
`/deliver`) and no generic, externally-triggerable status-update endpoint exists.

This feature does **not** duplicate the quote-approval/rejection transition — that external
channel already exists (`specs/service-order-quote-decision/requirements.md` §9, added
2026-10-12: `POST /api/v1/service-orders/{id}/quote/decision`). This feature covers the
*other* transitions a workshop might plausibly want to trigger from an outside tool,
without re-implementing any transition rule the domain already enforces.

## 2. User story

> As a workshop operator, I want to trigger stage changes from an integrated tool,
> so that the order's tracking stays in sync with what is happening in the workshop,
> without an administrator manually calling the JWT-protected action endpoints.

## 3. Business rules

1. This feature adds **no new transition logic**. It is a second, externally-authenticated
   entry point into transitions the domain already implements and already enforces the
   preconditions for (`StartDiagnosis`, `FinalizeOrder`, `DeliverOrder` — see §7 for why
   only these three, not every transition).
2. The caller identifies the target status by name. Only `IN_DIAGNOSIS`, `COMPLETED` and
   `DELIVERED` are accepted (§7). `RECEIVED` is the order's fixed starting status, never a
   target; `AWAITING_APPROVAL`, `IN_PROGRESS` and `CANCELED` are reachable only through
   their own dedicated channels, so this endpoint can never be used to skip quote approval
   on the way into execution.
3. Every precondition the underlying transition already enforces (BR of the owning spec)
   still applies unchanged — e.g. `FinalizeOrder` still requires the order to be
   `IN_PROGRESS` with no required execution left unfinished, exactly as
   `specs/service-order-execution/requirements.md` already specifies; this feature does not
   relax, bypass, or duplicate that rule, it only reaches the same use case (same status
   guard, same transaction) through a different, externally-authenticated route.
4. Authentication is a shared secret the external tool presents (`X-External-Status-Secret`
   header, configured by `EXTERNAL_STATUS_UPDATE_SECRET`) — not the administrative JWT (the
   caller is a tool, not a logged-in administrator; no JWT is required in addition) and not
   the customer tracking token (the caller is not the customer). Same posture as
   `specs/service-order-quote-decision/requirements.md` §9 item 4, with its **own, separate**
   secret: the API refuses to start if it is unset or equal to the quote-decision
   `EXTERNAL_NOTIFICATION_SECRET`, so the two integrations can be rotated/revoked
   independently and a leak of one never grants the other. The secret is checked **before**
   the order is looked up, so a caller without it learns nothing about which orders exist.
5. Every transition performed through this endpoint still generates its
   `service_order_history` entry, through the same existing transactional write the
   underlying use case already performs. When the request carries `notes` (§5), they are
   persisted in that same history entry's `description`, after the fixed event text
   (`"Diagnosis started. Notes: <notes>"`), in the same transaction — never accepted and
   discarded.
6. `notes` is optional; when present it is trimmed, and must be at most 500 characters
   after trimming (400 otherwise). Blank `notes` is treated as absent.
7. Rejections, none of which change the order's status or history:
   - missing/incorrect secret → `401`;
   - `status` outside the contract (§7), or invalid `notes` → `400 VALIDATION_ERROR`;
   - unknown order → `404`;
   - transition not currently legal for the order's actual status → `409`, with the same
     code the corresponding existing endpoint already returns
     (`INVALID_STATUS_TRANSITION`, or `EXECUTIONS_NOT_COMPLETED` for a finalization with
     pending required executions).
8. The success response is reduced to what the tool needs to confirm the outcome: the
   order's `id`, `code`, `previousStatus` and new `status`.

## 4. Scope

In scope:
- One new endpoint, `POST /api/v1/service-orders/{id}/status`, dispatching to
  `StartDiagnosis`/`FinalizeOrder`/`DeliverOrder` based on the requested target status.
- Shared-secret authentication for this endpoint.
- Persisting `notes` in the resulting history entry.
- A named, reproducible integration tool that calls the endpoint: an **n8n** workflow
  (versioned in the repo, imported automatically by Docker Compose), plus a demo script
  (roteiro) covering success, the resulting history, and rejections.
- Tests proving the dispatch is correct and that every existing precondition still applies
  unchanged.

Out of scope:
- Any real e-mail inbox integration (IMAP polling, parsing an inbound e-mail). The PDF says
  "via alguma ferramenta como email"; the tool chosen for this delivery is n8n (§7). An
  e-mail-triggered n8n workflow could be added later without any API change.
- `AWAITING_APPROVAL`/`IN_PROGRESS`/`CANCELED` as externally reachable targets through this
  endpoint — already covered by `POST .../quote/send` (administrative) and
  `POST .../quote/decision` (external, `specs/service-order-quote-decision/`) respectively;
  duplicating them here would create two different external paths to the same transition
  with two different validation rules (quote completeness, tracking context) to keep in
  sync.
- Any change to the existing JWT-protected action endpoints — all unchanged (behavior,
  request shape, authentication, fixed history descriptions), still the administrative path
  into the same transitions.

## 5. Endpoint

```
POST /api/v1/service-orders/{id}/status
X-External-Status-Secret: <shared secret>
Content-Type: application/json

{"status": "IN_DIAGNOSIS", "notes": "optional, persisted in the history entry"}
```

`200 OK`:

```json
{"id": "<uuid>", "code": 42, "previousStatus": "RECEIVED", "status": "IN_DIAGNOSIS"}
```

## 6. Acceptance criteria

1. `{"status": "IN_DIAGNOSIS"}` on a `RECEIVED` order transitions it exactly like
   `POST /{id}/diagnosis` already does, with the same history entry.
2. `{"status": "COMPLETED"}` on an eligible `IN_PROGRESS` order transitions it exactly like
   `POST /{id}/finalize` already does; with a required execution still pending it is
   rejected with `409 EXECUTIONS_NOT_COMPLETED`, exactly like `/finalize`.
3. `{"status": "DELIVERED"}` on a `COMPLETED` order transitions it exactly like
   `POST /{id}/deliver` already does.
4. A `status` value this endpoint does not support (`AWAITING_APPROVAL`, `IN_PROGRESS`,
   `CANCELED`, `RECEIVED`, or an unknown string) is rejected with `400 VALIDATION_ERROR`.
5. A structurally valid but currently-illegal transition (e.g. `DELIVERED` requested on an
   order still `RECEIVED`) is rejected with `409` and the same code the underlying use case
   already returns for that precondition failure.
6. A missing or incorrect `X-External-Status-Secret` is rejected with `401` before the order
   is looked up — the quote-decision secret and an administrative JWT are both rejected
   too.
7. Every successful call produces the same `service_order_history` entry (event,
   previous/new status) the corresponding existing endpoint produces; when `notes` is sent,
   its description is `"<fixed text> Notes: <trimmed notes>"`.
8. `notes` longer than 500 characters is rejected with `400`.
9. None of the failures in AC2 (pending executions), AC4, AC5, AC6, AC8 changes the order's
   status or adds a history entry.
10. The three existing endpoints (`/diagnosis`, `/finalize`, `/deliver`) are unchanged in
    behavior, request shape, and authentication.
11. The n8n workflow in the repo, started with `docker compose up`, performs the call with
    the secret from the environment and shows the API's response (success or rejection)
    to the operator.

## 7. Decisions (resolved 2026-10-19)

The original draft left these open (SDD, `CLAUDE.md` §17); they were confirmed with the
team before implementation:

1. **Externally reachable statuses**: `IN_DIAGNOSIS`, `COMPLETED`, `DELIVERED` only — the
   three transitions with no extra required input beyond "it happened" (no quote, no
   execution id). `IN_PROGRESS` only arises from quote *approval*, which has precondition
   data (a sent quote) this generic endpoint does not carry, and is already externally
   reachable through `POST .../quote/decision`.
2. **`notes`**: kept, validated (§3 BR6) and persisted in the history entry (§3 BR5).
   The draft's "accepted but not plumbed through" option was rejected: a field the API
   accepts and silently drops is misleading to the integrator.
3. **Response**: reduced to `{id, code, previousStatus, status}` (§3 BR8) instead of the
   full `ServiceOrder` — the tool only needs to confirm the outcome.
4. **Integration tool**: n8n, self-hosted as a Docker Compose service, with the workflow
   versioned under `tools/n8n/` and imported on startup — reproducible with no manual
   configuration beyond `.env`.
