-- Reverts 0001_init.up.sql (specs/database-migrations/design.md §2.2).
-- Run only on a disposable database (requirements.md BR9): this drops every
-- application table and its data.
--
-- No CASCADE on purpose: an object this migration did not create that still
-- depends on these tables (e.g. 0002's table, if 0002 was not reverted first)
-- makes the revert fail instead of disappearing silently. pgcrypto is left in
-- place — 0001 re-creates it with IF NOT EXISTS.

-- ==== Tables (children before parents; their triggers go with them) ====
DROP TABLE IF EXISTS stock_movements;
DROP TABLE IF EXISTS audit_services;
DROP TABLE IF EXISTS service_order_tracking_tokens;
DROP TABLE IF EXISTS service_order_history;
DROP TABLE IF EXISTS quote_services;
DROP TABLE IF EXISTS quote_products;
DROP TABLE IF EXISTS quotes;
DROP TABLE IF EXISTS service_order_requested_services;
DROP TABLE IF EXISTS service_orders;
DROP TABLE IF EXISTS vehicles;
DROP TABLE IF EXISTS customers;
DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS services;
DROP TABLE IF EXISTS users;

-- ==== updated_at trigger function ====
DROP FUNCTION IF EXISTS set_updated_at();

-- ==== Enums ====
DROP TYPE IF EXISTS stock_movement_type;
DROP TYPE IF EXISTS history_event;
DROP TYPE IF EXISTS quote_status;
DROP TYPE IF EXISTS service_order_status;
DROP TYPE IF EXISTS vehicle_status;
DROP TYPE IF EXISTS customer_status;
DROP TYPE IF EXISTS customer_document_type;
DROP TYPE IF EXISTS product_status;
DROP TYPE IF EXISTS product_type;
