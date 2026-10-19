-- Reverts 0002_service_order_requested_products.up.sql.
DROP INDEX IF EXISTS ix_service_order_requested_products_product_id;
DROP TABLE IF EXISTS service_order_requested_products;
