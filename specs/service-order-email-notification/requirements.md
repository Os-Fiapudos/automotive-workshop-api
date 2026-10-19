# Requirements — Quote-Sent E-mail Notification (Resend)

Status: **Approved for implementation** (F2-US14, PR #30)
Feature folder: `internal/features/service-order/` (implements the existing `QuoteNotifier`
port — no new feature package).

> **Revised for implementation.** The original draft assumed `order.Customer` is loaded,
> read env vars in `main`, and told the customer to use "the tracking link received when the
> order was opened". None holds: `ServiceOrder` only carries `CustomerID`, the card requires a
> testable composition, and the message must not promise a link or portal. The corrections
> are folded into the sections below.

## 1. Context

`specs/service-order-quote-decision/design.md` §1.4 defines the extension point this feature
fills: the `QuoteNotifier` port (`quote_notifier.go`), called by `SendQuote` after the
quote-sent write is committed. This feature adds a second implementation,
**`ResendQuoteNotifier`**, that actually sends an e-mail, wired instead of
`NoOpQuoteNotifier{}` when a Resend API key is configured. Resend is the PR's proposal; no
contract with the provider is presumed, and the port keeps the provider replaceable.

**The notification signature changed.** The port now takes an explicit DTO,
`QuoteNotification{OrderCode, TotalAmount, RecipientEmail}`, instead of `(*ServiceOrder,
*Quote)`. `ServiceOrder` has no customer data, so `SendQuote` resolves the recipient through
the existing `findCustomerByID` lookup and passes it in. This supersedes the
"signature unchanged" note in `specs/service-order-quote-decision/design.md` §1.4, which is
kept as the record of the earlier decision.

**Tracking link constraint.** Only a SHA-256 hash of each tracking token is stored
(`specs/service-order-tracking/design.md`); the raw token is shown once, at order creation.
`SendQuote` runs much later, so the e-mail **cannot** include a working link, and it never
tries to rebuild one from the hash. A link may only be added once a secure delivery/rotation
flow for such a token is specified.

## 2. User story

> As a customer, I want to be told when my quote is available, so that I know I need to
> evaluate the proposal.

## 3. Business rules

1. When and only when the order's customer has a non-blank e-mail (`customers.email` is
   nullable), `SendQuote` asks the notifier to send one e-mail via Resend's HTTP API.
2. **Customer without e-mail** (documented handling): no e-mail is sent, nothing fails, and
   the outcome is logged as skipped. No fallback channel exists.
3. The e-mail is informational: order code, "quote available" and the total. It has no
   tracking link or token, and does not promise a portal or any tracking feature that was
   not delivered; it asks the customer to contact the workshop.
4. Sending happens after the commit. A failure — customer lookup, provider error, network
   error, timeout — never undoes the send, never changes the order and never changes
   `SendQuote`'s response.
5. The operational result (`sent`, `skipped, customer has no e-mail`, `customer lookup
   failed`, `send failed`) is logged with the order code only. Logs never contain the API
   key, the recipient address, the customer name or any token.
6. There is **no retry and no outbox**: a failed notification is lost and only visible in the
   log. This is a known limitation, not an assumed capability.
7. Configuration comes from the environment and is read in `config.Load`:
   - neither `RESEND_API_KEY` nor `RESEND_FROM_EMAIL` set → notification disabled
     (`NoOpQuoteNotifier`), startup logs a one-line notice;
   - `RESEND_API_KEY` set without `RESEND_FROM_EMAIL` → startup fails;
   - both set → `ResendQuoteNotifier`.
8. The HTTP adapter lives outside the domain logic, uses an injected `http.Client` with a 5s
   default timeout, and no SDK / `go.mod` dependency is added.

## 4. Scope

In scope: the DTO and port change, `ResendQuoteNotifier`, `customerRef.Email` and the
queries that fill it, `config.Load` and `buildQuoteNotifier` (`cmd/api/wiring.go`),
`.env.example` / `docker-compose.yml` / docs.

Out of scope: other e-mails (order received, completed, delivered), templates beyond plain
text, inbound e-mail, tracking links, retries/outbox, a status-update endpoint.

## 5. Acceptance criteria

1. A customer with an e-mail and a valid configuration gets exactly one e-mail with the order
   code and total, and no link (tested with a fake transport).
2. A customer without e-mail triggers no provider call and `SendQuote` succeeds.
3. Provider errors (non-2xx, transport error, canceled context) leave `SendQuote` successful
   and the order `AWAITING_APPROVAL`.
4. With `RESEND_API_KEY` unset the app starts and `SendQuote` behaves as before.
5. A key without a sender fails `config.Load`.
6. No log line from this feature contains the key, the recipient address or the customer name.
7. The composition from environment variables is tested through `config.Load` +
   `buildQuoteNotifier`, not only at handler level.
8. `go build ./...`, `go vet ./...`, `go test ./...` pass.

## 6. Completion evidence

Unit and wiring tests with a simulated provider. Evidence of a real send in the demo only if
the team adopts Resend with a verified sender domain.
