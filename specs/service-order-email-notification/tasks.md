# Tasks — Quote-Sent E-mail Notification (Resend)

## 1. Domain/read-model (`design.md` §2)

- [ ] `customerRef` gains `Email *string`; the query `SendQuote` uses to resolve the order's
      customer selects `customers.email` too.

## 2. Notifier implementation (`design.md` §1)

- [ ] `ResendQuoteNotifier` implementing `QuoteNotifier`: no customer e-mail → no-op;
      builds and sends the Resend HTTP request; non-2xx or transport error → returned as
      `error` (still discarded by `SendQuote`, unchanged).
- [ ] Confirm message content has no tracking link (`requirements.md` §1/BR2 constraint).

## 3. Wiring (`design.md` §3, §4)

- [ ] `cmd/api/main.go`: read `RESEND_API_KEY`/`RESEND_FROM_EMAIL`; wire
      `ResendQuoteNotifier` when the key is set (fatal if the key is set but the from-address
      isn't), `NoOpQuoteNotifier{}` with a log line otherwise.
- [ ] `.env.example`: add `RESEND_API_KEY=`, `RESEND_FROM_EMAIL=`.
- [ ] `docker-compose.yml`: add both to the `api` service's environment, non-fail-fast form
      (`design.md` §4 — differs from `specs/containerization/`'s other Fase 2 secrets, which
      stay fail-fast).

## 4. Tests (`design.md` §5)

- [ ] Unit tests with a fake `http.RoundTripper`: success, no-email no-op, non-2xx response,
      transport error.
- [ ] Integration test: `RESEND_API_KEY` unset → `SendQuote` behaves identically to the
      pre-feature `NoOpQuoteNotifier` wiring.

## 5. Verification

- [ ] `go build ./...`, `go vet ./...`, `go test ./...` pass.
- [ ] Confirm no log line in this feature's code contains the API key, customer e-mail, or
      customer name (manual code read, `requirements.md` AC5 — not mechanically enforced).
- [ ] Re-check `requirements.md` AC1–AC6.
