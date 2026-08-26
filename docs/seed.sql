-- =============================================================================
-- Sample seed data — automotive-workshop-api
-- Prerequisite: docs/schema.sql already applied.
--
-- Uses fixed IDs (prefix-readable UUIDs) to allow explicit FKs and safe
-- re-execution via ON CONFLICT DO NOTHING (does not duplicate on re-run).
--
-- Run with:
--   docker compose exec -T db psql -U workshop -d automotive_workshop -f - < docs/seed.sql
--   (or, with the file mounted in the container: psql -f /docker-entrypoint-initdb.d/... )
-- =============================================================================

BEGIN;

-- ==== Customers ====
-- document is stored normalized (digits only) and document_type/status are
-- required as of the Customer Management feature — see
-- specs/customer-management/. Documents below are real, check-digit-valid
-- CPF/CNPJ numbers (not just plausible-looking strings), matching what the
-- application itself would persist.

INSERT INTO customers (id, name, document, document_type, phone, email, status) VALUES
    ('a0000000-0000-0000-0000-000000000001', 'João Pedro Silva',        '12345678909',     'CPF',  '(11) 91234-5678', 'joao.silva@example.com', 'ACTIVE'),
    ('a0000000-0000-0000-0000-000000000002', 'Maria Fernanda Costa',    '98765432100',     'CPF',  '(11) 99876-5432', 'maria.costa@example.com', 'ACTIVE'),
    ('a0000000-0000-0000-0000-000000000003', 'Transportadora Rota Sul Ltda', '12345678000195', 'CNPJ', '(11) 3333-4444', 'contato@rotasul.com.br', 'ACTIVE'),
    ('a0000000-0000-0000-0000-000000000004', 'Carlos Eduardo Almeida',  '11122233396',     'CPF',  '(11) 98888-1111', NULL, 'INACTIVE')
ON CONFLICT (id) DO NOTHING;

-- ==== Vehicles ====
-- status is required as of the Vehicle Management feature — see
-- specs/vehicle-management/. The last vehicle belongs to a customer that is
-- itself INACTIVE above; that customer's pre-existing vehicle stays exactly
-- as it is (seed data isn't run through the application's own create-time
-- "customer must be ACTIVE" check), illustrating that inactivity only blocks
-- *new* creation, never hides existing history (requirements.md BR8). This
-- also doubles as fixture data for Service Order Opening (see
-- specs/service-order-opening/): vehicle 5 being INACTIVE (owned by the
-- also-INACTIVE customer 4) exercises its rejection path.

INSERT INTO vehicles (id, license_plate, brand, model, year, color, customer_id, status) VALUES
    ('b0000000-0000-0000-0000-000000000001', 'ABC1D23', 'Fiat',       'Uno',    2018, 'White', 'a0000000-0000-0000-0000-000000000001', 'ACTIVE'),
    ('b0000000-0000-0000-0000-000000000002', 'DEF4E56', 'Volkswagen', 'Gol',    2020, 'Silver','a0000000-0000-0000-0000-000000000002', 'ACTIVE'),
    ('b0000000-0000-0000-0000-000000000003', 'GHI7F89', 'Chevrolet',  'Onix',   2022, 'Black', 'a0000000-0000-0000-0000-000000000002', 'ACTIVE'),
    ('b0000000-0000-0000-0000-000000000004', 'JKL0G12', 'Mercedes-Benz', 'Sprinter', 2019, 'White', 'a0000000-0000-0000-0000-000000000003', 'ACTIVE'),
    ('b0000000-0000-0000-0000-000000000005', 'MNO3H45', 'Honda',      'Civic',  2021, 'Gray',  'a0000000-0000-0000-0000-000000000004', 'INACTIVE')
ON CONFLICT (id) DO NOTHING;

-- ==== Products ====

