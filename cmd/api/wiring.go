package main

import (
	"net/http"

	serviceorder "automotive-workshop-api/internal/features/service-order"
	"automotive-workshop-api/internal/features/service-order/usecase"
	"automotive-workshop-api/internal/shared/config"
)

// buildQuoteNotifier picks the quote-sent notifier from configuration: the
// Resend adapter when RESEND_API_KEY is set (config.Load already guarantees a
// sender is set with it), otherwise the no-op notifier. Extracted from main so
// the composition is testable.
func buildQuoteNotifier(configuration config.Config, httpClient *http.Client) usecase.QuoteNotifier {
	if configuration.ResendAPIKey == "" {
		return serviceorder.NoOpQuoteNotifier{}
	}
	return serviceorder.NewResendQuoteNotifier(configuration.ResendAPIKey, configuration.ResendFromEmail, httpClient)
}

// buildServiceOrderUseCases composes the service-order use cases: the one
// Postgres repository satisfies every persistence port, each constructor
// receiving it through the narrow port it declares
// (specs/service-order-architecture-refactor/design.md §9.3).
func buildServiceOrderUseCases(repository *serviceorder.PostgresServiceOrderRepository, notifier usecase.QuoteNotifier, externalNotificationSecret, externalStatusUpdateSecret string) serviceorder.UseCases {
	return serviceorder.UseCases{
		Opening: usecase.NewOpening(repository, repository, repository, repository, repository),
		Lifecycle: usecase.NewLifecycle(repository, repository, repository, repository).
			WithExternalStatusUpdateSecret(externalStatusUpdateSecret),
		Quotes: usecase.NewQuotes(usecase.QuoteDependencies{
			Orders:        repository,
			TrackedOrders: repository,
			Quotes:        repository,
			Products:      repository,
			Catalog:       repository,
			Customers:     repository,
			Notifier:      notifier,
		}).WithExternalNotificationSecret(externalNotificationSecret),
		Executions: usecase.NewExecutions(repository, repository, repository),
		Queries:    usecase.NewQueries(repository, repository, repository, repository, repository, repository),
		StockUsage: usecase.NewStockUsage(repository, repository),
		Metrics:    usecase.NewMetrics(repository),
	}
}
