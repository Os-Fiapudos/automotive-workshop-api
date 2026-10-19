-- service_order_requested_products: products/parts requested at service order opening
-- (specs/service-order-opening/requirements.md §7a, design.md §7.3). Mirrored in docs/schema.sql.
-- Numbered 0002 on top of the 0001_init baseline from specs/database-migrations/.

CREATE TABLE IF NOT EXISTS service_order_requested_products (
    service_order_id  UUID NOT NULL REFERENCES service_orders (id) ON DELETE CASCADE,
    product_id        UUID NOT NULL REFERENCES products (id) ON DELETE RESTRICT,
    quantity          INTEGER NOT NULL CHECK (quantity > 0),
    PRIMARY KEY (service_order_id, product_id)
);

COMMENT ON TABLE service_order_requested_products IS 'Products/parts initially requested when a service order was opened — unpriced demand only: not a stock reservation and not a stock debit.';
COMMENT ON COLUMN service_order_requested_products.service_order_id IS 'Reference to the ServiceOrder this request belongs to.';
COMMENT ON COLUMN service_order_requested_products.product_id IS 'Reference to the requested Product.';
COMMENT ON COLUMN service_order_requested_products.quantity IS 'Requested quantity (positive integer). Each product appears at most once per order.';

CREATE INDEX IF NOT EXISTS ix_service_order_requested_products_product_id
    ON service_order_requested_products (product_id);