INSERT INTO products (id, name, description, unit_price, current_stock, type, status) VALUES
    ('c0000000-0000-0000-0000-000000000001', 'Oil Filter',            'Engine oil filter, general use.',                  35.90,  42, 'PART',   'ACTIVE'),
    ('c0000000-0000-0000-0000-000000000002', 'Engine Oil 5W30 (1L)',  'Synthetic engine oil, 1-liter package.',           48.50, 8,  'SUPPLY', 'ACTIVE'),
    ('c0000000-0000-0000-0000-000000000003', 'Brake Pad',             'Set of front brake pads.',                         129.90, 15, 'PART',   'ACTIVE'),
    ('c0000000-0000-0000-0000-000000000004', 'Timing Belt',           'Timing belt for the distribution system.',         89.90,  6,  'PART',   'ACTIVE'),
    ('c0000000-0000-0000-0000-000000000005', 'DOT4 Brake Fluid',      'Brake fluid, 500ml package.',                      22.00,  3,  'SUPPLY', 'ACTIVE'),
    ('c0000000-0000-0000-0000-000000000006', 'H4 Headlight Bulb',     'Halogen bulb for the front headlight.',            18.50,  25, 'PART',   'ACTIVE')
ON CONFLICT (id) DO NOTHING;

-- ==== Services ====

-- The last one is deliberately inactive: a retired service kept for history, so the
-- catalog listing has both states to distinguish (specs/service-catalog AC5).

INSERT INTO services (id, name, description, price, estimated_time, active) VALUES
    ('d0000000-0000-0000-0000-000000000001', 'Oil Change',              'Engine oil and filter change.',                       80.00,  30,   TRUE),
    ('d0000000-0000-0000-0000-000000000002', 'Alignment and Balancing', 'Steering alignment and wheel balancing.',             120.00, 60,   TRUE),
    ('d0000000-0000-0000-0000-000000000003', 'Brake Inspection',        'Inspection and replacement of pads/discs if needed.', 150.00, 90,   TRUE),
    ('d0000000-0000-0000-0000-000000000004', 'Timing Belt Replacement', 'Replacement of the distribution timing belt.',        250.00, 180,  TRUE),
    ('d0000000-0000-0000-0000-000000000005', 'Electronic Diagnostics',  'Reading error codes via automotive scanner.',         60.00,  NULL, TRUE),
    ('d0000000-0000-0000-0000-000000000006', 'Carburetor Cleaning',     'Retired service, kept for historical service orders.', 90.00, 45,   FALSE)
ON CONFLICT (id) DO NOTHING;

-- ==== Service Orders ====
-- Covering the main statuses of the lifecycle.
-- NOTE: status values are intentionally kept in Portuguese (RECEBIDA,
-- EM_DIAGNOSTICO, AGUARDANDO_APROVACAO, EM_EXECUCAO, FINALIZADA, ENTREGUE) —
-- see docs/entities.md for the rationale.

INSERT INTO service_orders (id, customer_id, vehicle_id, opened_at, status, notes) VALUES
    ('e0000000-0000-0000-0000-000000000001', 'a0000000-0000-0000-0000-000000000001', 'b0000000-0000-0000-0000-000000000001', now() - interval '10 days', 'ENTREGUE',             'Routine oil change. Customer reported a light engine noise.'),
    ('e0000000-0000-0000-0000-000000000002', 'a0000000-0000-0000-0000-000000000002', 'b0000000-0000-0000-0000-000000000002', now() - interval '5 days',  'EM_EXECUCAO',          'Brake inspection, customer approved the quote.'),
    ('e0000000-0000-0000-0000-000000000003', 'a0000000-0000-0000-0000-000000000002', 'b0000000-0000-0000-0000-000000000003', now() - interval '2 days',  'AGUARDANDO_APROVACAO', 'Diagnostics indicated the timing belt needs replacement.'),
    ('e0000000-0000-0000-0000-000000000004', 'a0000000-0000-0000-0000-000000000003', 'b0000000-0000-0000-0000-000000000004', now() - interval '1 day',   'EM_DIAGNOSTICO',       'Van with intermittent failure to start.'),
    ('e0000000-0000-0000-0000-000000000005', 'a0000000-0000-0000-0000-000000000004', 'b0000000-0000-0000-0000-000000000005', now(),                       'RECEBIDA',             'Customer brought it in for alignment and balancing.')
