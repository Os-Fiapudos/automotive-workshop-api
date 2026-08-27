# Service Order Tracking — Requirements

Source: Jira user story "Consultar Acompanhamento de OS" (RF12), pasted into the SDD prompt
that started this feature. This document formalizes that prompt as this project's
`requirements.md`, resolving the ambiguities it explicitly flagged (see §0) with the
requester before any design work started, per `specs/README.md`.

## 0. Ambiguities resolved before design (SDD rule: clarify, don't assume)

The source spec explicitly left several implementation-relevant questions open ("o token
pode ser enviado em cabeçalho próprio ou como Bearer Token específico", "retorna HTTP 401
ou 404, conforme política de segurança", no description at all of how a token is ever
issued). These were resolved with the requester before writing `design.md`:

1. **Token issuance**: out of the four options considered (admin issuance endpoint,
   auto-generate on OS creation, out-of-scope/seed-only, or leaving it fully unspecified),
   the requester chose **auto-generate on OS creation** — `POST /api/v1/service-orders`
   (Service Order Opening, already implemented) now also generates a tracking token for the
   new order and returns it once in the creation response. This necessarily touches
   `internal/features/service-order`, outside this feature's own package — see
   `design.md` §5 for how that's kept from becoming a forbidden cross-feature coupling.
2. **Token transport**: a **custom header**, `X-Tracking-Token` — not
   `Authorization: Bearer`, to keep it unambiguous from the administrative JWT.
3. **Invalid/revoked token status code**: **401** for a missing, invalid, or revoked token;
   **404** for a `{codigo}` that doesn't identify any service order. (The alternative —
   404 for both — was offered and declined.)
4. **Token storage**: only a **SHA-256 hash** of the token is persisted, never the raw
   value (RNF03). The raw token is returned to the caller exactly once, at issuance.

Two more gaps in the source spec were resolved by inference from its own explicit
constraints, not by invention, and are recorded here for traceability:

5. **Token expiration**: the source spec requires the token to be **revocable**
   ("o token deve ser revogável") but never mentions expiration. No TTL/expiry is
   implemented — inventing one would be adding a requirement not in the source spec. A
   `revoked_at` column exists so revocation can be represented and checked; see §3.4 note
   on `AUTO-1` open item.
6. **Milestones content**: "os principais marcos" is satisfied by the existing
   `ServiceOrderHistory` trail (`event`, `previousStatus`, `newStatus`, `occurredAt`), which
   already records exactly the significant events of an order's lifecycle
   (`docs/entities.md`). Its free-text `description` field is excluded from the public
   projection — the source spec's own exclusion list ("não devem ser expostos... informações
   administrativas") is written in terms of *kinds* of data, and free-text internal notes are
   the same kind of thing as the excluded PII: not necessary for "saber em qual etapa está o
   meu veículo".

## 1. User story

As a customer, I want to check the progress of my service order, so I can know which stage
my vehicle is at without contacting the workshop.

## 2. Related requirements

- RF12 — Secure service order tracking.
- RNF03 — Protection of inputs and sensitive data.
- RNF04 — Consistent REST contract.
- RNF08 — Logs without complete personal data.

## 3. Business rules

1. The service order code alone must not grant access — a valid tracking token tied to
   that specific order is also required.
2. The tracking token is a random, high-entropy value, bound exclusively to one service
   order (BR1 of §0 item 1: generated when that order is created).
3. The token must be revocable.
   - **AUTO-1 (open item, out of scope here)**: the source spec's "Endpoint sugerido"
     section lists only the GET tracking endpoint — no revoke endpoint is specified. This
     feature implements the *data model support* for revocation (a nullable `revoked_at`)
     and the *validator* honoring it (a revoked token is rejected exactly like an invalid
     one), but does not add an endpoint to actually revoke a token, since none was
     requested. A future slice can add that action against the existing column without a
     schema change.
4. The response must contain only the information necessary for tracking: the order's code,
   a limited identification of the vehicle, its current status, and its main milestones.
5. CPF/CNPJ, phone, e-mail, data from other service orders, and administrative information
   must never be returned by this endpoint.
6. The customer must be able to see the status and the main milestones of their own order,
   and only their own.
7. The public contract must be a distinct DTO from the administrative one
   (`serviceorder.Response`, used by `POST/GET .../service-orders...`).
8. Tracking tokens must never appear in logs (RNF08).

## 4. Endpoint

`GET /api/v1/acompanhamento/{codigo}`

- `{codigo}` is the service order's existing human-readable `code` (the same identifier
  already used by `service_orders.code`, per `docs/entities.md`).
- Token is sent via the `X-Tracking-Token` request header (§0 item 2).
- Does not require the administrative JWT — `middleware.RequireAuth` is not applied to
  this route.

## 5. Acceptance criteria

- [ ] AC1 — A customer with a valid token can look up their own service order.
- [ ] AC2 — The code alone (no token, or empty token) does not grant access → `401`.
- [ ] AC3 — An unknown `{codigo}` → `404`.
- [ ] AC4 — An invalid or revoked token for a `{codigo}` that does exist → `401`.
- [ ] AC5 — A token issued for service order A cannot be used to look up service order B
      (→ `401` when presented against B's `{codigo}`).
- [ ] AC6 — The response contains: code, a limited vehicle identification (license plate,
      brand, model, year, color — no `id`/`code`/`status`/owning customer), current status,
      and the milestone trail (event, previous/new status, `occurredAt` — no free-text
      `description`).
- [ ] AC7 — CPF/CNPJ, phone, and e-mail are never present anywhere in the response.
- [ ] AC8 — No administrative data (customer name/id, internal ids, other orders) is present
      in the response.
- [ ] AC9 — The public DTO (`trackingResponse`) is a separate Go type from the
      administrative `serviceorder.Response`, with no shared struct.
- [ ] AC10 — The request never requires/accepts the administrative JWT.
- [ ] AC11 — Automated tests cover valid access, invalid token, unknown code, revoked
      token, and cross-order access (AC1–AC5).

## 6. Out of scope

Per the source spec's own "Fora de escopo" section, plus AUTO-1 above: notifications,
service order status changes, an administrative panel, and a token-revocation endpoint.
