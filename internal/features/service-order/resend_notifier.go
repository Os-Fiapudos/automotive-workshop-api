package serviceorder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	resendEndpoint       = "https://api.resend.com/emails"
	resendDefaultTimeout = 5 * time.Second
)

// ErrResendRejected is returned when the provider answers with a non-2xx
// status. The response body is deliberately not included: it may echo the
// recipient address.
var ErrResendRejected = errors.New("e-mail provider rejected the request")

// ResendQuoteNotifier is the QuoteNotifier adapter that sends the quote-sent
// e-mail through Resend's HTTP API (no SDK — a single request). The HTTP
// client is injected so tests substitute the transport.
type ResendQuoteNotifier struct {
	apiKey     string
	fromEmail  string
	endpoint   string
	httpClient *http.Client
}

// NewResendQuoteNotifier builds the adapter. A nil httpClient gets a 5s timeout.
func NewResendQuoteNotifier(apiKey, fromEmail string, httpClient *http.Client) *ResendQuoteNotifier {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: resendDefaultTimeout}
	}
	return &ResendQuoteNotifier{apiKey: apiKey, fromEmail: fromEmail, endpoint: resendEndpoint, httpClient: httpClient}
}

// NotifyQuoteSent sends one informational e-mail. It contains the order code
// and total only — no tracking link or token, and no promise of a portal.
func (notifier *ResendQuoteNotifier) NotifyQuoteSent(ctx context.Context, notification QuoteNotification) error {
	if strings.TrimSpace(notification.RecipientEmail) == "" {
		return nil
	}

	total := strings.Replace(fmt.Sprintf("%.2f", notification.TotalAmount), ".", ",", 1)
	body, err := json.Marshal(map[string]any{
		"from":    notifier.fromEmail,
		"to":      []string{notification.RecipientEmail},
		"subject": fmt.Sprintf("Orçamento da OS #%d disponível", notification.OrderCode),
		"text": fmt.Sprintf("O orçamento da sua ordem de serviço #%d está disponível, no valor de R$ %s. "+
			"Entre em contato com a oficina para avaliá-lo.", notification.OrderCode, total),
	})
	if err != nil {
		return fmt.Errorf("encoding e-mail request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, notifier.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building e-mail request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+notifier.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := notifier.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("sending e-mail request: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))

	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("%w: status %d", ErrResendRejected, response.StatusCode)
	}
	return nil
}
