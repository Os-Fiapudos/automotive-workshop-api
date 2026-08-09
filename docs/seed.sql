-- =============================================================================
-- Seed de dados de exemplo — automotive-workshop-api
-- Pré-requisito: docs/schema.sql já aplicado.
--
-- Usa IDs fixos (UUIDs legíveis por prefixo) para permitir FKs explícitas e
-- reexecução segura via ON CONFLICT DO NOTHING (não duplica se rodar de novo).
--
-- Execução:
--   docker compose exec -T db psql -U workshop -d automotive_workshop -f - < docs/seed.sql
--   (ou, com o arquivo montado no container: psql -f /docker-entrypoint-initdb.d/... )
-- =============================================================================

BEGIN;

-- ==== Clientes ====

INSERT INTO clientes (id, nome, documento, telefone, email) VALUES
    ('a0000000-0000-0000-0000-000000000001', 'João Pedro Silva',        '123.456.789-00', '(11) 91234-5678', 'joao.silva@example.com'),
    ('a0000000-0000-0000-0000-000000000002', 'Maria Fernanda Costa',    '987.654.321-00', '(11) 99876-5432', 'maria.costa@example.com'),
    ('a0000000-0000-0000-0000-000000000003', 'Transportadora Rota Sul Ltda', '12.345.678/0001-90', '(11) 3333-4444', 'contato@rotasul.com.br'),
    ('a0000000-0000-0000-0000-000000000004', 'Carlos Eduardo Almeida',  '111.222.333-44', '(11) 98888-1111', NULL)
ON CONFLICT (id) DO NOTHING;

-- ==== Veiculos ====

INSERT INTO veiculos (id, placa, marca, modelo, ano, cor, cliente_id) VALUES
    ('b0000000-0000-0000-0000-000000000001', 'ABC1D23', 'Fiat',       'Uno',    2018, 'Branco',  'a0000000-0000-0000-0000-000000000001'),
    ('b0000000-0000-0000-0000-000000000002', 'DEF4E56', 'Volkswagen', 'Gol',    2020, 'Prata',   'a0000000-0000-0000-0000-000000000002'),
    ('b0000000-0000-0000-0000-000000000003', 'GHI7F89', 'Chevrolet',  'Onix',   2022, 'Preto',   'a0000000-0000-0000-0000-000000000002'),
    ('b0000000-0000-0000-0000-000000000004', 'JKL0G12', 'Mercedes-Benz', 'Sprinter', 2019, 'Branco', 'a0000000-0000-0000-0000-000000000003'),
    ('b0000000-0000-0000-0000-000000000005', 'MNO3H45', 'Honda',      'Civic',  2021, 'Cinza',   'a0000000-0000-0000-0000-000000000004')
ON CONFLICT (id) DO NOTHING;

-- ==== Produtos ====

INSERT INTO produtos (id, nome, descricao, valor_unitario, estoque_atual, tipo) VALUES
    ('c0000000-0000-0000-0000-000000000001', 'Filtro de Óleo',        'Filtro de óleo do motor, uso geral.',        35.90,  42, 'PECA'),
    ('c0000000-0000-0000-0000-000000000002', 'Óleo Motor 5W30 (1L)',  'Óleo sintético para motor, embalagem de 1 litro.', 48.50, 8, 'INSUMO'),
    ('c0000000-0000-0000-0000-000000000003', 'Pastilha de Freio',     'Jogo de pastilhas de freio dianteiras.',     129.90, 15, 'PECA'),
    ('c0000000-0000-0000-0000-000000000004', 'Correia Dentada',       'Correia dentada de distribuição.',           89.90,  6,  'PECA'),
    ('c0000000-0000-0000-0000-000000000005', 'Fluido de Freio DOT4',  'Fluido de freio, embalagem de 500ml.',       22.00,  3,  'INSUMO'),
    ('c0000000-0000-0000-0000-000000000006', 'Lâmpada Farol H4',      'Lâmpada halógena para farol dianteiro.',     18.50,  25, 'PECA')
ON CONFLICT (id) DO NOTHING;

