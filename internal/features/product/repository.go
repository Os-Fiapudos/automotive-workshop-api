package product

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ProductRepository defines persistence operations for the Product aggregate.
type ProductRepository interface {
	Create(ctx context.Context, product *Product) error
	FindByID(ctx context.Context, id uuid.UUID) (*Product, error)
	FindByCode(ctx context.Context, code int64) (*Product, error)
	ExistsByCode(ctx context.Context, code int64, excludeID *uuid.UUID) (bool, error)
	List(ctx context.Context, page, pageSize int, productType *Type, status *Status) ([]*Product, int, error)
	Update(ctx context.Context, product *Product) error
	AdjustStock(ctx context.Context, id uuid.UUID, movement *StockMovement) (*Product, error)
	ListMovements(ctx context.Context, productID uuid.UUID, page, pageSize int) ([]*StockMovement, int, error)
	IsUsedInQuotesOrOrders(ctx context.Context, id uuid.UUID) (bool, error)
}

const postgresUniqueViolation = "23505"
const codeUniqueConstraint = "ux_products_code"

const productColumns = `id, code, name, description, unit_price, current_stock, type, status, created_at, updated_at`

// PostgresProductRepository implements ProductRepository against PostgreSQL via pgx.
type PostgresProductRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresProductRepository creates a PostgresProductRepository instance.
//
// Args:
//
//	pool(*pgxpool.Pool): PostgreSQL connection pool
//
// Returns:
//
//	repository(*PostgresProductRepository): initialized repository
func NewPostgresProductRepository(pool *pgxpool.Pool) *PostgresProductRepository {
	return &PostgresProductRepository{pool: pool}
}

// Create inserts a new product record in PostgreSQL.
//
// Args:
//
//	ctx(context.Context): request context
//	product(*Product): product domain aggregate
//
// Returns:
//
//	err(error): nil on success or domain error
func (repository *PostgresProductRepository) Create(ctx context.Context, product *Product) error {
	var err error
	if product.Code > 0 {
		query := `
			INSERT INTO products (id, code, name, description, unit_price, current_stock, type, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING code, created_at, updated_at`
		err = repository.pool.QueryRow(ctx, query,
			product.ID, product.Code, product.Name, product.Description, product.UnitPrice, product.CurrentStock, string(product.Type), string(product.Status),
		).Scan(&product.Code, &product.CreatedAt, &product.UpdatedAt)
	} else {
		query := `
			INSERT INTO products (id, name, description, unit_price, current_stock, type, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING code, created_at, updated_at`
		err = repository.pool.QueryRow(ctx, query,
			product.ID, product.Name, product.Description, product.UnitPrice, product.CurrentStock, string(product.Type), string(product.Status),
		).Scan(&product.Code, &product.CreatedAt, &product.UpdatedAt)
	}

	return mapUniqueViolation(err)
}

// FindByID retrieves a product by its technical UUID identifier.
//
// Args:
//
//	ctx(context.Context): request context
//	id(uuid.UUID): technical product ID
//
// Returns:
//
//	product(*Product): product instance
//	err(error): ErrNotFound if not found
func (repository *PostgresProductRepository) FindByID(ctx context.Context, id uuid.UUID) (*Product, error) {
	row := repository.pool.QueryRow(ctx, `SELECT `+productColumns+` FROM products WHERE id = $1`, id)
	return scanProduct(row)
}

// FindByCode retrieves a product by its sequential integer code.
//
// Args:
//
//	ctx(context.Context): request context
//	code(int64): sequential code
//
// Returns:
//
//	product(*Product): product instance
//	err(error): ErrNotFound if not found
func (repository *PostgresProductRepository) FindByCode(ctx context.Context, code int64) (*Product, error) {
	row := repository.pool.QueryRow(ctx, `SELECT `+productColumns+` FROM products WHERE code = $1`, code)
	return scanProduct(row)
}

