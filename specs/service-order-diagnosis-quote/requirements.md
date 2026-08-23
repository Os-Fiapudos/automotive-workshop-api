# Requirements — Service Order Diagnosis and Quote Composition

Status: **Approved for implementation**
Feature folder: `internal/features/service-order/` (extends the existing package — same
`ServiceOrder` aggregate, not a new feature package; see `design.md` §1.1 for why).

## 1. Context

This is the direct follow-up explicitly deferred by
`specs/service-order-opening/requirements.md` §7.1:

> A future card (diagnosis + quote composition) will introduce `Orcamento`/`ItemOrcamento`
> on top of the `ServiceOrder` created here, with automatic price calculation and status
> transitions (`RECEBIDA` → `EM_DIAGNOSTICO` → ...).

`docs/schema.sql` already models Orçamento/ItemOrçamento as `quotes`/`quote_products`/
`quote_services` (English translation, per `CLAUDE.md` §8), created together with
`ServiceOrder`'s own tables even though no feature writes to them yet. This card is the
first feature to write to those tables.

Source: `card.txt` (RF05, RF06, RF08, RNF07).

## 2. User story

> As a mechanic,
> I want to register the diagnosis and the required items,
> so that I can generate a complete, trustworthy quote for the customer.

## 3. Business rules

1. Starting the diagnosis is only allowed for a service order currently `RECEBIDA`. It
   transitions the order to `EM_DIAGNOSTICO`. Starting diagnosis on an order in any other
   status is rejected (RF08).
2. Composing (or recomposing) the quote is only allowed once diagnosis has started, i.e.
   the order is `EM_DIAGNOSTICO` or `AGUARDANDO_APROVACAO` — not `RECEBIDA`. It is rejected
   for `RECEBIDA` (diagnosis not started yet).
3. A composed quote must have at least one item (product or service combined).
4. Each item (product or service) records: a description snapshot, a quantity (> 0), a
   unit price snapshot, and a total (`quantity * unit price`).
5. Description and price are copied from the product/service catalog at the moment of
   composition. Later catalog changes (renaming a product, changing a price) must never
   retroactively change an already-composed item.
6. Quantities must be greater than zero; any item with `quantity <= 0` is rejected.
7. The quote's total amount is calculated exclusively by the back end — a client-sent
   total (or per-item total) is never trusted, only used to verify shape.
8. Composing a quote never adjusts `products.current_stock` — no stock movement happens at
   this stage.
9. A quote that has already been decided (`APPROVED` or `REJECTED`) cannot be altered — a
   later composition request for it is rejected. Composing (`PUT`) is otherwise idempotent
   while the quote is `PENDING`: it fully replaces the item list and recalculates the total
   every time it's called, and (re-)sets the order status to `AGUARDANDO_APROVACAO` (this
   also covers the very first composition, which additionally performs that transition).
10. Every relevant change (diagnosis start, quote composition) must generate a
    `service_order_history` entry, transactionally with the change itself (RNF07).
11. A referenced product must exist and be `ACTIVE`; an unknown or `INACTIVE` product id is
    rejected.
12. A referenced service must exist; an unknown service id is rejected. **Scope limitation**
    (see §7.3): `services` has no `status` column in the current schema — no feature owns
    service catalog management yet — so "inactive service" cannot be checked. Only product
    items get the active/inactive check the card describes; this is a deliberate, documented
    gap, not a silent omission.

## 4. Scope

In scope for this feature:

- Two new use cases on the existing `ServiceOrder` aggregate:
  `StartDiagnosis` and `ComposeQuote`.
- `Quote`/quote-item domain types and their persistence (`quotes`, `quote_products`,
  `quote_services`).
- Status transitions `RECEBIDA → EM_DIAGNOSTICO` and `→ AGUARDANDO_APROVACAO`, each
  recorded in `service_order_history`.
- Schema additions: description snapshot columns on `quote_products`/`quote_services`,
  `quantity` on `quote_services`, and two new `history_event` values.
- Endpoints: start diagnosis, compose/replace the quote, read the quote.
- Protecting the new endpoints with `middleware.RequireAuth` (see §7.4 — a deliberate
  deviation from `customer`/`service-order-opening`, which are still unauthenticated).
- Unit and integration tests covering the rules above.

Out of scope (see §7):

