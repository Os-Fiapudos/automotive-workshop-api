package serviceorder

import "automotive-workshop-api/internal/features/service-order/domain"

// QuoteItemInput is a raw, unresolved line item as supplied by a compose
// request — mirrors CreateInput's "raw strings in, resolved refs out" shape
// (see ServiceOrderService.Create).
type QuoteItemInput struct {
	Kind      domain.QuoteItemKind
	ProductID string
	ServiceID string
	Quantity  int
}

// StockUsageItem is one raw line of a usage-deduction request: a product and
// the quantity taken from it (specs/service-order-stock-usage/requirements.md
// §3, BR3).
type StockUsageItem struct {
	ProductID string
	Quantity  int
}

// validateStockUsageItems enforces the business rules that don't need
// repository access: at least one item (BR7 implies a request has items to
// roll back), every quantity strictly positive (BR3). Existence, active
// status, and balance are enforced atomically by the repository
// (specs/service-order-stock-usage/design.md §3) to avoid a check-then-act
// race with a concurrent request (BR8).
func validateStockUsageItems(items []StockUsageItem) error {
	if len(items) == 0 {
		return domain.ErrEmptyStockUsage
	}
	for _, item := range items {
		if item.Quantity <= 0 {
			return domain.ErrInvalidQuantity
		}
	}
	return nil
}
