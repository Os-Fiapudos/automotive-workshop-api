# Tasks — Quote-Sent E-mail Notification (Resend)

## 1. Configuration
- [x] `config.Config` + `config.Load`: optional `RESEND_API_KEY` / `RESEND_FROM_EMAIL` pair.
- [x] `config_test.go` cases.

## 2. Port and lookup
- [x] `QuoteNotification` DTO; `QuoteNotifier` and `NoOpQuoteNotifier` updated.
- [x] `customerRef.Email`; `findCustomerByID` / `findCustomerByDocument` / `scanCustomerRef`.

## 3. Service
- [x] `SendQuote` → `notifyQuoteSent` (lookup, DTO, notify, PII-free log).

## 4. Adapter
- [x] `ResendQuoteNotifier` (HTTP, 5s timeout, no SDK, no link in the text).

## 5. Composition and ops
- [x] `cmd/api/wiring.go` `buildQuoteNotifier`; `main` uses it.
- [x] `.env.example`, `docker-compose.yml` (non-fail-fast).
- [x] `README.md`, `CLAUDE.md` §13, `specs/architecture.md` §7.

## 6. Tests
- [x] Adapter tests with a fake transport.
- [x] Service tests with a fake notifier (incl. log contents).
- [x] Wiring tests (`cmd/api/wiring_test.go`).

## 7. Verification
- [x] `go build ./...`, `go vet ./...`, `go test ./...`.
- [ ] `scripts/coverage.sh` against a real Postgres (RNF06).
- [ ] Demo evidence of a real send, only if Resend is adopted with a verified sender.
- [ ] PR review.
