package main

import (
	"testing"

	serviceorder "automotive-workshop-api/internal/features/service-order"
	"automotive-workshop-api/internal/shared/config"
)

func TestBuildQuoteNotifierWithoutKeyIsNoOp(t *testing.T) {
	notifier := buildQuoteNotifier(config.Config{}, nil)
	if _, ok := notifier.(serviceorder.NoOpQuoteNotifier); !ok {
		t.Fatalf("got %T, want NoOpQuoteNotifier", notifier)
	}
}

func TestBuildQuoteNotifierWithKeyIsResend(t *testing.T) {
	notifier := buildQuoteNotifier(config.Config{ResendAPIKey: "re_key", ResendFromEmail: "oficina@example.com"}, nil)
	if _, ok := notifier.(*serviceorder.ResendQuoteNotifier); !ok {
		t.Fatalf("got %T, want *ResendQuoteNotifier", notifier)
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
	if _, ok := buildQuoteNotifier(cfg, nil).(serviceorder.NoOpQuoteNotifier); !ok {
		t.Fatal("unset env must compose the no-op notifier")
	}

	t.Setenv("RESEND_API_KEY", "re_key")
	t.Setenv("RESEND_FROM_EMAIL", "oficina@example.com")
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := buildQuoteNotifier(cfg, nil).(*serviceorder.ResendQuoteNotifier); !ok {
		t.Fatal("configured env must compose the Resend notifier")
	}
}
