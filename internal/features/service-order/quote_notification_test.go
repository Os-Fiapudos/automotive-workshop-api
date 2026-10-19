package serviceorder

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingNotifier struct {
	calls []QuoteNotification
	err   error
}

func (notifier *recordingNotifier) NotifyQuoteSent(_ context.Context, notification QuoteNotification) error {
	notifier.calls = append(notifier.calls, notification)
	return notifier.err
}

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buffer bytes.Buffer
	log.SetOutput(&buffer)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buffer
}

// seedQuoteReadyOrder registers a customer (with the given e-mail) and an
// IN_DIAGNOSIS order of theirs with a composed quote, ready for SendQuote.
func seedQuoteReadyOrder(t *testing.T, repo *fakeRepository, service *ServiceOrderService, email *string) *ServiceOrder {
	t.Helper()
	customerID := uuid.New()
	repo.addCustomer(&customerRef{ID: customerID, Code: 1, Name: "Maria Secreta", Active: true, Email: email}, customerID.String())
	order := seedOrder(repo, StatusInDiagnosis)
	order.CustomerID = customerID
	composeQuoteForOrder(t, service, repo, order)
	return order
}

func TestSendQuoteNotifiesCustomerAfterSend(t *testing.T) {
	repo := newFakeRepository()
	notifier := &recordingNotifier{}
	service := NewServiceOrderService(repo, repo, notifier)
	email := "cliente@example.com"
	order := seedQuoteReadyOrder(t, repo, service, &email)
	logs := captureLog(t)

	quote, err := service.SendQuote(context.Background(), order.ID)
	require.NoError(t, err)

	require.Len(t, notifier.calls, 1)
	assert.Equal(t, QuoteNotification{OrderCode: order.Code, TotalAmount: quote.TotalAmount, RecipientEmail: email}, notifier.calls[0])
	assert.NotContains(t, logs.String(), email)
	assert.NotContains(t, logs.String(), "Maria Secreta")
}

func TestSendQuoteSkipsNotificationWithoutEmail(t *testing.T) {
	repo := newFakeRepository()
	notifier := &recordingNotifier{}
	service := NewServiceOrderService(repo, repo, notifier)
	blank := "  "
	for _, email := range []*string{nil, &blank} {
		order := seedQuoteReadyOrder(t, repo, service, email)
		_, err := service.SendQuote(context.Background(), order.ID)
		require.NoError(t, err)
	}
	assert.Empty(t, notifier.calls)
}

func TestSendQuoteSucceedsWhenNotifierFails(t *testing.T) {
	repo := newFakeRepository()
	notifier := &recordingNotifier{err: errors.New("provider down")}
	service := NewServiceOrderService(repo, repo, notifier)
	email := "cliente@example.com"
	order := seedQuoteReadyOrder(t, repo, service, &email)
	logs := captureLog(t)

	quote, err := service.SendQuote(context.Background(), order.ID)
	require.NoError(t, err)
	require.NotNil(t, quote.SentAt)
	assert.Equal(t, StatusAwaitingApproval, order.Status)
	assert.Contains(t, logs.String(), "send failed")
	assert.NotContains(t, logs.String(), email)
}

func TestSendQuoteSucceedsWhenCustomerLookupFails(t *testing.T) {
	repo := newFakeRepository()
	notifier := &recordingNotifier{}
	service := NewServiceOrderService(repo, repo, notifier)
	order := seedOrder(repo, StatusInDiagnosis) // customer not registered in the fake
	composeQuoteForOrder(t, service, repo, order)

	_, err := service.SendQuote(context.Background(), order.ID)
	require.NoError(t, err)
	assert.Empty(t, notifier.calls)
}
