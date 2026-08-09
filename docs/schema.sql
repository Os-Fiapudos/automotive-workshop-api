-- =============================================================================
-- Schema PostgreSQL — automotive-workshop-api
-- Gerado a partir de docs/entidades.md
--
-- Convenções:
--   - snake_case para tabelas/colunas.
--   - id UUID (chave técnica, gerada via pgcrypto) como PRIMARY KEY.
--   - codigo BIGINT IDENTITY (sequencial legível, exibido ao usuário).
--   - created_at/updated_at nas entidades "de cadastro" (não nas tabelas de
--     trilha de eventos, que já carregam data_hora do próprio evento).
--
-- Execução: psql -f docs/schema.sql -d automotive_workshop
-- =============================================================================

-- ==== Extensões ====

CREATE EXTENSION IF NOT EXISTS "pgcrypto"; -- gen_random_uuid()
-- Opcional: habilite para busca textual fuzzy em nome/placa (ver índices GIN abaixo).
-- CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- ==== Enums ====
-- Usamos ENUM nativo (não CHECK) por serem mais compactos em disco/índice e
-- auto-documentados. Custo: evoluir valores exige ALTER TYPE ... ADD VALUE.

DO $$ BEGIN
    CREATE TYPE produto_tipo AS ENUM ('PECA', 'INSUMO');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE ordem_servico_status AS ENUM (
        'RECEBIDA',
        'EM_DIAGNOSTICO',
        'AGUARDANDO_APROVACAO',
        'EM_EXECUCAO',
        'FINALIZADA',
        'ENTREGUE'
    );
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE orcamento_status AS ENUM ('PENDENTE', 'APROVADO', 'REPROVADO');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE historico_evento AS ENUM ('criacao', 'aprovacao', 'finalizacao', 'cancelamento');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE audit_evento AS ENUM ('inicio', 'fim');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- ==== Tabelas ====

-- ---- Cliente ----

