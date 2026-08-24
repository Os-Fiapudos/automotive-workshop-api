# Requirements — Service Order Stock Usage (FP-19)

Source: Jira ticket FP-19, "Baixa de peças e insumos na OS" — "Como mecânico, quero
registrar as peças e os insumos utilizados na OS, para manter o estoque atualizado e
rastrear o consumo de cada atendimento." Related requirements: RF10 (deduct and track
stock used on a service order), RNF06 (minimum coverage on the critical stock domain),
RNF07 (transactional integrity).

## 1. User story

As a mechanic, I want to register the parts and supplies used on a service order, so stock
stays up to date and each visit's consumption is traceable.

## 2. Scope of this feature

1. Registering one or more parts/supplies deducted from stock against a service order that
   is currently `EM_EXECUCAO`, all-or-nothing per request.
2. Listing the stock movements recorded against a service order.
3. Reversing (estorno) a previously registered usage movement, restoring the deducted
   quantity and leaving a traceable link back to the original movement.

### 2.1 Explicitly out of scope

- Reconciling the deducted quantity against the quantity budgeted in the order's `Quote` —
  the ticket explicitly allows the two to differ ("a baixa efetiva pode ser diferente da
  quantidade prevista no orçamento, mas deve permanecer rastreável"); this feature only
  guarantees traceability (product, order, quantity, date, type), not reconciliation.
- Product catalog CRUD and a product's own manual stock adjustments — already implemented
  by `specs/product-management/`. This feature adds a second, service-order-scoped producer
  of the same `stock_movements` ledger (see `design.md` §0), not a new product feature.
- Reversing a manual product adjustment, or reversing a reversal — not requested by the
  ticket.

## 3. Business rules

- BR1 — A usage deduction can only be registered while the service order is `EM_EXECUCAO`.
- BR2 — The referenced product must exist and be `ACTIVE`.
- BR3 — Quantity must be greater than zero.
- BR4 — A deduction can never take a product's balance negative.
- BR5 — Every movement records product, service order (when applicable), quantity, date,
  and type.
- BR6 — The deduction and its movement record are written in the same transaction (RNF07).
- BR7 — A request may include multiple items; if any single item fails validation or stock
  check, the entire request is rolled back — no partial deduction.
- BR8 — Concurrent requests against the same product's balance must never both succeed past
  what stock actually allows (no lost update / overselling).
- BR9 — A reversal creates a new, inverse `ENTRY` movement linked to the original `EXIT`
  movement it undoes; the original movement itself is never edited or deleted. A movement
  can be reversed at most once.

## 4. Endpoints

- `POST /api/v1/service-orders/{id}/stock-movements` — register one or more usage
  deductions against order `{id}`.
- `GET /api/v1/service-orders/{id}/stock-movements` — list the stock movements recorded
  against order `{id}` (usage deductions and their reversals).
- `POST /api/v1/service-orders/{id}/stock-movements/{movementId}/reversal` — reverse a
  previously registered usage movement.

Named and pathed in English under the existing `/api/v1/service-orders` collection, same
convention `specs/service-order-execution/requirements.md` §5 already establishes — not the
ticket's literal `ordens-servico/itens-utilizados` suggestion, which follows the product
feature's older Portuguese-route convention instead (`design.md` §0 explains the choice).
All three `requireAuth`-protected, per `specs/auth/design.md` §7.

## 5. Entity: StockMovement

`docs/entities.md`'s new `StockMovement` entity (`docs/schema.sql`'s `stock_movements`
table) is this feature's persisted shape, shared with `specs/product-management/`'s manual
adjustments — see `design.md` §0 for why one shared table, not two.

Fields: id, product id, service order id (absent for a manual product adjustment), type
(`ENTRY`/`EXIT`), quantity, previous stock, new stock, reason (absent for a service-order
movement), reversed-movement id (reversals only), occurred-at.

## 6. Acceptance criteria (from the ticket's checklist)

- [ ] A product used on an `EM_EXECUCAO` order can be registered.
- [ ] An invalid quantity (≤ 0) is rejected.
- [ ] A nonexistent or inactive product is rejected.
- [ ] Insufficient balance prevents the operation.
- [ ] Stock never goes negative.
- [ ] Every deduction is linked to its service order.
- [ ] Every movement records date, quantity, and product.
- [ ] A partial failure rolls back every deduction in that same request.
- [ ] Concurrent operations never consume the same balance twice.
- [ ] A reversal preserves the original movement.
- [ ] Tests exist for insufficient balance, rollback, and concurrency.
- [ ] The critical stock domain participates in coverage measurement (RNF06).

## 7. Non-functional requirements

- RNF06 — Minimum test coverage on the critical stock domain: unit tests for the domain/
  service validation plus integration tests (insufficient stock, multi-item rollback,
  concurrency, reversal) against the real repository, per `design.md` §5.
- RNF07 — Transactional integrity: the deduction(s), or the reversal, and their
  `stock_movements` row(s) are written atomically — a failure at any point rolls back every
  write already made in that same request.
