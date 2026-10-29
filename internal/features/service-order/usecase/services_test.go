package usecase

// testServices bundles every use case built on one fakeRepository, so a test
// can drive a whole flow (compose -> send -> approve -> execute -> finalize)
// against one consistent in-memory store (design.md §9.4). Method names are
// unique across the embedded use cases, so each call is promoted unambiguously.
type testServices struct {
	*Opening
	*Lifecycle
	*Quotes
	*Executions
	*Queries
	*StockUsage
	*Metrics
}

func newTestServices(repo *fakeRepository, notifier QuoteNotifier) *testServices {
	return &testServices{
		Opening:   NewOpening(repo, repo, repo, repo, repo),
		Lifecycle: NewLifecycle(repo, repo, repo, repo),
		Quotes: NewQuotes(QuoteDependencies{
			Orders:        repo,
			TrackedOrders: repo,
			Quotes:        repo,
			Products:      repo,
			Catalog:       repo,
			Customers:     repo,
			Notifier:      notifier,
		}),
		Executions: NewExecutions(repo, repo, repo),
		Queries:    NewQueries(repo, repo, repo, repo, repo, repo),
		StockUsage: NewStockUsage(repo, repo),
		Metrics:    NewMetrics(repo),
	}
}

func newTestService(repo *fakeRepository) *testServices {
	return newTestServices(repo, nil)
}

// WithExternalNotificationSecret and WithExternalStatusUpdateSecret shadow the
// promoted methods so the secrets can be chained in either order.
func (services *testServices) WithExternalNotificationSecret(secret string) *testServices {
	services.Quotes.WithExternalNotificationSecret(secret)
	return services
}

func (services *testServices) WithExternalStatusUpdateSecret(secret string) *testServices {
	services.Lifecycle.WithExternalStatusUpdateSecret(secret)
	return services
}
