# Requirements — Service Order Status Update via External Tool (E-mail)

Status: **Proposed** (one scope decision below needs confirmation before design is final —
see §7)
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
*other* transitions a workshop might plausibly want to trigger from an outside tool (e.g.
an inbound e-mail from a mechanic or the Resend integration's reply-handling, forwarded by
some out-of-scope mail-processing step into one HTTP call), without re-implementing any
transition rule the domain already enforces.

## 2. User story

> As an external tool (e.g. an e-mail-processing integration configured by the workshop),
> I want to notify the system that a service order has reached a new status,
> so that the order's status stays current without an administrator manually calling the
> existing JWT-protected action endpoints.

## 3. Business rules

1. This feature adds **no new transition logic**. It is a second, externally-authenticated
   entry point into transitions the domain already implements and already enforces the
   preconditions for (`startDiagnosis`, `finalizeOrder`, `deliverOrder` — see §7 for why
   only these three, not every transition).
2. The caller identifies the target status by name (`RECEIVED`'s following statuses only —
   you cannot "update to RECEIVED", since that is the order's fixed starting status, never a
   target of this endpoint).
3. Every precondition the underlying transition already enforces (BR of the owning spec)
   still applies unchanged — e.g. `finalizeOrder` still requires the order to be
   `IN_PROGRESS` with no execution left unfinished, exactly as
   `specs/service-order-execution/requirements.md` already specifies; this feature does not
   relax, bypass, or duplicate that rule, it only reaches the same use case through a
   different, externally-authenticated route.
4. Authentication is a shared secret the external tool presents (`X-External-Status-Secret`
   header) — not the administrative JWT (the caller is a tool, not a logged-in
   administrator) and not the customer tracking token (the caller is not the customer).
   Same posture as `specs/service-order-quote-decision/requirements.md` §9 item 4, with its
   own, separate secret (§7 — kept distinct so the two external integrations can be rotated
   independently).
5. Every transition performed through this endpoint still generates its
   `service_order_history` entry, through the same existing transactional write the
   underlying use case already performs — no change there.
6. An attempt to reach a status this endpoint does not support (§7), or a transition that is
   not currently legal for the order's actual status, is rejected the same way the
   corresponding existing endpoint already rejects it (409/404, per that endpoint's own
   error mapping) — this feature introduces no new rejection semantics, only a new way to
   reach the existing ones.

## 4. Scope

In scope:
- One new endpoint, `POST /api/v1/service-orders/{id}/status`, dispatching to
  `startDiagnosis`/`finalizeOrder`/`deliverOrder` based on the requested target status.
- Shared-secret authentication for this endpoint.
- Tests proving the dispatch is correct and that every existing precondition still applies
  unchanged.

Out of scope:
- Any real e-mail inbox integration (IMAP polling, parsing an inbound e-mail, mapping its
  content to a status) — out of the PDF's required scope (it says "via alguma ferramenta
  como email," not "implement e-mail parsing"); this feature provides the HTTP endpoint such
  a tool would call, the same posture `specs/service-order-quote-decision/requirements.md`
  §9 already takes for its own external channel.
- `AWAITING_APPROVAL`/`IN_PROGRESS`/`CANCELED` as externally reachable targets through this
  endpoint — already covered by `POST .../quote/send` (administrative) and
  `POST .../quote/decision` (external, `specs/service-order-quote-decision/`) respectively;
  duplicating them here would create two different external paths to the same transition
  with two different validation rules (quote completeness, tracking context) to keep in
  sync.
- Any change to the existing JWT-protected action endpoints — all unchanged, still the
  administrative path into the same transitions.

## 5. Endpoint

```
POST /api/v1/service-orders/{id}/status
X-External-Status-Secret: <shared secret>

{"status": "IN_DIAGNOSIS", "notes": "optional, forwarded to the history entry"}
```

## 6. Acceptance criteria

1. `{"status": "IN_DIAGNOSIS"}` on a `RECEIVED` order transitions it exactly like
   `POST /{id}/diagnosis` already does, with the same history entry.
2. `{"status": "COMPLETED"}` on an eligible `IN_PROGRESS` order transitions it exactly like
   `POST /{id}/finalize` already does.
3. `{"status": "DELIVERED"}` on a `COMPLETED` order transitions it exactly like
   `POST /{id}/deliver` already does.
4. A `status` value this endpoint does not support (`AWAITING_APPROVAL`, `IN_PROGRESS`,
   `CANCELED`, `RECEIVED`, or an unknown string) is rejected with `400 VALIDATION_ERROR`.
5. A structurally valid but currently-illegal transition (e.g. `COMPLETED` requested on an
   order still `RECEIVED`) is rejected with the same status/code the underlying use case
   already returns for that precondition failure.
6. A missing or incorrect `X-External-Status-Secret` is rejected with `401`, and no
   transition is applied.
7. Every successful call through this endpoint produces the same `service_order_history`
   entry shape the corresponding existing endpoint already produces.
8. The three existing endpoints (`/diagnosis`, `/finalize`, `/deliver`) are unchanged in
   behavior, request shape, and authentication.

## 7. Open question — confirm before design is finalized

**Which statuses should be externally reachable through this endpoint?** This spec's
working assumption (§4/§6) is `IN_DIAGNOSIS`, `COMPLETED`, `DELIVERED` only — the three
transitions with no extra required input beyond "it happened" (no quote, no execution id).
`AWAITING_APPROVAL`/`IN_PROGRESS`/`CANCELED` are deliberately excluded because they are
already reachable through the quote-decision external channel
(`specs/service-order-quote-decision/requirements.md` §9), and because `IN_PROGRESS`
specifically only arises from quote *approval*, which has precondition data (a sent quote)
this generic endpoint does not carry. **This is a reasonable reading of the PDF's generic
"atualização de status... via alguma ferramenta como email" requirement, but the PDF itself
does not name which statuses — flagged here per SDD (`CLAUDE.md` §17: ask before
implementing when requirements are ambiguous) for explicit confirmation before `design.md`'s
dispatch table is implemented.**
