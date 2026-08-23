# Requirements — Service Order Execution, Finalization, and Delivery

Source: Jira user story "Como mecânico, quero registrar a execução e a conclusão dos
serviços, para manter o andamento da OS atualizado e mensurável." Related requirements:
RF08 (update status per allowed actions), RF09 (register execution start/end), RF11
(expose the order's history), RNF07 (transactional integrity).

## 1. User story

As a mechanic, I want to register the execution and completion of a service order's
services, and then hand the vehicle back to the customer, so the order's progress is
tracked and measurable.

## 2. Scope of this feature

This feature covers exactly three things:

1. Registering the start and end of each individual service execution performed against
   a service order (`EM_EXECUCAO`).
2. Finalizing a service order once its required executions are complete
   (`EM_EXECUCAO → FINALIZADA`).
3. Delivering a finalized service order back to the customer (`FINALIZADA → ENTREGUE`).

### 2.1 Explicitly out of scope

- The `RECEBIDA → EM_DIAGNOSTICO` and `EM_DIAGNOSTICO → AGUARDANDO_APROVACAO` transitions
  — already implemented by `specs/service-order-diagnosis-quote/`.
- The `AGUARDANDO_APROVACAO → EM_EXECUCAO` transition (quote approval). **No code in this
  repository implements it today** — `Quote.Status` can only become `APPROVED` through a
  test fixture, never through a real endpoint. This gap was flagged to the product owner
  during planning and the decision was: treat "the order is already `EM_EXECUCAO`" as an
  external precondition this feature depends on but does not create. Concretely, this
  means:
  - This feature's endpoints require `ServiceOrder.status == EM_EXECUCAO` to already hold
    before an execution can be started — same as requiring `AGUARDANDO_APROVACAO` to have
    been reached and its quote approved, since reaching `EM_EXECUCAO` is only possible once
    that has happened.
  - Until a future feature implements quote approval, these endpoints cannot be exercised
    end-to-end starting from a freshly opened order via the public API alone — only from a
    service order that was moved to `EM_EXECUCAO` some other way (e.g. directly in the
    database, as today's tests already do for fixtures the API can't produce, such as
    vehicles and catalog products).
- Notifications, quote/price calculation, editing a delivered order, and anything else not
  explicitly listed below.

## 3. Business rules

- BR1 — Manual, arbitrary status transitions are not allowed. Only the transitions listed
  in §4 are ever produced by this feature's endpoints.
- BR2 — An execution cannot be started unless the order's quote has already been approved
  — enforced via the `EM_EXECUCAO` precondition described in §2.1.
- BR3 — Each execution records which service it is for, its start date/time, and (once
  finished) its end date/time.
- BR4 — An execution's end date/time cannot be before its start date/time.
- BR5 — An order can only be finalized once every one of its **required executions** is
  complete. A required execution is one for a service that appears as a line item of the
  order's approved quote; every such service must have at least one completed (started and
  finished) execution before the order can move to `FINALIZADA`. (This definition is an
  explicit design decision — see `design.md` §2.4 — since the ticket does not itself define
  which executions are "required".)
- BR6 — Once an order is `FINALIZADA`, it no longer accepts new executions, nor finishing
  an execution still in progress ("baixas comuns").
- BR7 — Only an order that is `FINALIZADA` can be delivered (`ENTREGUE`).
- BR8 — Every status transition this feature performs records the previous status, the new
  status, the date/time, and an event type in the order's history
  (`ServiceOrderHistory`/`service_order_history`), reusing the mechanism
  `service-order-diagnosis-quote` already established. Starting/finishing an individual
  execution does not itself change the order's status, so it is not a
  `ServiceOrderHistory` entry — its own start/end record (§3, BR3) is its trail.
- BR9 — A completed history entry is immutable — never updated or deleted after being
  written, same as the rest of `service_order_history`.

## 4. Transitions this feature implements

```
EM_EXECUCAO → FINALIZADA   (finalize)
FINALIZADA  → ENTREGUE     (deliver)
```

No other `ServiceOrder.status` transition is implemented here.

## 5. Endpoints

- `POST /api/v1/service-orders/{id}/executions` — start a service execution.
- `POST /api/v1/service-orders/{id}/executions/{executionId}/finish` — finish a service
  execution.
- `POST /api/v1/service-orders/{id}/finalize` — finalize the order.
- `POST /api/v1/service-orders/{id}/deliver` — deliver the order.

Named and pathed in English, under the existing `/api/v1/service-orders` collection,
consistent with every other route this package already exposes (`docs/entities.md`'s
English-domain-language convention; only `ServiceOrder.status` values themselves stay in
Portuguese). All four are `requireAuth`-protected, per `specs/auth/design.md` §7's
"every non-public route requires auth" convention, which `service-order-diagnosis-quote`
and `service-order-query` already followed for every route they added.

## 6. Entity: ServiceExecution

The Jira ticket's `ExecucaoServico` entity is the same concept `docs/entities.md` already
documents as `AuditServices` ("records the start and end of the execution of each service
within a service order") — that entity exists in the schema/docs today but has no Go
feature implementing it yet (`CLAUDE.md` §1). This feature is that first implementation,
under the English name `ServiceExecution`. See `design.md` §2.3 for why its persisted shape
changes from `docs/schema.sql`'s current (unused-by-code) event-log columns to one row per
execution with its own start/end timestamps.

Fields: id, service order id, service id, started-at, ended-at (absent until finished).

## 7. Acceptance criteria (from the ticket's checklist)

- [ ] An execution cannot be started on an order that is not `EM_EXECUCAO` (BR2).
- [ ] Starting an execution records its start date/time and the service it is for (BR3).
- [ ] Finishing an execution records its end date/time (BR3).
- [ ] An end date/time before the start date/time is rejected (BR4).
- [ ] An invalid status transition returns HTTP 409 or 422.
- [ ] Finalizing an order requires its required executions to be complete (BR5).
- [ ] Finalizing moves the order to `FINALIZADA`.
- [ ] Delivering moves the order to `ENTREGUE`.
- [ ] A delivered order accepts no further operational changes from this feature (BR6/BR7).
- [ ] Every transition this feature performs generates a history entry (BR8).
- [ ] Unit tests for the state machine and integration tests both exist.

## 8. Non-functional requirements

- RNF07 — Transactional integrity: any operation that writes to more than one table (order
  status + history) must do so atomically — if one write fails, none of them are applied.
  Same pattern as `service-order-diagnosis-quote`'s `StartDiagnosis`/`SaveQuote`.
