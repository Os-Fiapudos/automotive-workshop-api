# Design — Quote-Sent E-mail Notification (Resend)

Satisfies: `requirements.md` (all sections). Implements the `QuoteNotifier` port
`specs/service-order-quote-decision/design.md` §1.4 already defines; does not change that
interface's signature.

If `specs/service-order-architecture-refactor/` has already landed, `ResendQuoteNotifier`
is an adapter (`adapter/resend/notifier.go` or alongside the other adapters) implementing
the `usecase.QuoteNotifier` port; if not, it is a new file in the current flat package
(`resend_notifier.go`), implementing the existing `QuoteNotifier` interface in
`quote_notifier.go`. Either way, the interface itself and the call site in `SendQuote` are
unchanged — only the wiring in `main.go` picks a different implementation.

## 1. `ResendQuoteNotifier` — direct HTTP call, no SDK

```go
package serviceorder // or adapter/resend, per the note above

type ResendQuoteNotifier struct {
    apiKey     string
    fromEmail  string
    httpClient *http.Client // injected, so tests substitute a fake RoundTripper
}

func NewResendQuoteNotifier(apiKey, fromEmail string, httpClient *http.Client) *ResendQuoteNotifier {
    if httpClient == nil {
        httpClient = &http.Client{Timeout: 5 * time.Second}
    }
    return &ResendQuoteNotifier{apiKey: apiKey, fromEmail: fromEmail, httpClient: httpClient}
}

func (n *ResendQuoteNotifier) NotifyQuoteSent(ctx context.Context, order *ServiceOrder, quote *Quote) error {
    if order.Customer == nil || order.Customer.Email == nil || *order.Customer.Email == "" {
        return nil // requirements.md BR1 — no address, nothing to do, not an error
    }

    body, _ := json.Marshal(map[string]any{
        "from":    n.fromEmail,
        "to":      []string{*order.Customer.Email},
        "subject": fmt.Sprintf("Seu orçamento da OS #%d está pronto", order.Code),
        "text": fmt.Sprintf(
            "Olá! O orçamento da sua ordem de serviço #%d está pronto, no valor de %s. "+
                "Acesse o link de acompanhamento que você recebeu ao abrir a ordem de "+
                "serviço para aprovar ou recusar.",
            order.Code, quote.TotalAmount, // formatted via money.Cents's own String()/MarshalJSON once specs/monetary-value-precision/ lands; plain float64 formatting until then
        ),
    })

    req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(body))
    if err != nil {
        return err
    }
    req.Header.Set("Authorization", "Bearer "+n.apiKey)
    req.Header.Set("Content-Type", "application/json")

    resp, err := n.httpClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    if resp.StatusCode >= 300 {
        return fmt.Errorf("resend: unexpected status %d", resp.StatusCode)
    }
    return nil
}
```

No `resend-go` (or any other) SDK import — a single, small HTTP call, matching the
recommendation in `requirements.md` §1 and this project's "don't add a dependency without
justified need" posture (`CLAUDE.md` §12). The error this method returns is still discarded
by its caller (`SendQuote`, per `specs/service-order-quote-decision/design.md` §1.4,
unchanged) — returning a real `error` value here (rather than always `nil`) is what lets
this type be unit-tested for its failure path (`requirements.md` AC3) even though the
caller never inspects it.

## 2. `customerRef.Email` (requirements.md §4)

```go
type customerRef struct {
    ID       uuid.UUID
    Code     int64
    Name     string
    Active   bool
    Document string
    Phone    string
    Email    *string // added by this feature
}
```

The query `SendQuote`'s call path uses to resolve `order.Customer` (already loaded before
`SendQuote` calls `notifier.NotifyQuoteSent`, per the existing flow) is extended to select
`customers.email` alongside the columns it already selects — same "extend the existing
query, don't add a parallel one" pattern `specs/service-order-query/design.md` §1.6 already
used for `Document`/`Phone` on this exact struct.

## 3. Wiring (`cmd/api/main.go`)

```go
var quoteNotifier serviceorder.QuoteNotifier = serviceorder.NoOpQuoteNotifier{}
if resendAPIKey := os.Getenv("RESEND_API_KEY"); resendAPIKey != "" {
    fromEmail := os.Getenv("RESEND_FROM_EMAIL")
    if fromEmail == "" {
        log.Fatal("RESEND_FROM_EMAIL is required when RESEND_API_KEY is set")
    }
    quoteNotifier = serviceorder.NewResendQuoteNotifier(resendAPIKey, fromEmail, nil)
} else {
    log.Println("RESEND_API_KEY not set — quote-sent e-mail notifications are disabled")
}

serviceOrderService := serviceorder.NewServiceOrderService(serviceOrderRepository, serviceOrderRepository, quoteNotifier)
```

`RESEND_API_KEY` unset is not fatal (`requirements.md` BR4); `RESEND_API_KEY` set without
`RESEND_FROM_EMAIL` *is* fatal — a half-configured integration (a key but no sender address)
is a configuration mistake worth catching at startup, not a legitimate "disabled" state like
having neither set at all.

## 4. Configuration

`.env.example` gains:
```
RESEND_API_KEY=
RESEND_FROM_EMAIL=
```
both empty, grouped under the same `# Fase 2 — external integrations` comment
`specs/service-order-quote-decision/design.md` §5.4 and
`specs/service-order-status-update/design.md` §4 already introduced there.
`docker-compose.yml`'s `api` service environment list gains both — **not** with the
fail-fast `${VAR:?...}` form the other Fase 2 secrets use (`specs/containerization/design.md`
§3), since neither is required (`requirements.md` BR4): `RESEND_API_KEY: ${RESEND_API_KEY:-}`
(defaults to empty, not an error).

## 5. Testing strategy

- **Unit** (`resend_notifier_test.go`, new): `NotifyQuoteSent` with a fake
  `http.RoundTripper` —
  success (one request made, correct `Authorization`/body/recipient), no customer e-mail
  (zero requests made, `nil` returned, AC2), Resend returns a non-2xx status (`error`
  returned, not panicked, AC3), network error from the transport (`error` returned).
- **Integration** (`internal/handlers_test/service_order_quote_decision_test.go`, extended):
  one case with `RESEND_API_KEY` unset — confirms `SendQuote` behaves identically to before
  this feature (AC4); this is the one integration test that actually exercises the fallback
  wiring in `main.go`, not just the unit-level `NoOpQuoteNotifier` behavior.
- No test calls the real Resend API — every test substitutes the `http.Client`'s transport
  (§1's `httpClient` injection point exists specifically so tests never make a real network
  call, consistent with every other test in this project running without external network
  access).

## 6. Traceability

| Requirement | Design section |
| --- | --- |
| BR1 (no e-mail on file → no-op) | §1 |
| BR2 (informational only, no tracking link) | §1 |
| BR3 (failure never propagates/blocks, no PII in logs) | §1, §5 |
| BR4 (config from env, non-fatal when unset) | §3 |
| BR5 (SendQuote unchanged) | §1 (same interface, same call site) |
| AC1–AC6 | §1, §2, §3, §5 |