// ExistsByCode checks whether a product with the given code already exists.
//
// Args:
//
//	ctx(context.Context): request context
//	code(int64): product code
//	excludeID(*uuid.UUID): optional ID to exclude from query (for updates)
//
// Returns:
//
//	exists(bool): true if code exists
//	err(error): database error if any
func (repository *PostgresProductRepository) ExistsByCode(ctx context.Context, code int64, excludeID *uuid.UUID) (bool, error) {
	var exists bool
	var err error

	if excludeID != nil {
		err = repository.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM products WHERE code = $1 AND id <> $2)`,
			code, *excludeID,
		).Scan(&exists)
	} else {
		err = repository.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM products WHERE code = $1)`,
			code,
		).Scan(&exists)
	}

	return exists, err
}

// List retrieves a paginated list of products with optional type and status filtering.
//
// Args:
//
//	ctx(context.Context): request context
//	page(int): 1-based page index
//	pageSize(int): items per page
//	productType(*Type): optional filter by PART or SUPPLY
//	status(*Status): optional filter by ACTIVE or INACTIVE
//
// Returns:
//
//	products([]*Product): slice of products
//	total(int): total matching count
//	err(error): database error if any
func (repository *PostgresProductRepository) List(ctx context.Context, page, pageSize int, productType *Type, status *Status) ([]*Product, int, error) {
	offset := (page - 1) * pageSize

	query := `SELECT ` + productColumns + `, COUNT(*) OVER() AS total FROM products WHERE 1=1`
	args := []any{}
	argIdx := 1

	if productType != nil {
		query += ` AND type = $` + strconvIdx(&argIdx)
		args = append(args, string(*productType))
	}

	if status != nil {
		query += ` AND status = $` + strconvIdx(&argIdx)
		args = append(args, string(*status))
	}

	query += ` ORDER BY code LIMIT $` + strconvIdx(&argIdx) + ` OFFSET $` + strconvIdx(&argIdx)
	args = append(args, pageSize, offset)

	rows, err := repository.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var products []*Product
	total := 0
	for rows.Next() {
		product := &Product{}
		var typeStr, statusStr string
		if err := rows.Scan(
			&product.ID, &product.Code, &product.Name, &product.Description, &product.UnitPrice, &product.CurrentStock, &typeStr, &statusStr,
			&product.CreatedAt, &product.UpdatedAt, &total,
		); err != nil {
			return nil, 0, err
		}
		product.Type = Type(typeStr)
		product.Status = Status(statusStr)
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return products, total, nil
}

// Update persists changes to product details in PostgreSQL.
//
// Args:
//
//	ctx(context.Context): request context
//	product(*Product): updated product aggregate
//
// Returns:
//
//	err(error): nil on success or error
func (repository *PostgresProductRepository) Update(ctx context.Context, product *Product) error {
	const query = `
		UPDATE products
		SET name = $2, description = $3, unit_price = $4, type = $5, status = $6
		WHERE id = $1
		RETURNING updated_at`

	err := repository.pool.QueryRow(ctx, query,
		product.ID, product.Name, product.Description, product.UnitPrice, string(product.Type), string(product.Status),
	).Scan(&product.UpdatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return mapUniqueViolation(err)
}

// AdjustStock applies a positive (ENTRY) or negative (EXIT) stock adjustment
// and persists movement to the shared stock_movements ledger
// (specs/service-order-stock-usage/design.md §6 — the same table
// internal/features/service-order writes its own usage deductions to,
// distinguished by a NULL service_order_id here), atomically in one
// transaction (RNF07).
//
// Args:
//
//	ctx(context.Context): request context
//	id(uuid.UUID): product ID
//	movement(*StockMovement): the movement to apply and persist — Type/Quantity/Reason
//	    already set by the caller (Product.ApplyStockAdjustment); PreviousStock/NewStock/
//	    CreatedAt are filled in by this call from the actual database values.
//
// Returns:
//
//	product(*Product): updated product aggregate
//	err(error): error if product not found, inactive, or stock insufficient
func (repository *PostgresProductRepository) AdjustStock(ctx context.Context, id uuid.UUID, movement *StockMovement) (*Product, error) {
	delta := movement.Quantity
	if movement.Type == MovementTypeExit {
		delta = -movement.Quantity
	}

	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once Commit succeeds

	const query = `
		UPDATE products
		SET current_stock = current_stock + $2, updated_at = now()
		WHERE id = $1 AND status = 'ACTIVE' AND (current_stock + $2) >= 0
		RETURNING ` + productColumns

	row := tx.QueryRow(ctx, query, id, delta)
	product, err := scanProduct(row)
	if errors.Is(err, ErrNotFound) {
		var currentStock int
		var status string
		checkErr := tx.QueryRow(ctx, `SELECT current_stock, status FROM products WHERE id = $1`, id).Scan(&currentStock, &status)
		if errors.Is(checkErr, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		if status == string(StatusInactive) {
			return nil, ErrInactiveProduct
		}
		if delta < 0 && currentStock+delta < 0 {
			return nil, ErrInsufficientStock
		}
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	movement.PreviousStock = product.CurrentStock - delta
	movement.NewStock = product.CurrentStock

	if err := tx.QueryRow(ctx,
		`INSERT INTO stock_movements (id, product_id, type, quantity, previous_stock, new_stock, reason)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING occurred_at`,
		movement.ID, id, string(movement.Type), movement.Quantity, movement.PreviousStock, movement.NewStock, movement.Reason,
	).Scan(&movement.CreatedAt); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return product, nil
}

// ListMovements loads the movements recorded for a product, most recent
// first, paginated the same way List does.
//
// Args:
//
//	ctx(context.Context): request context
//	productID(uuid.UUID): product ID
//	page(int): 1-based page number
//	pageSize(int): items per page
//
// Returns:
//
//	movements([]*StockMovement): page of movements
//	total(int): total movement count for this product
//	err(error): database error if any
func (repository *PostgresProductRepository) ListMovements(ctx context.Context, productID uuid.UUID, page, pageSize int) ([]*StockMovement, int, error) {
	offset := (page - 1) * pageSize

	rows, err := repository.pool.Query(ctx,
		`SELECT id, product_id, type, quantity, previous_stock, new_stock, reason, occurred_at, COUNT(*) OVER() AS total
		 FROM stock_movements
		 WHERE product_id = $1
		 ORDER BY occurred_at DESC
		 LIMIT $2 OFFSET $3`,
		productID, pageSize, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var movements []*StockMovement
	total := 0
	for rows.Next() {
		movement := &StockMovement{}
		var typeStr string
		var reason *string
		if err := rows.Scan(&movement.ID, &movement.ProductID, &typeStr, &movement.Quantity,
			&movement.PreviousStock, &movement.NewStock, &reason, &movement.CreatedAt, &total); err != nil {
			return nil, 0, err
		}
		movement.Type = MovementType(typeStr)
		if reason != nil {
			movement.Reason = *reason
		}
		movements = append(movements, movement)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return movements, total, nil
}

// IsUsedInQuotesOrOrders checks if a product is referenced in existing quotes or service orders.
//
// Args:
//
//	ctx(context.Context): request context
//	id(uuid.UUID): product technical ID
//
// Returns:
//
//	used(bool): true if referenced in quote_products
//	err(error): database error if any
func (repository *PostgresProductRepository) IsUsedInQuotesOrOrders(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	err := repository.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM quote_products WHERE product_id = $1)`,
		id,
	).Scan(&exists)
	return exists, err
}

func scanProduct(row pgx.Row) (*Product, error) {
	product := &Product{}
	var typeStr, statusStr string

	err := row.Scan(
		&product.ID, &product.Code, &product.Name, &product.Description, &product.UnitPrice, &product.CurrentStock, &typeStr, &statusStr,
		&product.CreatedAt, &product.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	product.Type = Type(typeStr)
	product.Status = Status(statusStr)
	return product, nil
}

func mapUniqueViolation(err error) error {
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != postgresUniqueViolation {
		return err
	}

	if postgresError.ConstraintName == codeUniqueConstraint {
		return ErrDuplicateCode
	}
	return err
}

func strconvIdx(idx *int) string {
	res := string(rune('0' + *idx))
	if *idx >= 10 {
		res = convertIntToString(*idx)
	}
	*idx++
	return res
}

func convertIntToString(n int) string {
	if n == 0 {
		return "0"
	}
	var b [10]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
