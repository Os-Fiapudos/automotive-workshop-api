# Design — Service Order Quote Sending, Approval, and Rejection

Satisfies: `requirements.md` (all sections). Extends
`specs/service-order-diagnosis-quote/design.md` and reuses
`specs/service-order-tracking/design.md`'s token mechanism rather than reopening either.

## 1. Architecture decisions

### 1.1 Same package as `service-order`, not a new feature

Sending/approving/rejecting all operate on the same `ServiceOrder`/`Quote` aggregate
`specs/service-order-diagnosis-quote/` already added to `internal/features/service-order/`.
A separate package would either duplicate that aggregate or import
`internal/features/service-order` directly, breaking the "no direct coupling between
features" rule (`CLAUDE.md` §9.2) — the same reasoning that feature's own design.md §1.1
already used to justify not creating `internal/features/quote/`. New logic goes in
`quote_*.go` files in the existing package:

```
internal/features/service-order/
├── model.go              → + StatusCanceled; sendQuote/approveQuote/rejectQuote;
│                            Quote.Version/SentAt/SentVersion
├── quote_notifier.go      → + QuoteNotifier port, NoOpQuoteNotifier (new file)
├── quote_dto.go           → + QuoteResponse.Version/SentAt/SentVersion;
│                            quoteDecisionResponse (public DTO)
├── errors.go              → + ErrTrackingTokenInvalid
├── repository.go           → + findServiceOrderByCodeWithTrackingToken (lookups);
│                            SendQuote/DecideQuote (ServiceOrderRepository)
├── quote_repository.go    → + SendQuote/DecideQuote implementations;
│                            SaveQuote no longer transitions the order, increments version
├── quote_service.go        → + SendQuote/ApproveQuote/RejectQuote/decideQuote;
│                            ComposeQuote no longer calls markAwaitingApproval
├── service.go              → + notifier field/constructor param
├── handler.go               → + 3 new routes/handlers, trackingTokenHeader const
├── httpsupport.go           → + ErrTrackingTokenInvalid mapping (401)
├── quote_model_test.go      → markAwaitingApproval tests replaced by sendQuote/
│                            approveQuote/rejectQuote tests
├── quote_service_test.go   → + Send/Approve/Reject unit tests; ComposeQuote's own
│                            assertion updated (see §1.3)
└── fake_repository_test.go → + SendQuote/DecideQuote/findServiceOrderByCodeWithTrackingToken
```

The customer-facing approve/reject routes are registered from this same package's
`RegisterRoutes`, unwrapped (no `requireAuth`), the same way `service-order-tracking`
registers its own unauthenticated route from its own package — the difference here is that
the *business logic* they call belongs to the `ServiceOrder`/`Quote` aggregate itself, so the
routes are exposed by the aggregate's own package rather than by a second, tracking-shaped
feature that would need to import it.

### 1.2 Erratum: `ComposeQuote` no longer transitions the order

