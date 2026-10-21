package notifier

import (
	"automotive-workshop-api/internal/features/service-order/usecase"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

type capturedRequest struct {
	request *http.Request
	body    string
}

func fakeResendClient(status int, transportErr error, captured *[]capturedRequest) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(request.Body)
		*captured = append(*captured, capturedRequest{request: request, body: string(raw)})
		if transportErr != nil {
			return nil, transportErr
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"id":"x"}`))}, nil
	})}
}

func TestResendNotifierSendsOneEmail(t *testing.T) {
	var requests []capturedRequest
	notifier := NewResendQuoteNotifier("re_secret", "oficina@example.com", fakeResendClient(200, nil, &requests))

	err := notifier.NotifyQuoteSent(context.Background(), usecase.QuoteNotification{OrderCode: 42, TotalAmount: 450.5, RecipientEmail: "cliente@example.com"})
	require.NoError(t, err)

	require.Len(t, requests, 1)
	assert.Equal(t, http.MethodPost, requests[0].request.Method)
	assert.Equal(t, "https://api.resend.com/emails", requests[0].request.URL.String())
	assert.Equal(t, "Bearer re_secret", requests[0].request.Header.Get("Authorization"))

	var payload struct {
		From    string   `json:"from"`
		To      []string `json:"to"`
		Subject string   `json:"subject"`
		Text    string   `json:"text"`
	}
	require.NoError(t, json.Unmarshal([]byte(requests[0].body), &payload))
	assert.Equal(t, "oficina@example.com", payload.From)
	assert.Equal(t, []string{"cliente@example.com"}, payload.To)
	assert.Contains(t, payload.Subject, "#42")
	assert.Contains(t, payload.Text, "#42")
	assert.Contains(t, payload.Text, "R$ 450,50")
	assert.NotContains(t, strings.ToLower(payload.Text), "http")
	assert.NotContains(t, strings.ToLower(payload.Text), "token")
}

func TestResendNotifierSkipsWithoutRecipient(t *testing.T) {
	var requests []capturedRequest
	notifier := NewResendQuoteNotifier("re_secret", "oficina@example.com", fakeResendClient(200, nil, &requests))

	require.NoError(t, notifier.NotifyQuoteSent(context.Background(), usecase.QuoteNotification{OrderCode: 1, RecipientEmail: "  "}))
	assert.Empty(t, requests)
}

func TestResendNotifierProviderRejection(t *testing.T) {
	for _, status := range []int{400, 401, 422, 500} {
		var requests []capturedRequest
		notifier := NewResendQuoteNotifier("re_secret", "oficina@example.com", fakeResendClient(status, nil, &requests))

		err := notifier.NotifyQuoteSent(context.Background(), usecase.QuoteNotification{OrderCode: 1, RecipientEmail: "cliente@example.com"})
		require.ErrorIs(t, err, ErrResendRejected)
		assert.NotContains(t, err.Error(), "cliente@example.com")
	}
}

func TestResendNotifierTransportError(t *testing.T) {
	var requests []capturedRequest
	boom := errors.New("connection reset")
	notifier := NewResendQuoteNotifier("re_secret", "oficina@example.com", fakeResendClient(0, boom, &requests))

	err := notifier.NotifyQuoteSent(context.Background(), usecase.QuoteNotification{OrderCode: 1, RecipientEmail: "cliente@example.com"})
	require.ErrorIs(t, err, boom)
}

func TestResendNotifierCanceledContext(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return nil, request.Context().Err()
	})}
	notifier := NewResendQuoteNotifier("re_secret", "oficina@example.com", client)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := notifier.NotifyQuoteSent(ctx, usecase.QuoteNotification{OrderCode: 1, RecipientEmail: "cliente@example.com"})
	require.ErrorIs(t, err, context.Canceled)
}
