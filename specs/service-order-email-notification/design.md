# Design — Quote-Sent E-mail Notification (Resend)

Satisfies: `requirements.md`. Replaces the earlier draft of this file (see the revision note
in `requirements.md`).

## 1. Port and DTO (`quote_notifier.go`)

```go
type QuoteNotification struct {
    OrderCode      int64
    TotalAmount    float64
    RecipientEmail string
}
type QuoteNotifier interface {
    NotifyQuoteSent(ctx context.Context, notification QuoteNotification) error
}
```

The DTO deliberately has no customer name and no token. `NoOpQuoteNotifier` stays the default.

## 2. Recipient lookup

`customerRef` gains `Email *string`; `findCustomerByID` / `findCustomerByDocument` select
`customers.email` and `scanCustomerRef` scans it (extends the existing query, no parallel
one). No schema change: `customers.email` already exists and is nullable.

## 3. `SendQuote` flow (`quote_service.go`)

After the repository commit, `notifyQuoteSent`:
1. `findCustomerByID(order.CustomerID)` — error → log `customer lookup failed`, return;
2. build the DTO; blank/nil e-mail → log `skipped`, return;
3. `notifier.NotifyQuoteSent` — error → log `send failed`; success → log `sent`.

Nothing is returned to `SendQuote`'s caller. Logs include the order code only.

## 4. Adapter (`resend_notifier.go`)

`ResendQuoteNotifier{apiKey, fromEmail, endpoint, httpClient}`; nil client → `Timeout: 5s`.
One `POST https://api.resend.com/emails` with `Authorization: Bearer`, JSON `from`, `to`,
`subject`, `text`. The text carries the order code and total (`R$ 450,50`) and asks the
customer to contact the workshop. Non-2xx → `ErrResendRejected` with the status only (the
response body may echo the address); transport/context errors are wrapped and returned.

## 5. Configuration and composition

`config.Config` gains `ResendAPIKey`, `ResendFromEmail`; `config.Load` rejects a key without
a sender. `cmd/api/wiring.go` `buildQuoteNotifier(cfg, httpClient)` returns the Resend
adapter when the key is set and `NoOpQuoteNotifier{}` otherwise; `main` calls it and logs the
"disabled" notice. `.env.example` lists both variables (empty); `docker-compose.yml` passes
them with `${VAR:-}` (not fail-fast).

## 6. Tests

- `resend_notifier_test.go`: fake `RoundTripper` — success (headers, recipient, no link),
  blank recipient, 4xx/5xx, transport error, canceled context.
- `quote_notification_test.go`: fake notifier — DTO after send, no e-mail, notifier failure,
  lookup failure, and log contents without PII.
- `config_test.go`: the key/sender combinations.
- `cmd/api/wiring_test.go`: composition, including `config.Load` from environment variables.
- Integration (`internal/handlers_test/`) keeps wiring a nil/no-op notifier, covering the
  unconfigured behavior of `SendQuote`.

## 7. Traceability

| Requirement | Section |
| --- | --- |
| BR1, BR2 | §2, §3 |
| BR3, BR4, BR5 | §3, §4 |
| BR6 | §3 (no retry/outbox) |
| BR7, BR8 | §4, §5 |
| AC1–AC8 | §3–§6 |
