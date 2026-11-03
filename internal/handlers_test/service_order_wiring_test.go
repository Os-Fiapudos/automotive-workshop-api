package handlers_test

import (
	"automotive-workshop-api/internal/features/service-order/adapter/httpapi"
	orderpostgres "automotive-workshop-api/internal/features/service-order/adapter/postgres"
	"automotive-workshop-api/internal/features/service-order/usecase"
)

// newServiceOrderUseCases composes the service-order use cases the same way
// cmd/api/wiring.go's buildServiceOrderUseCases does, minus the e-mail
// notifier (specs/service-order-architecture-refactor/design.md §9.3). An
// empty secret leaves that external channel rejecting every caller.
func newServiceOrderUseCases(repository *orderpostgres.Repository, externalNotificationSecret, externalStatusUpdateSecret string) httpapi.UseCases {
	return httpapi.UseCases{
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
		}).WithExternalNotificationSecret(externalNotificationSecret),
		Executions: usecase.NewExecutions(repository, repository, repository),
		Queries:    usecase.NewQueries(repository, repository, repository, repository, repository, repository),
		StockUsage: usecase.NewStockUsage(repository, repository),
		Metrics:    usecase.NewMetrics(repository),
	}
}