ON CONFLICT (id) DO NOTHING;

-- ==== Service Order Requested Services ====
-- Initial demand recorded at order-opening time (see
-- specs/service-order-opening/) — not the priced quote. Order 5 (RECEBIDA)
-- is the only one still at that stage; earlier orders had their demand
-- already superseded by a generated quote.

INSERT INTO service_order_requested_services (service_order_id, service_id) VALUES
    ('e0000000-0000-0000-0000-000000000005', 'd0000000-0000-0000-0000-000000000002')
ON CONFLICT DO NOTHING;

-- ==== Quotes ====
-- 1:1 with service_orders. status/responded_at consistent with the service order status.
-- sent_at/sent_version (specs/service-order-quote-decision/) are populated for every quote
-- whose order already reached AGUARDANDO_APROVACAO or beyond, since that transition is only
-- ever produced by sending the quote; quote 5 stays unsent, consistent with its order still
-- being RECEBIDA.

INSERT INTO quotes (id, service_order_id, total_amount, status, version, generated_at, sent_at, sent_version, responded_at) VALUES
    ('f0000000-0000-0000-0000-000000000001', 'e0000000-0000-0000-0000-000000000001', 84.40,  'APPROVED', 1, now() - interval '10 days', now() - interval '10 days' + interval '1 hour', 1, now() - interval '10 days' + interval '2 hours'),
    ('f0000000-0000-0000-0000-000000000002', 'e0000000-0000-0000-0000-000000000002', 279.90, 'APPROVED', 1, now() - interval '5 days',  now() - interval '5 days' + interval '30 minutes', 1, now() - interval '5 days' + interval '1 hour'),
    ('f0000000-0000-0000-0000-000000000003', 'e0000000-0000-0000-0000-000000000003', 339.90, 'PENDING',  1, now() - interval '2 days',  now() - interval '2 days' + interval '1 hour', 1, NULL),
    ('f0000000-0000-0000-0000-000000000005', 'e0000000-0000-0000-0000-000000000005', 120.00, 'PENDING',  1, now(),                       NULL, NULL, NULL)
ON CONFLICT (id) DO NOTHING;

-- Service order 4 (EM_DIAGNOSTICO) has no quote generated yet — a realistic scenario.

-- ==== Quote items (products) ====
-- applied_description/applied_total_price are new columns added by
-- specs/service-order-diagnosis-quote/ — description copied from the matching
-- products row, total = quantity * applied_unit_price.

INSERT INTO quote_products (quote_id, product_id, quantity, applied_description, applied_unit_price, applied_total_price) VALUES
    ('f0000000-0000-0000-0000-000000000001', 'c0000000-0000-0000-0000-000000000001', 1, 'Engine oil filter, general use.',          35.90,  35.90),
    ('f0000000-0000-0000-0000-000000000001', 'c0000000-0000-0000-0000-000000000002', 1, 'Synthetic engine oil, 1-liter package.',   48.50,  48.50),
    ('f0000000-0000-0000-0000-000000000002', 'c0000000-0000-0000-0000-000000000003', 1, 'Set of front brake pads.',                 129.90, 129.90),
    ('f0000000-0000-0000-0000-000000000003', 'c0000000-0000-0000-0000-000000000004', 1, 'Timing belt for the distribution system.', 89.90,  89.90),
    ('f0000000-0000-0000-0000-000000000003', 'c0000000-0000-0000-0000-000000000005', 1, 'Brake fluid, 500ml package.',              22.00,  22.00)
ON CONFLICT DO NOTHING;

-- ==== Quote items (services) ====
-- quantity/applied_description/applied_total_price are new columns added by
-- specs/service-order-diagnosis-quote/.

