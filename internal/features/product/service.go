package product

import (
	"context"

	"github.com/google/uuid"
)

// ProductService orchestrates product use cases on top of ProductRepository.
type ProductService struct {
	repository ProductRepository
}

// NewProductService initializes a new ProductService instance.
//
// Args:
//
//	repository(ProductRepository): repository implementation
//
// Returns:
//
//	service(*ProductService): initialized service
func NewProductService(repository ProductRepository) *ProductService {
	return &ProductService{repository: repository}
}

// Create validates and persists a new Product.
//
// Args:
//
//	ctx(context.Context): request context
//	code(*int64): optional manual code pointer
//	name(string): product display name
//	description(string): product description
//	unitPrice(float64): unit sale price
//	currentStock(int): initial stock quantity
//	productType(string): product type string ("PART" or "SUPPLY")
//
// Returns:
//
//	product(*Product): created product aggregate
//	err(error): error if validation or code pre-check fails
func (service *ProductService) Create(ctx context.Context, code *int64, name, description string, unitPrice float64, currentStock int, productType string) (*Product, error) {
	product, err := NewProduct(code, name, description, unitPrice, currentStock, productType)
	if err != nil {
		return nil, err
	}

	if code != nil && *code > 0 {
		exists, err := service.repository.ExistsByCode(ctx, *code, nil)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, ErrDuplicateCode
		}
	}

	if err := service.repository.Create(ctx, product); err != nil {
		return nil, err
	}
	return product, nil
}

// Get retrieves a product by its technical UUID identifier.
//
// Args:
//
//	ctx(context.Context): request context
//	id(uuid.UUID): product ID
//
// Returns:
//
//	product(*Product): product instance
//	err(error): ErrNotFound if not found
func (service *ProductService) Get(ctx context.Context, id uuid.UUID) (*Product, error) {
	return service.repository.FindByID(ctx, id)
}

// GetByCode retrieves a product by its sequential integer code.
//
// Args:
//
//	ctx(context.Context): request context
//	code(int64): product code
//
// Returns:
//
//	product(*Product): product instance
//	err(error): ErrNotFound if not found
func (service *ProductService) GetByCode(ctx context.Context, code int64) (*Product, error) {
	return service.repository.FindByCode(ctx, code)
}

// List retrieves a paginated list of products with optional type and status filtering.
//
// Args:
//
//	ctx(context.Context): request context
//	page(int): page number (1-based)
//	pageSize(int): items per page
//	rawType(*string): optional raw product type filter string
//	rawStatus(*string): optional status filter string
//
// Returns:
//
//	products([]*Product): list of products
//	total(int): total matching count
//	err(error): error if query fails
func (service *ProductService) List(ctx context.Context, page, pageSize int, rawType *string, rawStatus *string) ([]*Product, int, error) {
	var productType *Type
	if rawType != nil && *rawType != "" {
		parsedType, err := ParseType(*rawType)
		if err == nil {
			productType = &parsedType
		}
	}

	var status *Status
	if rawStatus != nil && *rawStatus != "" {
		st := Status(*rawStatus)
		status = &st
	}

	return service.repository.List(ctx, page, pageSize, productType, status)
}

// UpdateInput carries partial cadastral update fields for a product.
type UpdateInput struct {
	Name         *string
	Description  *string
	UnitPrice    *float64
	CurrentStock *int
	Type         *string
}

// Update applies partial cadastral updates to a product. Rejeita alterações diretas de estoque (RNF07).
//
// Args:
//
//	ctx(context.Context): request context
//	id(uuid.UUID): product ID
//	input(UpdateInput): input fields
//
// Returns:
//
//	product(*Product): updated product aggregate
//	err(error): error if validation or update fails
func (service *ProductService) Update(ctx context.Context, id uuid.UUID, input UpdateInput) (*Product, error) {
	if input.CurrentStock != nil {
		return nil, ErrStockDirectUpdateNotAllowed
	}

	product, err := service.repository.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := product.UpdateDetails(input.Name, input.Description, input.UnitPrice, input.Type); err != nil {
		return nil, err
	}

	if err := service.repository.Update(ctx, product); err != nil {
		return nil, err
	}
	return product, nil
}

// Deactivate logically deactivates a product (moves status to INACTIVE).
//
// Args:
//
//	ctx(context.Context): request context
//	id(uuid.UUID): product ID
//
// Returns:
//
//	product(*Product): deactivated product instance
//	err(error): error if product not found or update fails
func (service *ProductService) Deactivate(ctx context.Context, id uuid.UUID) (*Product, error) {
	product, err := service.repository.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	product.Deactivate()

	if err := service.repository.Update(ctx, product); err != nil {
		return nil, err
	}
	return product, nil
}

// AdjustStock validates and executes an entry or exit stock movement on a product aggregate.
//
// Args:
//
//	ctx(context.Context): request context
//	id(uuid.UUID): product ID
//	rawType(string): movement type ("ENTRY" or "EXIT")
//	quantity(int): positive quantity to adjust
//	reason(string): justification text for stock movement
//
// Returns:
//
//	product(*Product): updated product aggregate
//	movement(*StockMovement): created movement domain record
//	err(error): error if validation or stock update fails
func (service *ProductService) AdjustStock(ctx context.Context, id uuid.UUID, rawType string, quantity int, reason string) (*Product, *StockMovement, error) {
	movementType, err := ParseMovementType(rawType)
	if err != nil {
		return nil, nil, err
	}

	existingProduct, err := service.repository.FindByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	movement, err := existingProduct.ApplyStockAdjustment(movementType, quantity, reason)
	if err != nil {
		return nil, nil, err
	}

	delta := quantity
	if movementType == MovementTypeExit {
		delta = -quantity
	}

	updatedProduct, err := service.repository.AdjustStock(ctx, id, delta)
	if err != nil {
		return nil, nil, err
	}

	movement.PreviousStock = existingProduct.CurrentStock - delta
	movement.NewStock = updatedProduct.CurrentStock

	return updatedProduct, movement, nil
}

// GetStockBalance retrieves current stock balance information for a product.
//
// Args:
//
//	ctx(context.Context): request context
//	id(uuid.UUID): product ID
//
// Returns:
//
//	product(*Product): product instance
//	err(error): ErrNotFound if product does not exist
func (service *ProductService) GetStockBalance(ctx context.Context, id uuid.UUID) (*Product, error) {
	return service.repository.FindByID(ctx, id)
}
