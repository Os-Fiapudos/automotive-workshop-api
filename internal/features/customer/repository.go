package customer

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"automotive-workshop-api/internal/shared/document"
)

// CustomerRepository is the persistence boundary for the Customer aggregate.
// It is defined next to its only consumer (CustomerService) rather than in a
// separate "contracts" package — see specs/customer-management/design.md
// §1.1/§1.3.
type CustomerRepository interface {
	Create(ctx context.Context, customer *Customer) error
	FindByID(ctx context.Context, id uuid.UUID) (*Customer, error)
	FindByDocument(ctx context.Context, normalizedDocument string) (*Customer, error)
	// ExistsByDocument reports whether normalizedDocument already belongs to
	// a customer other than excludeID (nil to check against every customer).
	ExistsByDocument(ctx context.Context, normalizedDocument string, excludeID *uuid.UUID) (bool, error)
	List(ctx context.Context, page, pageSize int) ([]*Customer, int, error)
	Update(ctx context.Context, customer *Customer) error
}

// postgresUniqueViolation is the SQLSTATE Postgres returns for any unique
// index/constraint violation. SQLSTATE alone doesn't say *which* constraint
// was violated — see mapUniqueViolation below, which reads
// pgconn.PgError.ConstraintName to tell customers.document and
// customers.email violations apart.
const postgresUniqueViolation = "23505"

// Names of the unique constraints declared in docs/schema.sql that this
// repository's writes can violate.
const (
	documentUniqueConstraint = "ux_customers_document"
	emailUniqueConstraint    = "ux_customers_email"
)

const customerColumns = `id, code, name, document, document_type, phone, email, status, created_at, updated_at`

// PostgresCustomerRepository implements CustomerRepository against pgx.
type PostgresCustomerRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresCustomerRepository(pool *pgxpool.Pool) *PostgresCustomerRepository {
	return &PostgresCustomerRepository{pool: pool}
}

func (repository *PostgresCustomerRepository) Create(ctx context.Context, customer *Customer) error {
	const query = `
		INSERT INTO customers (id, name, document, document_type, phone, email, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING code, created_at, updated_at`

	err := repository.pool.QueryRow(ctx, query,
		customer.ID, customer.Name, customer.Document.Value, string(customer.Document.Type), customer.Phone, customer.Email, string(customer.Status),
	).Scan(&customer.Code, &customer.CreatedAt, &customer.UpdatedAt)

	return mapUniqueViolation(err)
}

func (repository *PostgresCustomerRepository) FindByID(ctx context.Context, id uuid.UUID) (*Customer, error) {
	row := repository.pool.QueryRow(ctx, `SELECT `+customerColumns+` FROM customers WHERE id = $1`, id)
	return scanCustomer(row)
}

func (repository *PostgresCustomerRepository) FindByDocument(ctx context.Context, normalizedDocument string) (*Customer, error) {
	row := repository.pool.QueryRow(ctx, `SELECT `+customerColumns+` FROM customers WHERE document = $1`, normalizedDocument)
	return scanCustomer(row)
}

func (repository *PostgresCustomerRepository) ExistsByDocument(ctx context.Context, normalizedDocument string, excludeID *uuid.UUID) (bool, error) {
	var exists bool
	var err error

	if excludeID != nil {
		err = repository.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM customers WHERE document = $1 AND id <> $2)`,
			normalizedDocument, *excludeID,
		).Scan(&exists)
	} else {
		err = repository.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM customers WHERE document = $1)`,
			normalizedDocument,
		).Scan(&exists)
	}

	return exists, err
}

func (repository *PostgresCustomerRepository) List(ctx context.Context, page, pageSize int) ([]*Customer, int, error) {
	offset := (page - 1) * pageSize

	rows, err := repository.pool.Query(ctx,
		`SELECT `+customerColumns+`, COUNT(*) OVER() AS total
		 FROM customers
		 ORDER BY code
		 LIMIT $1 OFFSET $2`,
		pageSize, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var customers []*Customer
	total := 0
	for rows.Next() {
		customer, documentType, status := &Customer{}, "", ""
		var documentValue string
		if err := rows.Scan(
			&customer.ID, &customer.Code, &customer.Name, &documentValue, &documentType, &customer.Phone, &customer.Email, &status,
			&customer.CreatedAt, &customer.UpdatedAt, &total,
		); err != nil {
			return nil, 0, err
		}
		customer.Document = document.Document{Value: documentValue, Type: document.Type(documentType)}
		customer.Status = Status(status)
		customers = append(customers, customer)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return customers, total, nil
}

func (repository *PostgresCustomerRepository) Update(ctx context.Context, customer *Customer) error {
	const query = `
		UPDATE customers
		SET name = $2, document = $3, document_type = $4, phone = $5, email = $6, status = $7
		WHERE id = $1
		RETURNING updated_at`

	err := repository.pool.QueryRow(ctx, query,
		customer.ID, customer.Name, customer.Document.Value, string(customer.Document.Type), customer.Phone, customer.Email, string(customer.Status),
	).Scan(&customer.UpdatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return mapUniqueViolation(err)
}

func scanCustomer(row pgx.Row) (*Customer, error) {
	customer := &Customer{}
	var documentValue, documentType, status string

	err := row.Scan(
		&customer.ID, &customer.Code, &customer.Name, &documentValue, &documentType, &customer.Phone, &customer.Email, &status,
		&customer.CreatedAt, &customer.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	customer.Document = document.Document{Value: documentValue, Type: document.Type(documentType)}
	customer.Status = Status(status)
	return customer, nil
}

// mapUniqueViolation translates a Postgres unique-constraint-violation error
// into the matching domain error, by constraint name, so the API reports
// which field actually collided instead of always assuming it was the
// document (see design.md §1.4). Any other error — including nil — is
// returned unchanged.
func mapUniqueViolation(err error) error {
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != postgresUniqueViolation {
		return err
	}

	switch postgresError.ConstraintName {
	case documentUniqueConstraint:
		return ErrDuplicateDocument
	case emailUniqueConstraint:
		return ErrDuplicateEmail
	default:
		return err
	}
}
