package postgres

import "automotive-workshop-api/internal/features/service-order/usecase"

// Compile-time proof that Repository satisfies every persistence port the
// use cases declare (specs/service-order-architecture-refactor/design.md
// §5 step 3, §9.2). A port that drifts from its implementation breaks the
// build here, next to the SQL, instead of at a distant call site.
var (
	_ usecase.OrderCreator           = (*Repository)(nil)
	_ usecase.OrderFinder            = (*Repository)(nil)
	_ usecase.OrderCodeFinder        = (*Repository)(nil)
	_ usecase.TrackedOrderFinder     = (*Repository)(nil)
	_ usecase.StatusTransitionStore  = (*Repository)(nil)
	_ usecase.CustomerLookup         = (*Repository)(nil)
	_ usecase.VehicleLookup          = (*Repository)(nil)
	_ usecase.ServiceCatalogLookup   = (*Repository)(nil)
	_ usecase.ProductLookup          = (*Repository)(nil)
	_ usecase.QuoteReader            = (*Repository)(nil)
	_ usecase.QuoteStore             = (*Repository)(nil)
	_ usecase.ExecutionStore         = (*Repository)(nil)
	_ usecase.ExecutionLister        = (*Repository)(nil)
	_ usecase.ServiceOrderQueryStore = (*Repository)(nil)
	_ usecase.StockLedger            = (*Repository)(nil)
	_ usecase.ExecutionMetricsReader = (*Repository)(nil)
)