INSERT INTO quote_services (quote_id, service_id, quantity, applied_description, applied_price, applied_total_price) VALUES
    ('f0000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001', 1, 'Engine oil and filter change.',                       80.00,  80.00),
    ('f0000000-0000-0000-0000-000000000002', 'd0000000-0000-0000-0000-000000000003', 1, 'Inspection and replacement of pads/discs if needed.', 150.00, 150.00),
    ('f0000000-0000-0000-0000-000000000003', 'd0000000-0000-0000-0000-000000000004', 1, 'Replacement of the distribution timing belt.',        228.00, 228.00),
    ('f0000000-0000-0000-0000-000000000005', 'd0000000-0000-0000-0000-000000000002', 1, 'Steering alignment and wheel balancing.',              120.00, 120.00)
ON CONFLICT DO NOTHING;

-- ==== Service Order History ====
-- Full trail for service order 1 (already ENTREGUE) and partial for service order 2 (EM_EXECUCAO).

INSERT INTO service_order_history (id, service_order_id, occurred_at, event, description, previous_status, new_status) VALUES
    ('11100000-0000-0000-0000-000000000001', 'e0000000-0000-0000-0000-000000000001', now() - interval '10 days',                    'creation',    'Service order opened for an oil change.',        'RECEBIDA',             'RECEBIDA'),
    ('11100000-0000-0000-0000-000000000002', 'e0000000-0000-0000-0000-000000000001', now() - interval '10 days' + interval '2 hours','approval',    'Customer approved the quote.',                   'AGUARDANDO_APROVACAO', 'EM_EXECUCAO'),
    ('11100000-0000-0000-0000-000000000003', 'e0000000-0000-0000-0000-000000000001', now() - interval '9 days',                      'completion',  'Service completed and vehicle delivered.',       'EM_EXECUCAO',          'ENTREGUE'),
    ('11100000-0000-0000-0000-000000000004', 'e0000000-0000-0000-0000-000000000002', now() - interval '5 days',                      'creation',    'Service order opened for a brake inspection.',   'RECEBIDA',             'RECEBIDA'),
    ('11100000-0000-0000-0000-000000000005', 'e0000000-0000-0000-0000-000000000002', now() - interval '5 days' + interval '1 hour',  'approval',    'Customer approved the quote.',                   'AGUARDANDO_APROVACAO', 'EM_EXECUCAO')
ON CONFLICT (id) DO NOTHING;

-- ==== Service Execution Audit (ServiceExecution — specs/service-order-execution/) ====
-- One row per execution, started_at/ended_at instead of a start/end event log
-- (specs/service-order-execution/design.md §1.3).
-- Service order 1 (ENTREGUE): oil change execution, started and finished.
-- Service order 2 (EM_EXECUCAO): brake inspection started, not yet finished.

INSERT INTO audit_services (id, service_order_id, service_id, started_at, ended_at) VALUES
    ('22200000-0000-0000-0000-000000000001', 'e0000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001', now() - interval '10 days' + interval '2 hours', now() - interval '10 days' + interval '2 hours 30 minutes'),
    ('22200000-0000-0000-0000-000000000003', 'e0000000-0000-0000-0000-000000000002', 'd0000000-0000-0000-0000-000000000003', now() - interval '5 days' + interval '1 hour', NULL)
ON CONFLICT (id) DO NOTHING;

-- ==== Users (administrative) ====
-- Dev-only credentials: admin@workshop.local / admin123, and
-- soat-architecture@workshop.local / soat-architecture (evaluation account,
-- so a reviewer can log in without creating a user first).
-- The password is bcrypt-hashed at insert time via pgcrypto's crypt() —
-- only the hash is stored (BR1/AC5). Never use these credentials outside
-- local development.

INSERT INTO users (id, name, email, password_hash) VALUES
    ('f0000000-0000-0000-0000-000000000001', 'Workshop Admin', 'admin@workshop.local',
     crypt('admin123', gen_salt('bf', 10))),
    ('f0000000-0000-0000-0000-000000000002', 'soat-architecture', 'soat-architecture@workshop.local',
     crypt('soat-architecture', gen_salt('bf', 10)))
ON CONFLICT (id) DO NOTHING;

COMMIT;
