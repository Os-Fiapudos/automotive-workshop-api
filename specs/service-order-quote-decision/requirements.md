# Requirements — Service Order Quote Sending, Approval, and Rejection

Status: **Approved for implementation**
Feature folder: `internal/features/service-order/` (extends the existing package — same
`ServiceOrder`/`Quote` aggregate introduced by
`specs/service-order-diagnosis-quote/`, not a new feature package; see `design.md` §1 for
why).

## 1. Context

This is the direct follow-up explicitly deferred by
`specs/service-order-diagnosis-quote/requirements.md` §7.1 ("Quote approval/rejection") and
flagged as an external, unimplemented precondition by
`specs/service-order-execution/requirements.md` §2.1: no code in the repository produced the
`AWAITING_APPROVAL → IN_PROGRESS` transition until this feature. It also introduces the
explicit "send the quote to the customer" step the original card described but
`service-order-diagnosis-quote` did not implement — composing a quote there already moved
the order to `AWAITING_APPROVAL` by itself.

Source: Jira user story (RF06, RF07, RF08, RF12, RNF07), pasted into the SDD prompt that
started this feature.

## 2. User story

> As an attendant,
> I want to register the sending of the quote and the customer's decision on it,
> so that I can determine whether the services can be executed.

## 3. Ambiguities resolved before design (SDD rule: clarify, don't assume)

The source spec left several points ambiguous or in direct conflict with already-shipped
behavior. These were resolved with the requester before writing `design.md`:

