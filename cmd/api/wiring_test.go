package main

import (
	"testing"

	"automotive-workshop-api/internal/features/service-order/adapter/notifier"
	"automotive-workshop-api/internal/shared/config"
)

func TestBuildQuoteNotifierWithoutKeyIsNoOp(t *testing.T) {
	got := buildQuoteNotifier(config.Config{}, nil)
	if _, ok := got.(notifier.NoOpQuoteNotifier); !ok {
		t.Fatalf("got %T, want NoOpQuoteNotifier", got)
	}
}

func TestBuildQuoteNotifierWithKeyIsResend(t *testing.T) {
	got := buildQuoteNotifier(config.Config{ResendAPIKey: "re_key", ResendFromEmail: "oficina@example.com"}, nil)
	if _, ok := got.(*notifier.ResendQuoteNotifier); !ok {
		t.Fatalf("got %T, want *ResendQuoteNotifier", got)
	}
}

// config.Load plus buildQuoteNotifier prove the env vars reach the
// composition, which an isolated handler test would not.
func TestComposeFromEnvironment(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")

	t.Setenv("RESEND_API_KEY", "")
	t.Setenv("RESEND_FROM_EMAIL", "")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := buildQuoteNotifier(cfg, nil).(notifier.NoOpQuoteNotifier); !ok {
		t.Fatal("unset env must compose the no-op notifier")
	}

	t.Setenv("RESEND_API_KEY", "re_key")
	t.Setenv("RESEND_FROM_EMAIL", "oficina@example.com")
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := buildQuoteNotifier(cfg, nil).(*notifier.ResendQuoteNotifier); !ok {
		t.Fatal("configured env must compose the Resend notifier")
	}
}

// TestBuildServiceOrderUseCases checks the composition root wires every use
// case and hands each external channel its own secret
// (specs/service-order-architecture-refactor/design.md §9.3). No method that
// reaches the repository is called, so a nil repository is enough.
func TestBuildServiceOrderUseCases(t *testing.T) {
	useCases := buildServiceOrderUseCases(nil, notifier.NoOpQuoteNotifier{}, "quote-secret", "status-secret")

	if useCases.Opening == nil || useCases.Lifecycle == nil || useCases.Quotes == nil || useCases.Executions == nil ||
		useCases.Queries == nil || useCases.StockUsage == nil || useCases.Metrics == nil {
		t.Fatalf("every use case must be composed: %+v", useCases)
	}
	if !useCases.Quotes.AuthenticateExternalNotification("quote-secret") {
		t.Fatal("the quote-decision secret must reach Quotes")
	}
	if !useCases.Lifecycle.AuthenticateExternalStatusUpdate("status-secret") {
		t.Fatal("the status-update secret must reach Lifecycle")
	}
	if useCases.Lifecycle.AuthenticateExternalStatusUpdate("quote-secret") {
		t.Fatal("the secrets must not be swapped")
	}
}
