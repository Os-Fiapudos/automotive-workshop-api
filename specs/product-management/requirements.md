# Requirements Specification: Product & Stock Management

## 1. Feature Purpose
Manage replacement parts (`PART`) and consumable supplies (`SUPPLY`) catalog, stock adjustments, and balance querying in the automotive workshop system (`Modules/CatalogoEstoque/Produtos`), guaranteeing stock integrity, non-negative balances, and concurrent safety.

## 2. Functional Requirements (RF)
- **RF03.1 (Create Product)**: System must allow creating a new product with name, description, unit price, initial stock, and product type (`PART` or `SUPPLY`).
- **RF03.2 (Query Product & Stock)**: System must allow retrieving product details and current stock balance (`GET /api/v1/products/{id}/stock`).
- **RF03.3 (Update Product Cadastral)**: System must allow updating cadastral details (name, description, unit price, type). Direct stock updates via cadastral update are prohibited (RNF07).
- **RF03.4 (Stock Adjustment)**: System must allow registering stock adjustments (`POST /api/v1/products/{id}/stock/adjustments`) with type (`ENTRY` or `EXIT`), quantity (> 0), and reason.
- **RF03.5 (Deactivate Product)**: System must allow logical deactivation (`DELETE /api/v1/products/{id}`).
- **RF10 (Movement Traceability)**: System must allow querying product movement records (`GET /api/v1/products/{id}/movements`).

## 3. Non-Functional & Business Requirements (RNF / RN)
- **RNF02 (JWT Authentication)**: All product endpoints require valid JWT authentication.
- **RNF04 (Consistent REST Contracts)**: Responses follow consistent JSON structures and error envelopes (`apierror`).
- **RNF07 (Transactional Integrity)**: Stock balance updates are executed atomically preventing race conditions and double allocation.
- **RNF08 (Secure Logs)**: Sensitive details are omitted from system logs.
- **RN01 (Stock Adjustment Rules)**:
  - `ENTRY` increases `currentStock`.
  - `EXIT` decreases `currentStock`.
  - Quantity must be > 0.
  - Reason is required.
  - `EXIT` adjustment cannot generate negative stock (`currentStock - quantity >= 0`). If stock is insufficient, returns 409 Conflict (`INSUFFICIENT_STOCK`).
  - **Accepted `type` spellings** (documented on 2026-08-26; the behaviour itself predates
    this note â€” `product.ParseMovementType`): the request value is upper-cased and trimmed,
    then `ENTRY`, `ENTRADA` and `IN` all resolve to `ENTRY`, and `EXIT`, `SAIDA`, `SAÃDA`
    and `OUT` all resolve to `EXIT`. This is **input tolerance only** â€” the canonical
    `ENTRY`/`EXIT` values are the only ones ever persisted or returned, so it is not an
    exception to the English domain-language convention (`CLAUDE.md` Â§8). Kept deliberately
    rather than removed: dropping the aliases would break clients that already send them,
    and both collections in `docs/` document them as part of the contract.

## 4. Endpoints
- `POST /api/v1/products`
- `GET /api/v1/products`
- `GET /api/v1/products/{id}`
- `PATCH /api/v1/products/{id}`
- `DELETE /api/v1/products/{id}`
- `POST /api/v1/products/{id}/stock/adjustments`
- `GET /api/v1/products/{id}/stock`
- `GET /api/v1/products/{id}/movements`
