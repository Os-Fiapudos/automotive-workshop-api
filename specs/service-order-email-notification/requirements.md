# Requirements — Quote-Sent E-mail Notification (Resend)

Status: **Approved for implementation**
Feature folder: `internal/features/service-order/` (implements the existing `QuoteNotifier`
port — no new feature package).

## 1. Context

Source: this repo's Fase 2 planning conversation (2026-10-12), team decision #4 — "Dá pra
usar o Resend," given with no stated preference between Resend's official SDK and calling
its HTTP API directly; this spec's recommendation (not objected to when raised) is calling
the HTTP API directly, no new `go.mod` dependency (`design.md` §1).

`specs/service-order-quote-decision/design.md` §1.4 already defines the extension point this
feature fills: a `QuoteNotifier` interface (`internal/features/service-order/
quote_notifier.go`), called by `SendQuote` on a best-effort basis after the quote-sent write
is already committed, with `NoOpQuoteNotifier{}` wired as the default in `cmd/api/main.go:69`
(confirmed in the current code — `grep` shows this exact wiring already in place). This
feature adds a second implementation of that same interface,
**`ResendQuoteNotifier`**, that actually sends an e-mail, and makes it the default wiring
instead of `NoOpQuoteNotifier{}` when a Resend API key is configured.

This feature was missing from this repo's broader Fase 2 spec set even though three
infrastructure specs (`specs/containerization/`, `specs/kubernetes-deployment/`,
`specs/ci-cd-pipeline/`) already reference a `RESEND_API_KEY` secret — those specs describe
*where the secret lives*, not what consumes it; this spec is the missing piece that actually
uses it.

**Important constraint discovered while designing this feature** (not previously
documented): `specs/service-order-tracking/design.md` stores only a **SHA-256 hash** of each
order's tracking token (`service_order_tracking_tokens`), by design — the raw token is
returned to the caller exactly once, in `POST /api/v1/service-orders`'s own creation
response, and is never persisted or retrievable afterward
(`specs/service-order-opening/design.md` §4.2's cross-reference: "returned only here,
once — never on any other response, and never logged"). `SendQuote` runs long after order
creation (after diagnosis and quote composition), so **this feature's e-mail cannot include
a working tracking link** — the system has no way to reconstruct the raw token at send time.
This is not a bug to route around; it is the deliberate security property
`service-order-tracking` was built for (a token that can be regenerated/re-shown on demand
would defeat the "shown once" guarantee). The e-mail is informational only (§3).

## 2. User story

> As a customer whose quote has just been sent,
> I want to receive an e-mail saying so,
> so that I know to go check it, without needing the workshop to call me.

## 3. Business rules

1. `ResendQuoteNotifier.NotifyQuoteSent` sends one e-mail via Resend's HTTP API to the
   order's customer, when and only when that customer has an e-mail on file
   (`customers.email` is nullable, `internal/features/customer/model.go`'s `Customer.Email
   *string`) — no e-mail on file is not an error, the call simply does nothing (consistent
   with `NoOpQuoteNotifier`'s own "no real integration required" fallback behavior for that
   case).
2. The e-mail is informational only: order code, a short status message ("seu orçamento
   está pronto"), and the total amount. It does **not** include a tracking link or the
   tracking token — per §1's discovered constraint, neither is available at this point in
   the flow.
3. A failure to send (Resend API error, network failure, missing configuration) is
   **never** propagated as an error from `SendQuote` and never blocks or undoes the
   already-committed quote-sent transition — `specs/service-order-quote-decision/
   requirements.md` §4 item 10's existing rule, now exercised by a real implementation
   instead of a no-op. The failure is logged (never the API key, never the customer's
   e-mail address or name — `CLAUDE.md` §13's "never log sensitive data" rule, applied here
   to PII the same way it's applied to secrets).
4. Resend configuration (`RESEND_API_KEY`, `RESEND_FROM_EMAIL`) comes from the environment,
   same as every other secret/config value (`CLAUDE.md` §13). Unlike
   `EXTERNAL_NOTIFICATION_SECRET`/`EXTERNAL_STATUS_UPDATE_SECRET` (which gate inbound,
   security-sensitive endpoints and must fail the whole process at startup if missing), a
   missing Resend configuration is **not** fatal to startup — `cmd/api/main.go` falls back to
   `NoOpQuoteNotifier{}` with a one-line startup log noting e-mail notification is disabled.
   This is a deliberate difference from the other two secrets, not an inconsistency: this
   integration is explicitly best-effort/non-critical (§3 item 3, and
   `specs/service-order-quote-decision/requirements.md` §4 item 10's pre-existing framing),
   so the project should still run, fully functional otherwise, in an environment that
   hasn't configured Resend yet (e.g. a fresh CI run before secrets are provisioned).
5. No change to `SendQuote`'s own transaction, return value, or error handling —
   `NotifyQuoteSent` is still called exactly where `specs/service-order-quote-decision/
   design.md` §1.4 already calls it, after the commit, with its error discarded. This
   feature only changes *which* `QuoteNotifier` implementation is wired in.

## 4. Scope

In scope:
- `ResendQuoteNotifier` implementing the existing `QuoteNotifier` interface.
- `customerRef` (`internal/features/service-order/repository.go`) gains `Email *string`,
  and the query that populates it for the order being sent is extended to select it —
  needed so `NotifyQuoteSent` knows who to send to without importing the `customer` feature
  (`CLAUDE.md` §9.2, same pattern `Document`/`Phone` already follow there).
- `cmd/api/main.go` wiring: `ResendQuoteNotifier` when `RESEND_API_KEY` is set,
  `NoOpQuoteNotifier{}` otherwise (§3 item 4).
- `.env.example` / `docker-compose.yml` entries for `RESEND_API_KEY`/`RESEND_FROM_EMAIL`.

Out of scope:
- Any other transactional e-mail (order received, execution finished, delivery ready) —
  only the quote-sent notification is specified anywhere in this project's requirements;
  adding others would be inventing scope (`CLAUDE.md` §17).
- E-mail templates beyond plain text/minimal HTML — no design system, no branding assets
  exist in this project to template against.
- Any inbound e-mail handling — covered, to the extent it's in scope at all, by
  `specs/service-order-status-update/`, not this feature.

## 5. Acceptance criteria

1. `ResendQuoteNotifier.NotifyQuoteSent`, given an order whose customer has an e-mail on
   file and a valid Resend configuration, sends exactly one e-mail containing the order code
   and total amount, no tracking link.
2. Given a customer with no e-mail on file, `NotifyQuoteSent` returns `nil` without calling
   Resend.
3. Given a Resend API failure (simulated via a fake HTTP transport in tests), `SendQuote`
   still succeeds and returns the quote normally — the notification failure is invisible to
   the caller.
4. With `RESEND_API_KEY` unset, the application starts successfully (no fatal error) and
   `SendQuote` behaves exactly as it did with `NoOpQuoteNotifier{}` before this feature.
5. No log line produced by this feature contains the Resend API key, the customer's e-mail
   address, or the customer's name.
6. `go build ./...`, `go vet ./...`, `go test ./...` pass.

## 6. Out of scope note — tracking link limitation

Flagged explicitly (§1) rather than worked around: if a future requirement wants the e-mail
to include a live tracking link, that requires its own design decision (e.g. a short-lived,
single-purpose link token minted specifically for this e-mail, distinct from the
order-creation tracking token) — not assumed or stubbed here.