CREATE TABLE IF NOT EXISTS clientes (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    codigo     BIGINT GENERATED ALWAYS AS IDENTITY,
    nome       TEXT NOT NULL,
    documento  TEXT NOT NULL,
    telefone   TEXT NOT NULL,
    email      TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE clientes IS 'Cliente da oficina, pessoa física ou jurídica.';
COMMENT ON COLUMN clientes.id IS 'Identificador técnico seguro, gerado pelo sistema.';
COMMENT ON COLUMN clientes.codigo IS 'Identificador legível/sequencial exibido ao usuário (ex: em telas, recibos, buscas).';
COMMENT ON COLUMN clientes.nome IS 'Nome completo ou razão social do cliente.';
COMMENT ON COLUMN clientes.documento IS 'CPF ou CNPJ do cliente, usado para identificação fiscal.';
COMMENT ON COLUMN clientes.telefone IS 'Telefone de contato principal.';
COMMENT ON COLUMN clientes.email IS 'E-mail de contato, usado para notificações e comunicação.';
COMMENT ON COLUMN clientes.created_at IS 'Data/hora de criação do registro, gerada automaticamente.';
COMMENT ON COLUMN clientes.updated_at IS 'Data/hora da última atualização do registro, gerada automaticamente.';

-- ---- Veiculo ----

CREATE TABLE IF NOT EXISTS veiculos (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    codigo     BIGINT GENERATED ALWAYS AS IDENTITY,
    placa      TEXT NOT NULL,
    marca      TEXT NOT NULL,
    modelo     TEXT NOT NULL,
    ano        SMALLINT NOT NULL,
    cor        TEXT NOT NULL,
    cliente_id UUID NOT NULL REFERENCES clientes (id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE veiculos IS 'Veículo pertencente a um cliente.';
COMMENT ON COLUMN veiculos.id IS 'Identificador técnico seguro, gerado pelo sistema.';
COMMENT ON COLUMN veiculos.codigo IS 'Identificador legível/sequencial exibido ao usuário.';
COMMENT ON COLUMN veiculos.placa IS 'Placa de identificação do veículo.';
COMMENT ON COLUMN veiculos.marca IS 'Fabricante do veículo (ex: Fiat, Volkswagen).';
COMMENT ON COLUMN veiculos.modelo IS 'Modelo do veículo (ex: Uno, Gol).';
COMMENT ON COLUMN veiculos.ano IS 'Ano de fabricação/modelo do veículo.';
COMMENT ON COLUMN veiculos.cor IS 'Cor predominante do veículo.';
COMMENT ON COLUMN veiculos.cliente_id IS 'Referência ao Cliente proprietário do veículo.';
COMMENT ON COLUMN veiculos.created_at IS 'Data/hora de criação do registro, gerada automaticamente.';
COMMENT ON COLUMN veiculos.updated_at IS 'Data/hora da última atualização do registro, gerada automaticamente.';

-- ---- Produto ----

CREATE TABLE IF NOT EXISTS produtos (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    codigo         BIGINT GENERATED ALWAYS AS IDENTITY,
    nome           TEXT NOT NULL,
    descricao      TEXT NOT NULL,
    valor_unitario NUMERIC(12, 2) NOT NULL CHECK (valor_unitario >= 0),
    estoque_atual  INTEGER NOT NULL DEFAULT 0 CHECK (estoque_atual >= 0),
    tipo           produto_tipo NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE produtos IS 'Peça de reposição ou insumo de consumo.';
COMMENT ON COLUMN produtos.id IS 'Identificador técnico seguro, gerado pelo sistema.';
COMMENT ON COLUMN produtos.codigo IS 'Identificador legível/sequencial exibido ao usuário.';
COMMENT ON COLUMN produtos.nome IS 'Nome do produto.';
COMMENT ON COLUMN produtos.descricao IS 'Descrição detalhada do produto.';
COMMENT ON COLUMN produtos.valor_unitario IS 'Preço de venda por unidade do produto.';
COMMENT ON COLUMN produtos.estoque_atual IS 'Quantidade disponível em estoque no momento da consulta.';
COMMENT ON COLUMN produtos.tipo IS 'Categoria do produto: PECA (peça de reposição) ou INSUMO (material de consumo).';
COMMENT ON COLUMN produtos.created_at IS 'Data/hora de criação do registro, gerada automaticamente.';
COMMENT ON COLUMN produtos.updated_at IS 'Data/hora da última atualização do registro, gerada automaticamente.';

-- ---- Servico ----

CREATE TABLE IF NOT EXISTS servicos (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    codigo         BIGINT GENERATED ALWAYS AS IDENTITY,
    nome           TEXT NOT NULL,
    descricao      TEXT NOT NULL,
    valor          NUMERIC(12, 2) NOT NULL CHECK (valor >= 0),
    tempo_estimado INTEGER CHECK (tempo_estimado IS NULL OR tempo_estimado > 0),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE servicos IS 'Serviço oferecido pela oficina (ex: troca de óleo, alinhamento).';
COMMENT ON COLUMN servicos.id IS 'Identificador técnico seguro, gerado pelo sistema.';
COMMENT ON COLUMN servicos.codigo IS 'Identificador legível/sequencial exibido ao usuário.';
COMMENT ON COLUMN servicos.nome IS 'Nome do serviço oferecido (ex: troca de óleo, alinhamento).';
COMMENT ON COLUMN servicos.descricao IS 'Descrição detalhada do que o serviço compreende.';
COMMENT ON COLUMN servicos.valor IS 'Preço cobrado pela execução do serviço.';
COMMENT ON COLUMN servicos.tempo_estimado IS 'Tempo estimado de execução, em minutos. Campo opcional.';
COMMENT ON COLUMN servicos.created_at IS 'Data/hora de criação do registro, gerada automaticamente.';
COMMENT ON COLUMN servicos.updated_at IS 'Data/hora da última atualização do registro, gerada automaticamente.';

-- ---- OrdemDeServico ----

CREATE TABLE IF NOT EXISTS ordens_servico (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    codigo        BIGINT GENERATED ALWAYS AS IDENTITY,
    cliente_id    UUID NOT NULL REFERENCES clientes (id) ON DELETE RESTRICT,
    veiculo_id    UUID NOT NULL REFERENCES veiculos (id) ON DELETE RESTRICT,
    data_criacao  TIMESTAMPTZ NOT NULL DEFAULT now(),
    status        ordem_servico_status NOT NULL DEFAULT 'RECEBIDA',
    observacoes   TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE ordens_servico IS 'Ordem de serviço aberta para um veículo/cliente.';
COMMENT ON COLUMN ordens_servico.id IS 'Identificador técnico seguro, gerado pelo sistema.';
COMMENT ON COLUMN ordens_servico.codigo IS 'Identificador legível/sequencial da OS, usado para consulta e comunicação com o cliente.';
COMMENT ON COLUMN ordens_servico.cliente_id IS 'Referência ao Cliente solicitante do serviço.';
COMMENT ON COLUMN ordens_servico.veiculo_id IS 'Referência ao Veiculo que está sendo atendido.';
COMMENT ON COLUMN ordens_servico.data_criacao IS 'Data/hora de abertura da ordem de serviço.';
COMMENT ON COLUMN ordens_servico.status IS 'RECEBIDA -> EM_DIAGNOSTICO -> AGUARDANDO_APROVACAO -> EM_EXECUCAO -> FINALIZADA -> ENTREGUE.';
COMMENT ON COLUMN ordens_servico.observacoes IS 'Anotações livres sobre o atendimento (ex: relato do cliente, condições do veículo).';
COMMENT ON COLUMN ordens_servico.created_at IS 'Data/hora de criação do registro, gerada automaticamente.';
COMMENT ON COLUMN ordens_servico.updated_at IS 'Data/hora da última atualização do registro, gerada automaticamente.';
-- Nota: o doc modela OrdemDeServico.orcamento <-> Orcamento.ordemServicoId como
-- referência 1:1 circular. A FK física fica só em orcamentos.ordem_servico_id
-- (UNIQUE), evitando duas FKs redundantes apontando uma pra outra; o orçamento
-- de uma OS é obtido via esse índice reverso.

-- ---- Orcamento ----

CREATE TABLE IF NOT EXISTS orcamentos (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    codigo            BIGINT GENERATED ALWAYS AS IDENTITY,
    ordem_servico_id  UUID NOT NULL UNIQUE REFERENCES ordens_servico (id) ON DELETE CASCADE,
    valor             NUMERIC(12, 2) NOT NULL CHECK (valor >= 0),
    status            orcamento_status NOT NULL DEFAULT 'PENDENTE',
    data_geracao      TIMESTAMPTZ NOT NULL DEFAULT now(),
    data_resposta     TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (data_resposta IS NULL OR data_resposta >= data_geracao)
);

COMMENT ON TABLE orcamentos IS 'Orçamento vinculado 1:1 a uma ordem de serviço.';
COMMENT ON COLUMN orcamentos.id IS 'Identificador técnico seguro, gerado pelo sistema.';
COMMENT ON COLUMN orcamentos.codigo IS 'Identificador legível/sequencial do orçamento.';
COMMENT ON COLUMN orcamentos.ordem_servico_id IS 'Referência à OrdemDeServico à qual este orçamento pertence.';
COMMENT ON COLUMN orcamentos.valor IS 'Valor total do orçamento (soma de produtos e serviços).';
COMMENT ON COLUMN orcamentos.status IS 'Situação do orçamento: PENDENTE (aguardando resposta do cliente), APROVADO (cliente aceitou) ou REPROVADO (cliente recusou).';
COMMENT ON COLUMN orcamentos.data_geracao IS 'Data/hora em que o orçamento foi gerado e enviado ao cliente.';
COMMENT ON COLUMN orcamentos.data_resposta IS 'Data/hora em que o cliente respondeu (aprovou/reprovou). Campo opcional, preenchido só após resposta.';
COMMENT ON COLUMN orcamentos.created_at IS 'Data/hora de criação do registro, gerada automaticamente.';
COMMENT ON COLUMN orcamentos.updated_at IS 'Data/hora da última atualização do registro, gerada automaticamente.';

-- ---- Orcamento <-> Produto / Servico (N:N) ----
-- O doc modela Orcamento.products/services como arrays; fisicamente isso vira
-- tabelas de junção. valor_unitario_aplicado/valor_aplicado fazem snapshot do
-- preço no momento do orçamento, já que o preço do catálogo pode mudar depois.

CREATE TABLE IF NOT EXISTS orcamento_produtos (
    orcamento_id           UUID NOT NULL REFERENCES orcamentos (id) ON DELETE CASCADE,
    produto_id             UUID NOT NULL REFERENCES produtos (id) ON DELETE RESTRICT,
    quantidade             INTEGER NOT NULL CHECK (quantidade > 0),
    valor_unitario_aplicado NUMERIC(12, 2) NOT NULL CHECK (valor_unitario_aplicado >= 0),
    PRIMARY KEY (orcamento_id, produto_id)
);

COMMENT ON TABLE orcamento_produtos IS 'Produtos/peças incluídos em um orçamento, com preço aplicado no momento.';
COMMENT ON COLUMN orcamento_produtos.orcamento_id IS 'Referência ao Orcamento ao qual este item pertence.';
COMMENT ON COLUMN orcamento_produtos.produto_id IS 'Referência ao Produto incluído no orçamento.';
COMMENT ON COLUMN orcamento_produtos.quantidade IS 'Quantidade do produto incluída no orçamento.';
COMMENT ON COLUMN orcamento_produtos.valor_unitario_aplicado IS 'Preço unitário do produto no momento em que foi incluído no orçamento (snapshot).';

CREATE TABLE IF NOT EXISTS orcamento_servicos (
    orcamento_id     UUID NOT NULL REFERENCES orcamentos (id) ON DELETE CASCADE,
    servico_id       UUID NOT NULL REFERENCES servicos (id) ON DELETE RESTRICT,
    valor_aplicado   NUMERIC(12, 2) NOT NULL CHECK (valor_aplicado >= 0),
    PRIMARY KEY (orcamento_id, servico_id)
);

COMMENT ON TABLE orcamento_servicos IS 'Serviços incluídos em um orçamento, com preço aplicado no momento.';
COMMENT ON COLUMN orcamento_servicos.orcamento_id IS 'Referência ao Orcamento ao qual este item pertence.';
COMMENT ON COLUMN orcamento_servicos.servico_id IS 'Referência ao Servico incluído no orçamento.';
COMMENT ON COLUMN orcamento_servicos.valor_aplicado IS 'Preço do serviço no momento em que foi incluído no orçamento (snapshot).';

-- ---- HistoricoOrdemServico ----

CREATE TABLE IF NOT EXISTS historico_ordem_servico (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ordem_servico_id UUID NOT NULL REFERENCES ordens_servico (id) ON DELETE CASCADE,
    data_hora        TIMESTAMPTZ NOT NULL DEFAULT now(),
    evento           historico_evento NOT NULL,
    descricao        TEXT NOT NULL,
    status_old       ordem_servico_status NOT NULL,
    status_new       ordem_servico_status NOT NULL
);

COMMENT ON TABLE historico_ordem_servico IS 'Trilha de eventos e mudanças de status de uma OS, para auditoria e rastreabilidade.';
COMMENT ON COLUMN historico_ordem_servico.id IS 'Identificador técnico do registro de histórico.';
COMMENT ON COLUMN historico_ordem_servico.ordem_servico_id IS 'Referência à OrdemDeServico à qual este evento pertence.';
COMMENT ON COLUMN historico_ordem_servico.data_hora IS 'Data/hora em que o evento ocorreu.';
COMMENT ON COLUMN historico_ordem_servico.evento IS 'Tipo de evento registrado: criacao, aprovacao, finalizacao ou cancelamento.';
COMMENT ON COLUMN historico_ordem_servico.descricao IS 'Detalhamento do que ocorreu no evento.';
COMMENT ON COLUMN historico_ordem_servico.status_old IS 'Status da OS imediatamente antes do evento.';
COMMENT ON COLUMN historico_ordem_servico.status_new IS 'Status da OS imediatamente após o evento.';

-- ---- AuditServices ----

CREATE TABLE IF NOT EXISTS audit_services (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ordem_servico_id UUID NOT NULL REFERENCES ordens_servico (id) ON DELETE CASCADE,
    service_id       UUID NOT NULL REFERENCES servicos (id) ON DELETE RESTRICT,
    data_hora        TIMESTAMPTZ NOT NULL DEFAULT now(),
    evento           audit_evento NOT NULL
);

COMMENT ON TABLE audit_services IS 'Início e fim da execução de cada serviço dentro de uma OS, para controle de tempo/produtividade.';
COMMENT ON COLUMN audit_services.id IS 'Identificador técnico do registro de auditoria.';
COMMENT ON COLUMN audit_services.ordem_servico_id IS 'Referência à OrdemDeServico em execução.';
COMMENT ON COLUMN audit_services.service_id IS 'Referência ao Servico sendo executado.';
COMMENT ON COLUMN audit_services.data_hora IS 'Data/hora em que o evento foi registrado.';
COMMENT ON COLUMN audit_services.evento IS 'Marco do evento: inicio (início da execução) ou fim (conclusão).';

-- =============================================================================
-- ==== Índices ====
-- =============================================================================

-- ---- Códigos legíveis (busca por número exibido ao usuário) ----

CREATE UNIQUE INDEX IF NOT EXISTS ux_clientes_codigo ON clientes (codigo);
CREATE UNIQUE INDEX IF NOT EXISTS ux_veiculos_codigo ON veiculos (codigo);
CREATE UNIQUE INDEX IF NOT EXISTS ux_produtos_codigo ON produtos (codigo);
CREATE UNIQUE INDEX IF NOT EXISTS ux_servicos_codigo ON servicos (codigo);
CREATE UNIQUE INDEX IF NOT EXISTS ux_ordens_servico_codigo ON ordens_servico (codigo);
CREATE UNIQUE INDEX IF NOT EXISTS ux_orcamentos_codigo ON orcamentos (codigo);

-- ---- Unicidade de negócio ----

CREATE UNIQUE INDEX IF NOT EXISTS ux_clientes_documento ON clientes (documento);
CREATE UNIQUE INDEX IF NOT EXISTS ux_veiculos_placa ON veiculos (placa);
-- Único parcial: e-mail é opcional, mas quando presente não pode repetir.
CREATE UNIQUE INDEX IF NOT EXISTS ux_clientes_email ON clientes (email) WHERE email IS NOT NULL;

-- ---- Foreign keys (Postgres não indexa FK automaticamente) ----

CREATE INDEX IF NOT EXISTS ix_veiculos_cliente_id ON veiculos (cliente_id);
CREATE INDEX IF NOT EXISTS ix_ordens_servico_cliente_id ON ordens_servico (cliente_id);
CREATE INDEX IF NOT EXISTS ix_ordens_servico_veiculo_id ON ordens_servico (veiculo_id);
CREATE INDEX IF NOT EXISTS ix_orcamento_produtos_produto_id ON orcamento_produtos (produto_id);
CREATE INDEX IF NOT EXISTS ix_orcamento_servicos_servico_id ON orcamento_servicos (servico_id);
CREATE INDEX IF NOT EXISTS ix_historico_os_ordem_servico_id ON historico_ordem_servico (ordem_servico_id);
CREATE INDEX IF NOT EXISTS ix_audit_services_ordem_servico_id ON audit_services (ordem_servico_id);
CREATE INDEX IF NOT EXISTS ix_audit_services_service_id ON audit_services (service_id);
-- orcamentos.ordem_servico_id já tem índice único (UNIQUE acima cria índice).

-- ---- Consultas frequentes de oficina ----

-- Listagem de OS por cliente, mais recentes primeiro.
CREATE INDEX IF NOT EXISTS ix_ordens_servico_cliente_data
    ON ordens_servico (cliente_id, data_criacao DESC);

-- "Quais OS estão em andamento" é a consulta mais comum do painel operacional;
-- índice parcial mantém esse subconjunto pequeno e rápido, ignorando OS já
-- finalizadas/entregues (a maioria histórica).
CREATE INDEX IF NOT EXISTS ix_ordens_servico_status_ativas
    ON ordens_servico (status, data_criacao DESC)
    WHERE status NOT IN ('FINALIZADA', 'ENTREGUE');

-- Filtro por tipo de produto (PECA/INSUMO) em telas de catálogo/estoque.
CREATE INDEX IF NOT EXISTS ix_produtos_tipo ON produtos (tipo);

-- Alerta de reposição: produtos com estoque baixo.
CREATE INDEX IF NOT EXISTS ix_produtos_estoque_baixo
    ON produtos (estoque_atual)
    WHERE estoque_atual < 10;

-- Orçamentos pendentes de resposta do cliente.
CREATE INDEX IF NOT EXISTS ix_orcamentos_status_pendente
    ON orcamentos (status, data_geracao DESC)
    WHERE status = 'PENDENTE';

-- Auditoria de execução de serviço por OS.
CREATE INDEX IF NOT EXISTS ix_audit_services_os_service
    ON audit_services (ordem_servico_id, service_id, data_hora);

-- ---- Busca textual (opcional) ----
-- Requer a extensão pg_trgm (comentada no topo do arquivo). Acelera buscas
-- com ILIKE '%termo%' em nome de cliente e placa de veículo.
-- CREATE INDEX IF NOT EXISTS ix_clientes_nome_trgm ON clientes USING GIN (nome gin_trgm_ops);
-- CREATE INDEX IF NOT EXISTS ix_veiculos_placa_trgm ON veiculos USING GIN (placa gin_trgm_ops);

-- =============================================================================
-- ==== Trigger utilitária para updated_at ====
-- =============================================================================

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
DECLARE
    tbl TEXT;
BEGIN
    FOREACH tbl IN ARRAY ARRAY['clientes', 'veiculos', 'produtos', 'servicos', 'ordens_servico', 'orcamentos']
    LOOP
        EXECUTE format(
            'DROP TRIGGER IF EXISTS trg_set_updated_at ON %I; '
            'CREATE TRIGGER trg_set_updated_at BEFORE UPDATE ON %I '
            'FOR EACH ROW EXECUTE FUNCTION set_updated_at();',
            tbl, tbl
        );
    END LOOP;
END $$;