1. **`CANCELED` status** (source spec's own flagged, non-final recommendation): **added**.
   Without it, a rejected quote — which `specs/service-order-diagnosis-quote/requirements.md`
   §3.9 already forbids altering once decided — would leave its order permanently stuck in
   `AWAITING_APPROVAL` with no path forward. `CANCELED` is the order's explicit closing
   status for this case (§5).
2. **Compose vs. send owning the `IN_DIAGNOSIS → AWAITING_APPROVAL` transition**:
   `specs/service-order-diagnosis-quote/design.md` §1.6 already made `ComposeQuote` (`PUT
   .../quote`) perform this transition on every successful compose, including a recompose.
   This card's own RF06 explicitly attributes the transition to *sending* the quote, not
   composing it. Resolved: **sending owns the transition**. `ComposeQuote` no longer
   transitions the order — it only requires diagnosis to have started (order not
   `RECEIVED`), same precondition as before, just without the side effect. This is a
   behavioral change to already-shipped code from
   `specs/service-order-diagnosis-quote/design.md` §1.6/§1.4, recorded there as an erratum
   rather than rewritten as if it had always been this way (`specs/README.md`'s "the
   specification is the source of truth" rule).
3. **"A versão efetivamente apresentada"**: the existing `Quote` had no versioning concept
   at all (a recompose replaced items in place). Resolved: `quotes.version` is an integer
   that increments on every successful compose/recompose
   (`specs/service-order-diagnosis-quote/design.md`'s `SaveQuote`, extended by this feature);
   sending snapshots the current `version` into `sent_version`, so it is always possible to
   tell which exact version of the quote the customer was actually shown.
4. **RF12's "mecanismo seguro"**: resolved by reusing, not reinventing, the mechanism
   `specs/service-order-tracking/` already built for this exact purpose — an opaque tracking
   token issued once per order at creation, sent via the `X-Tracking-Token` header, and
   validated against a SHA-256 hash. The approve/reject endpoints sit under the same
   `/acompanhamento/{codigo}` namespace tracking already established and are authenticated
   the same way, rather than inventing a second secure-response mechanism.

## 4. Business rules

1. Only a complete quote (at least one item — the same completeness `ComposeQuote` already
   guarantees before a quote can exist at all) can be sent. Sending a service order with no
   composed quote yet is rejected.
2. Sending records the send date/time (`sent_at`) and the quote version actually presented
   (`sent_version`), and moves the order from `IN_DIAGNOSIS` to `AWAITING_APPROVAL`.
   Sending is only allowed while the order is `IN_DIAGNOSIS`.
3. The customer's decision (approve/reject) is only allowed while the quote is `PENDING`.
4. Approving sets the quote to `APPROVED`, records the response date/time (`responded_at`),
   and automatically moves the order to `IN_PROGRESS`.
5. Rejecting sets the quote to `REJECTED`, records `responded_at`, and automatically moves
   the order to `CANCELED` (§3 item 1).
6. The response date and the customer's decision are immutable once recorded — a quote that
   is no longer `PENDING` cannot be decided again, in either direction.
7. A second response — whether the same decision repeated or a different one — is rejected
   as a conflict. The two cases are not distinguished from the caller's perspective; both
   report "this quote has already been decided."
8. A customer cannot decide a quote belonging to a different service order than the one
   their tracking token was issued for (the same invariant
   `specs/service-order-tracking/requirements.md` §3.1/§6 already established for the GET
   endpoint).
9. Every transition this feature performs (send, approve, reject) generates a
   `service_order_history` entry, in the same transaction as the change itself — if the
   history write fails, the whole decision (quote status, `responded_at`, and the order's
   status change) is rolled back; nothing is left partially applied (RNF07).
10. The MVP registers the send even though no real e-mail integration exists — a
    notification failure must never block or undo a send that already happened.

## 5. `CANCELED` (domain decision, §3 item 1)

`ServiceOrder.status` gains a seventh value, `CANCELED`, reached only from
`AWAITING_APPROVAL` when the quote is rejected. The six statuses required by the original
domain (`docs/entities.md`) are unchanged; `CANCELED` is an additional branch, not a
replacement for any of them. No other transition produces or consumes `CANCELED` in this
feature's scope (e.g. there is no "reopen a cancelled order" action here).

## 6. Endpoints

```
POST /api/v1/service-orders/{id}/quote/send
POST /api/v1/acompanhamento/{codigo}/orcamento/aprovar
POST /api/v1/acompanhamento/{codigo}/orcamento/reprovar
```

> **Naming note**: the source spec used `/ordens-servico/{id}/orcamento/enviar`. Per the
> precedent `specs/service-order-diagnosis-quote/requirements.md` §5 already set
> (`/ordens-servico` → `/service-orders`, and the entity is already called `quote` in this
> codebase's English routes), sending is exposed as
> `POST /service-orders/{id}/quote/send`, alongside the existing `PUT .../quote` and
> `GET .../quote`. The approve/reject endpoints, by contrast, keep the source spec's
> Portuguese `orcamento/aprovar`/`orcamento/reprovar` wording, since they nest under the
> `/acompanhamento/{codigo}` namespace `specs/service-order-tracking/` already shipped
> untranslated — introducing a translated sibling would be the inconsistent choice here.

- `POST .../quote/send`: requires the administrative JWT (an attendant action), same
  convention as the diagnosis/compose/get-quote routes already added by
  `specs/service-order-diagnosis-quote/`.
- `POST .../orcamento/aprovar` / `.../reprovar`: never require the administrative JWT —
  authenticated via the `X-Tracking-Token` header instead (§3 item 4), same convention as
  `GET /api/v1/acompanhamento/{codigo}`.

## 7. Acceptance criteria

- [ ] AC1 — An incomplete quote (none composed yet) cannot be sent.
- [ ] AC2 — Sending records `sent_at`/`sent_version` and moves the order to
      `AWAITING_APPROVAL`.
- [ ] AC3 — Sending is only allowed from `IN_DIAGNOSIS` (not before diagnosis, not a
      second time once already sent).
- [ ] AC4 — Approval is only accepted while the quote is `PENDING`.
- [ ] AC5 — Approval records `responded_at`.
- [ ] AC6 — Approval moves the order to `IN_PROGRESS`.
- [ ] AC7 — Rejection records `responded_at` and sets the quote to `REJECTED`.
- [ ] AC8 — Rejection moves the order to `CANCELED` (§5).
- [ ] AC9 — A quote cannot be both approved and rejected.
- [ ] AC10 — A repeated decision (same or different) on an already-decided quote is
      rejected consistently (409), never partially applied.
- [ ] AC11 — Every decision (send, approve, reject) generates a `service_order_history`
      entry.
- [ ] AC12 — A customer cannot respond to a service order other than the one their token
      was issued for.
- [ ] AC13 — A failure while writing the history entry rolls back the entire decision
      (quote status, `responded_at`, order status) — nothing is left partially applied.
- [ ] AC14 — Automated tests cover approval, rejection, repetition, and unauthorized/
      cross-order access.

## 8. Out of scope

Per the source spec's own "Fora de escopo" section: real e-mail integration, editing the
quote after it has been sent (recomposing after send is unaffected — it was already possible
before this feature and is not extended or restricted here), quote creation itself (owned by
`specs/service-order-diagnosis-quote/`), and any definition of `CANCELED` beyond the
explicit decision recorded in §5.