-- ==== Servicos ====

INSERT INTO servicos (id, nome, descricao, valor, tempo_estimado) VALUES
    ('d0000000-0000-0000-0000-000000000001', 'Troca de Óleo',        'Troca de óleo e filtro do motor.',                     80.00,  30),
    ('d0000000-0000-0000-0000-000000000002', 'Alinhamento e Balanceamento', 'Alinhamento de direção e balanceamento das rodas.', 120.00, 60),
    ('d0000000-0000-0000-0000-000000000003', 'Revisão de Freios',    'Inspeção e troca de pastilhas/discos se necessário.',  150.00, 90),
    ('d0000000-0000-0000-0000-000000000004', 'Troca de Correia Dentada', 'Substituição da correia dentada de distribuição.',  250.00, 180),
    ('d0000000-0000-0000-0000-000000000005', 'Diagnóstico Eletrônico', 'Leitura de códigos de erro via scanner automotivo.', 60.00, NULL)
ON CONFLICT (id) DO NOTHING;

-- ==== Ordens de Serviço ====
-- Cobrindo os principais status do ciclo de vida.

INSERT INTO ordens_servico (id, cliente_id, veiculo_id, data_criacao, status, observacoes) VALUES
    ('e0000000-0000-0000-0000-000000000001', 'a0000000-0000-0000-0000-000000000001', 'b0000000-0000-0000-0000-000000000001', now() - interval '10 days', 'ENTREGUE',            'Troca de óleo de rotina. Cliente relatou barulho leve no motor.'),
    ('e0000000-0000-0000-0000-000000000002', 'a0000000-0000-0000-0000-000000000002', 'b0000000-0000-0000-0000-000000000002', now() - interval '5 days',  'EM_EXECUCAO',         'Revisão de freios, cliente aprovou orçamento.'),
    ('e0000000-0000-0000-0000-000000000003', 'a0000000-0000-0000-0000-000000000002', 'b0000000-0000-0000-0000-000000000003', now() - interval '2 days',  'AGUARDANDO_APROVACAO', 'Diagnóstico apontou necessidade de troca de correia dentada.'),
    ('e0000000-0000-0000-0000-000000000004', 'a0000000-0000-0000-0000-000000000003', 'b0000000-0000-0000-0000-000000000004', now() - interval '1 day',   'EM_DIAGNOSTICO',      'Van com falha intermitente ao ligar.'),
    ('e0000000-0000-0000-0000-000000000005', 'a0000000-0000-0000-0000-000000000004', 'b0000000-0000-0000-0000-000000000005', now(),                       'RECEBIDA',            'Cliente trouxe para alinhamento e balanceamento.')
ON CONFLICT (id) DO NOTHING;

-- ==== Orcamentos ====
-- 1:1 com ordens_servico. status/data_resposta coerentes com o status da OS.

INSERT INTO orcamentos (id, ordem_servico_id, valor, status, data_geracao, data_resposta) VALUES
    ('f0000000-0000-0000-0000-000000000001', 'e0000000-0000-0000-0000-000000000001', 84.40,  'APROVADO', now() - interval '10 days', now() - interval '10 days' + interval '2 hours'),
    ('f0000000-0000-0000-0000-000000000002', 'e0000000-0000-0000-0000-000000000002', 279.90, 'APROVADO', now() - interval '5 days',  now() - interval '5 days' + interval '1 hour'),
    ('f0000000-0000-0000-0000-000000000003', 'e0000000-0000-0000-0000-000000000003', 339.90, 'PENDENTE', now() - interval '2 days',  NULL),
    ('f0000000-0000-0000-0000-000000000005', 'e0000000-0000-0000-0000-000000000005', 120.00, 'PENDENTE', now(),                       NULL)
ON CONFLICT (id) DO NOTHING;

-- OS 4 (EM_DIAGNOSTICO) ainda não tem orçamento gerado — cenário realista.

-- ==== Itens do Orcamento (produtos) ====

