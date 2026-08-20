# Design Specification: Product & Stock Management

## 1. Architecture Overview
Vertical slice architecture located at `internal/features/product/`.

### Stock Adjustment Flow
1. Client submits `POST /api/v1/produtos/{id}/estoque/ajustes` with JSON payload (`type`, `quantity`, `reason`).
2. `handler.go` decodes payload and calls `request.Validate()`. If invalid, writes `apierror.Validation`.
3. `service.go` calls domain method `product.AdjustStock(movementType, quantity, reason)`.
4. `repository.go` executes atomic SQL update `UPDATE products SET current_stock = current_stock + $2, updated_at = now() WHERE id = $1 AND (current_stock + $2) >= 0`.
5. If result affects 0 rows and type is `EXIT`, repository returns `ErrInsufficientStock` mapped to 409 Conflict (`INSUFFICIENT_STOCK`).

## 2. Data Contract & DTOs

### Stock Adjustment Request (`POST /api/v1/produtos/{id}/estoque/ajustes`)
```json
{
  "type": "ENTRY",
  "quantity": 10,
  "reason": "Ajuste de inventário anual"
}
```

### Stock Balance Response (`GET /api/v1/produtos/{id}/estoque`)
```json
{
  "id": "c0000000-0000-0000-0000-000000000001",
  "code": 101,
  "name": "Filtro de Óleo",
  "currentStock": 30,
  "status": "ACTIVE",
  "updatedAt": "2026-08-20T00:00:00Z"
}
```

### Stock Movements Response (`GET /api/v1/produtos/{id}/movimentacoes`)
```json
{
  "data": [
    {
      "id": "c0000000-0000-0000-0000-000000000001",
      "productId": "c0000000-0000-0000-0000-000000000001",
      "type": "ENTRY",
      "quantity": 10,
      "reason": "Ajuste de inventário anual",
      "createdAt": "2026-08-20T00:00:00Z"
    }
  ],
  "page": 1,
  "pageSize": 20,
  "total": 1,
  "totalPages": 1
}
```
