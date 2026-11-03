// Package notifier holds the service-order feature's implementations of the
// usecase.QuoteNotifier port (specs/service-order-email-notification/): the
// Resend HTTP adapter and the no-op notifier wired when no e-mail provider
// is configured. cmd/api/wiring.go picks one from configuration.
package notifier