`specs/service-order-diagnosis-quote/design.md` §1.6 made `SaveQuote`'s `UPDATE
service_orders SET status = 'AWAITING_APPROVAL'` part of every successful compose. Per
`requirements.md` §3 item 2, that transition now belongs exclusively to `SendQuote`. Concretely:

- `ComposeQuote` (quote_service.go) replaces its call to the old `order.markAwaitingApproval()`
  with a direct, side-effect-free check: `if order.Status == StatusReceived { return nil,
  ErrDiagnosisNotStarted }`. Diagnosis must still have started; composing/recomposing itself
  no longer changes `order.Status`.
- `SaveQuote` (quote_repository.go) drops the `UPDATE service_orders` statement entirely. Its
  `service_order_history` insert for `quote_composed` is unchanged in shape (it already
  recorded `previous_status = new_status = order.Status`, which was only ever a real
  transition on the very first compose; it is now always a same-status entry, which is a
  legitimate history record of "this event happened while the order was in this status", the
  same pattern `Create`'s own `creation` event already uses).
- This is a behavioral change to already-merged code, not a reinterpretation: existing tests
  in `internal/handlers_test/service_order_test.go` that asserted `AWAITING_APPROVAL`
  immediately after a `PUT .../quote` (`TestServiceOrderComposeQuoteFullFlow`,
  `TestServiceOrderDetailByIDFullLifecycle`) were updated to assert `IN_DIAGNOSIS` instead,
  with a comment pointing at this design section.
- `specs/service-order-diagnosis-quote/design.md` §1.6 itself is annotated with a forward
  reference to this section rather than rewritten, per `specs/README.md`'s "a specification
  should not be changed just to make the code fit" rule — the original design was correct for
  what it implemented at the time; this feature is the one that changes the requirement.

### 1.3 Domain layer

`model.go` gains:

- `StatusCanceled Status = "CANCELED"`, added to `knownStatusValues` (used by
  `specs/service-order-query/`'s status filter).
- Three new `ServiceOrder` methods, replacing `markAwaitingApproval` (deleted):
  - `sendQuote() error` — requires `Status == StatusInDiagnosis`
    (`ErrInvalidStatusTransition` otherwise); sets `Status = StatusAwaitingApproval`.
    Unlike the old `markAwaitingApproval`, this is a **strict** precondition — it does not
    accept `AWAITING_APPROVAL` as a no-op source, since sending a second time is not a
    requirement this card describes (`requirements.md` AC3).
  - `approveQuote() error` — requires `Status == StatusAwaitingApproval`; sets `Status =
    StatusInProgress`. This is the transition
    `specs/service-order-execution/requirements.md` §2.1 flagged as an unimplemented,
    external precondition.
  - `rejectQuote() error` — requires `Status == StatusAwaitingApproval`; sets `Status =
    StatusCanceled`.
- `Quote` gains `Version int`, `SentAt *time.Time`, `SentVersion *int` (requirements.md §3
  item 3).

### 1.4 Notification port

`quote_notifier.go` (new file):

```go
type QuoteNotifier interface {
    NotifyQuoteSent(ctx context.Context, order *ServiceOrder, quote *Quote) error
}
type NoOpQuoteNotifier struct{}
func (NoOpQuoteNotifier) NotifyQuoteSent(context.Context, *ServiceOrder, *Quote) error { return nil }
```

This is the "porta de notificação" the source spec asks for, satisfied without a real e-mail
integration (requirements.md §4 item 10). `ServiceOrderService` gains a `notifier
QuoteNotifier` field; `NewServiceOrderService`'s new third parameter defaults to
`NoOpQuoteNotifier{}` when `nil` is passed, so every existing caller only needs to add one
argument. `SendQuote` calls `notifier.NotifyQuoteSent` **after** the send is already
committed, discarding its error deliberately (commented) — a notification failure must never
undo or block a send that has already durably happened.

### 1.5 Persistence

`quotes` gains three columns (`docs/schema.sql`):

```sql
version      INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
sent_at      TIMESTAMPTZ,
sent_version INTEGER,
CHECK (sent_at IS NULL OR sent_at >= generated_at),
CHECK (responded_at IS NULL OR sent_at IS NULL OR responded_at >= sent_at),
```

`service_order_status` gains `'CANCELED'`. `history_event` gains `'quote_sent'`;
`'approval'`/`'cancellation'` already existed in the enum (added ahead of time when
`specs/service-order-execution/` was designed) but had no producer until this feature.

`SaveQuote`'s upsert increments `version` on conflict
(`ON CONFLICT (service_order_id) DO UPDATE SET total_amount = EXCLUDED.total_amount, version
= quotes.version + 1`) — first insert leaves it at its `DEFAULT 1`.

Two new `ServiceOrderRepository` methods, both following the exact `pool.Begin` / `defer
tx.Rollback` / `tx.Commit`-at-the-end shape every other multi-statement write in this package
already uses (RNF07):

- **`SendQuote(ctx, order, quote) (*Quote, error)`**: `UPDATE service_orders SET status = $2
  WHERE id = $1 AND status = 'IN_DIAGNOSIS'` (same race-closing guard `StartDiagnosis`
  already uses — zero rows affected maps to `ErrInvalidStatusTransition`), then `UPDATE
  quotes SET sent_at = now(), sent_version = version WHERE id = $1 RETURNING sent_at,
  sent_version, version`, then insert a `quote_sent` history row. Commit only at the end.
- **`DecideQuote(ctx, order, quote, decision) (*Quote, error)`**: `UPDATE quotes SET status =
  $2, responded_at = now() WHERE id = $1 AND status = 'PENDING'` — zero rows affected (a
  concurrent or repeated decision) maps to `ErrQuoteAlreadyDecided` via a `pgx.ErrNoRows`
  check on the `RETURNING` scan; then `UPDATE service_orders SET status = $2 WHERE id = $1
  AND status = 'AWAITING_APPROVAL'` (same guard pattern, `ErrInvalidStatusTransition` on
  zero rows); then an `approval`/`cancellation` history row. Commit only at the end, so a
  failure at the history insert — or at either guarded `UPDATE` — rolls back every write
  already issued in this same transaction (requirements.md AC13). This was verified with an
  integration test that forces the *quote*-update guard to fail on a second decision attempt
  (`TestQuoteDecisionApproveThenRejectConflict`,
  `TestQuoteDecisionRepeatApproveConflict` in `internal/handlers_test/`) and asserts zero side
  effects from the rejected attempt — the same "trigger a real, guarded failure mid-transaction
  and assert nothing leaked" technique `service-order-opening`'s own
  `TestServiceOrderCreateRollsBackOnPartialFailure` already established, applied here to the
  natural failure point this transaction has (the guarded `UPDATE`) rather than an artificial
  one.

One new `serviceOrderLookups` method,
**`findServiceOrderByCodeWithTrackingToken(ctx, code, tokenHash) (*ServiceOrder, error)`**,
implemented against `service_orders`/`service_order_tracking_tokens` directly via
parameterized SQL — the same "read another feature's table via plain SQL instead of
importing its Go package" pattern `service-order-tracking`'s own repository already uses in
the other direction. Resolves by `code` first (`ErrServiceOrderNotFound` if unknown,
regardless of the token), then checks the token hash against that specific order's active
token (`ErrTrackingTokenInvalid` otherwise) — identical precedence to
`servicetracking.PostgresTrackingRepository.FindByCodeAndTokenHash`, so an unknown code is
never conflated with a wrong token, and a token issued for a different order is rejected
(requirements.md AC12).

### 1.6 Application layer

```go
func (s *ServiceOrderService) SendQuote(ctx, serviceOrderID) (*Quote, error)
func (s *ServiceOrderService) ApproveQuote(ctx, code int64, rawTrackingToken string) (*ServiceOrder, *Quote, error)
func (s *ServiceOrderService) RejectQuote(ctx, code int64, rawTrackingToken string) (*ServiceOrder, *Quote, error)
```

`SendQuote`: load the order; load its quote (`ErrQuoteNotFound` if none composed yet — this
*is* "orçamento incompleto", requirements.md AC1, since a quote cannot exist with zero items);
`validateQuoteItems(quote.Items)` as a defensive completeness check (the same validation
`ComposeQuote` already runs, reused rather than duplicated); `order.sendQuote()`; persist via
the repository; best-effort notify.

`ApproveQuote`/`RejectQuote` share a private `decideQuote(ctx, code, rawToken, decision)`:
reject an empty token immediately (`ErrTrackingTokenInvalid`, requirements.md §6); resolve the
order via `findServiceOrderByCodeWithTrackingToken` (hashing the raw token with the same
`internal/shared/trackingtoken.Hash` `service-order-tracking` uses); load the quote and check
`Status == QuoteStatusPending` (`ErrQuoteAlreadyDecided` otherwise — covers AC9/AC10 at the
application layer, ahead of the repository's own guard); call `order.approveQuote()` /
`order.rejectQuote()`; persist via `DecideQuote`. Returns both the updated order and quote,
since the public response reports both statuses.

### 1.7 API layer

New routes (`handler.go`, registered from `RegisterRoutes`):

```go
mux.Handle("POST /api/v1/service-orders/{id}/quote/send", wrap(handler.sendQuote))
mux.HandleFunc("POST /api/v1/acompanhamento/{codigo}/orcamento/aprovar", handler.approveQuote)
mux.HandleFunc("POST /api/v1/acompanhamento/{codigo}/orcamento/reprovar", handler.rejectQuote)
```

`sendQuote` returns the existing `QuoteResponse` (now including `version`/`sentAt`/
`sentVersion`) — an internal, administrative view, same as `GET .../quote`.

`approveQuote`/`rejectQuote` return a new, deliberately reduced public DTO
(`quoteDecisionResponse`: `code`, `quoteStatus`, `orderStatus`, `respondedAt`) rather than the
full `QuoteResponse` — no internal ids, no item breakdown — the same "public DTO distinct from
the administrative one" principle `service-order-tracking`'s `trackingResponse` already
established for this exact unauthenticated, customer-facing surface.

Error mapping (`httpsupport.go`), one addition to the existing `writeServiceError` switch:

| Error | Status | `error.code` |
| --- | --- | --- |
| `ErrTrackingTokenInvalid` | 401 | `INVALID_TRACKING_TOKEN` |

Every other error this feature can produce (`ErrQuoteNotFound`, `ErrEmptyQuote`,
`ErrInvalidStatusTransition`, `ErrQuoteAlreadyDecided`, `ErrServiceOrderNotFound`) already had
a mapping from `specs/service-order-diagnosis-quote/`/`specs/service-order-execution/` and is
reused as-is.

`trackingTokenHeader = "X-Tracking-Token"` is duplicated as a local constant in this
package's `handler.go` rather than imported from `service-order-tracking` — importing it would
be the exact cross-feature coupling `CLAUDE.md` §9.2 forbids for a single string literal.

## 2. Documentation updates

- `docs/entities.md`: `ServiceOrder.status`'s note and the `ServiceOrderStatus` enum gain
  `CANCELED`; `Quote` gains `version`/`sentAt`/`sentVersion`; `ServiceOrderHistory.event`'s
  enum description gains `quote_sent` and attributes `approval`/`cancellation` to this
  feature.
- `docs/seed.sql`: every quote whose order already reached `AWAITING_APPROVAL` or beyond
  gets a `sent_at`/`sent_version` consistent with that (it could only have gotten there by
  being sent); the one quote seeded against a still-`RECEIVED` order is left unsent.

## 3. Testing strategy

- **Unit tests** (`quote_model_test.go`): `sendQuote`/`approveQuote`/`rejectQuote` transition
  rules, replacing the deleted `markAwaitingApproval` tests.
- **Unit tests** (`quote_service_test.go`, extending `fake_repository_test.go`):
  `ComposeQuote`'s own success assertion updated to expect no order transition; `SendQuote`
  success (records `sentAt`/`sentVersion`, transitions to `AWAITING_APPROVAL`), rejects an
  incomplete quote, rejects sending twice, rejects an unknown order; `ApproveQuote`/
  `RejectQuote` success, missing/wrong/cross-order token, unknown code, approve-then-reject
  conflict, repeated-approve conflict.
- **Integration tests** (`internal/handlers_test/service_order_quote_decision_test.go`, new
  file, self-skips without `DATABASE_URL` per the existing convention): full send flow (DB
  status + history assertions), auth required for send, incomplete-quote/before-diagnosis
  rejections, full approve/reject flows (DB status + history assertions for both branches),
  missing/wrong/cross-order token (401, and order A's state verified untouched), unknown code
  (404), no admin JWT required, approve-then-reject and repeat-approve both 409 with zero
  additional history rows written — the RNF07 rollback proof (§1.5).
- Two existing integration tests in `service_order_test.go`
  (`TestServiceOrderComposeQuoteFullFlow`, `TestServiceOrderDetailByIDFullLifecycle`) were
  updated for the erratum in §1.2.

## 4. Traceability

Every decision above satisfies a specific `requirements.md` item; `tasks.md` breaks this
design into the ordered steps actually taken.