- Quote approval/rejection by the customer (`AGUARDANDO_APROVACAO → EM_EXECUCAO`, or
  `Quote.status` moving away from `PENDING`) — a future card.
- Any status transition beyond `EM_DIAGNOSTICO`/`AGUARDANDO_APROVACAO`.
- Stock movements (`AuditServices`, decrementing `current_stock` on execution) — a future
  card; explicitly rule §3.8 forbids it happening here.
- Service catalog management (CRUD, `status` field) — not owned by this feature (§3.12).

## 5. Endpoints covered (contract details in `design.md`)

```
POST /api/v1/service-orders/{id}/diagnosis
PUT  /api/v1/service-orders/{id}/quote
GET  /api/v1/service-orders/{id}/quote
```

> **Naming note**: `card.txt` used `/ordens-servico/{id}/diagnostico` and
> `/ordens-servico/{id}/orcamento`. Per `CLAUDE.md` §8 and the precedent already set by
> `specs/service-order-opening/requirements.md` §5 (`/ordens-servico` →
> `/service-orders`), and since the schema already calls this entity `quotes` in English,
> this feature uses `/service-orders/{id}/diagnosis` and `/service-orders/{id}/quote`. A
> deliberate, documented deviation from the literal task wording, not an invented
> requirement.

## 6. Acceptance criteria

```
[ ] Diagnosis can be registered for an order that is RECEBIDA
[ ] The order automatically moves to EM_DIAGNOSTICO
[ ] Starting diagnosis on a non-RECEBIDA order is rejected
[ ] Services, parts, and supplies can be added to a quote
[ ] Invalid quantities (<= 0) are rejected
[ ] A non-existent or inactive product is rejected
[ ] A non-existent service is rejected
[ ] Each item preserves its description and applied price as a snapshot
[ ] The total value is calculated on the server
[ ] Values sent by the API client do not override the official calculation
[ ] Composing the quote does not reduce stock
[ ] A decided quote (APPROVED/REJECTED) cannot be altered
[ ] Composing the quote (first time or recompose) sets the order to AGUARDANDO_APROVACAO
[ ] Changes generate service order history entries
[ ] GET returns the current quote with its items
[ ] There are calculation tests, price/description snapshot tests, and transition tests
```

## 7. Future requirements (explicitly deferred, not implemented here)

### 7.1 Quote approval/rejection

Moving `Quote.status` away from `PENDING` (and the corresponding
`AGUARDANDO_APROVACAO → EM_EXECUCAO` order transition) is a separate, not-yet-specified
future card. This feature only ever produces/keeps `Quote.status = PENDING`.

### 7.2 Stock movement

Decrementing `products.current_stock` happens only when execution actually consumes a
part — out of scope here by explicit rule (§3.8). `AuditServices`/execution tracking is
a separate future card.

### 7.3 Service catalog `status`

If a future service-catalog-management feature adds `services.status`, this feature's
`ComposeQuote` should be revisited to also reject inactive services, mirroring the
product check. Not implemented now — inventing an unrequested schema field would violate
`CLAUDE.md` §17.

### 7.4 Authentication

Decided explicitly for this feature (unlike `customer`/`service-order-opening`, which
remain open decisions per `specs/auth/design.md` §7): the new routes are wrapped in
`middleware.RequireAuth`, since a mechanic performing a diagnosis/quote is expected to be
an authenticated user. This does not retroactively change the existing
`POST /api/v1/service-orders` route's authentication status — that stays its own open
decision.

## 8. Open questions resolved before implementation

- **Does composing the quote transition the order to `AGUARDANDO_APROVACAO`?** Yes, every
  successful `PUT`, including recomposition while still `PENDING`. Resolved with the
  project owner — see §3.9.
- **Snapshot of item description**: the existing schema only snapshot price/quantity.
  Added `applied_description` columns via schema change (`design.md` §3.1) rather than
  reading the live catalog name, since the acceptance checklist explicitly requires the
  description to survive catalog changes.
- **`quote_services.quantity`**: did not exist (only `quote_products` had it). Added with
  `DEFAULT 1` and the same `> 0` check, since the card requires quantity on every item type
  uniformly (§3.4).
- **New `history_event` values**: `creation`/`approval`/`completion`/`cancellation` did not
  cover diagnosis start or quote composition. Added `diagnosis_started` and
  `quote_composed`.