INSERT INTO orcamento_produtos (orcamento_id, produto_id, quantidade, valor_unitario_aplicado) VALUES
    ('f0000000-0000-0000-0000-000000000001', 'c0000000-0000-0000-0000-000000000001', 1, 35.90),
    ('f0000000-0000-0000-0000-000000000001', 'c0000000-0000-0000-0000-000000000002', 1, 48.50),
    ('f0000000-0000-0000-0000-000000000002', 'c0000000-0000-0000-0000-000000000003', 1, 129.90),
    ('f0000000-0000-0000-0000-000000000003', 'c0000000-0000-0000-0000-000000000004', 1, 89.90),
    ('f0000000-0000-0000-0000-000000000003', 'c0000000-0000-0000-0000-000000000005', 1, 22.00)
ON CONFLICT DO NOTHING;

-- ==== Itens do Orcamento (servicos) ====

INSERT INTO orcamento_servicos (orcamento_id, servico_id, valor_aplicado) VALUES
    ('f0000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001', 80.00),
    ('f0000000-0000-0000-0000-000000000002', 'd0000000-0000-0000-0000-000000000003', 150.00),
    ('f0000000-0000-0000-0000-000000000003', 'd0000000-0000-0000-0000-000000000004', 228.00),
    ('f0000000-0000-0000-0000-000000000005', 'd0000000-0000-0000-0000-000000000002', 120.00)
ON CONFLICT DO NOTHING;

-- ==== Historico da Ordem de Servico ====
-- Trilha completa para a OS 1 (já ENTREGUE) e parcial para a OS 2 (EM_EXECUCAO).

INSERT INTO historico_ordem_servico (id, ordem_servico_id, data_hora, evento, descricao, status_old, status_new) VALUES
    ('11100000-0000-0000-0000-000000000001', 'e0000000-0000-0000-0000-000000000001', now() - interval '10 days',                    'criacao',     'OS aberta para troca de óleo.',            'RECEBIDA',             'RECEBIDA'),
    ('11100000-0000-0000-0000-000000000002', 'e0000000-0000-0000-0000-000000000001', now() - interval '10 days' + interval '2 hours','aprovacao',   'Cliente aprovou o orçamento.',              'AGUARDANDO_APROVACAO', 'EM_EXECUCAO'),
    ('11100000-0000-0000-0000-000000000003', 'e0000000-0000-0000-0000-000000000001', now() - interval '9 days',                      'finalizacao', 'Serviço concluído e veículo entregue.',     'EM_EXECUCAO',          'ENTREGUE'),
    ('11100000-0000-0000-0000-000000000004', 'e0000000-0000-0000-0000-000000000002', now() - interval '5 days',                      'criacao',     'OS aberta para revisão de freios.',         'RECEBIDA',             'RECEBIDA'),
    ('11100000-0000-0000-0000-000000000005', 'e0000000-0000-0000-0000-000000000002', now() - interval '5 days' + interval '1 hour',  'aprovacao',   'Cliente aprovou o orçamento.',              'AGUARDANDO_APROVACAO', 'EM_EXECUCAO')
ON CONFLICT (id) DO NOTHING;

-- ==== Auditoria de Execucao de Servicos ====
-- OS 1 (ENTREGUE): troca de óleo com início e fim registrados.
-- OS 2 (EM_EXECUCAO): revisão de freios iniciada, ainda sem fim.

INSERT INTO audit_services (id, ordem_servico_id, service_id, data_hora, evento) VALUES
    ('22200000-0000-0000-0000-000000000001', 'e0000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001', now() - interval '10 days' + interval '2 hours', 'inicio'),
    ('22200000-0000-0000-0000-000000000002', 'e0000000-0000-0000-0000-000000000001', 'd0000000-0000-0000-0000-000000000001', now() - interval '10 days' + interval '2 hours 30 minutes', 'fim'),
    ('22200000-0000-0000-0000-000000000003', 'e0000000-0000-0000-0000-000000000002', 'd0000000-0000-0000-0000-000000000003', now() - interval '5 days' + interval '1 hour', 'inicio')
ON CONFLICT (id) DO NOTHING;

COMMIT;
