# 🚗 Automotive Workshop API

> **Tech Challenge — Fase 1 | Pós-Tech FIAP (Software Architecture)**  
> REST API robusta para gerenciamento completo do ciclo de atendimento em uma oficina mecânica: cadastro de clientes e veículos, catálogo de serviços, controle de peças/insumos com estoque, abertura de Ordens de Serviço (OS), diagnóstico, composição e aprovação de orçamentos, execução de serviços com auditoria, baixa de peças, acompanhamento público pelo cliente e métricas operacionais.

---

## 👥 Integrantes do Grupo

- **Jean Ferreira dos Santos Cruz Junior** — RM376169
- **João Victor dos Santos Cerqueira** — RM376742
- **Giovane Kenuy Soares Colognesi Rubira Cardona** — RM376443
- **Augusto José Rizzi** — RM376822
- **Lucas Guimarães Fabris** — RM376444

---

## 📌 Sumário

1. [Visão Geral do Projeto](#-visão-geral-do-projeto)
2. [Ciclo de Vida da Ordem de Serviço](#-ciclo-de-vida-da-ordem-de-serviço)
3. [Arquitetura e Padrões](#-arquitetura-e-padrões)
4. [Tecnologias, Bibliotecas e Dependências](#-tecnologias-bibliotecas-e-dependências)
5. [Como Rodar o Projeto com Docker (Passo a Passo)](#-como-rodar-o-projeto-com-docker-passo-a-passo)
6. [Credenciais de Teste e Dados Iniciais (Seed)](#-credenciais-de-teste-e-dados-iniciais-seed)
7. [Documentação dos Endpoints da API](#-documentação-dos-endpoints-da-api)
8. [Coleções para Testes (Bruno, Postman e Insomnia)](#-coleções-para-testes-bruno-postman-e-insomnia)
9. [Testes Automatizados, Cobertura e Segurança](#-testes-automatizados-cobertura-e-segurança)
10. [Estrutura de Pastas do Projeto](#-estrutura-de-pastas-do-projeto)

---

## 🎯 Visão Geral do Projeto

A **Automotive Workshop API** foi desenvolvida para solucionar os principais desafios operacionais de oficinas mecânicas: desorganização no fluxo de atendimento, falta de rastreabilidade do status dos veículos, controle inadequado de peças e insumos em estoque, perda de histórico de orçamentos e dificuldade de comunicação com o cliente.

### Principais Capacidades da Aplicação:
- **Gestão de Clientes**: Suporte a Pessoa Física (CPF) e Pessoa Jurídica (CNPJ, incluindo o formato alfanumérico em vigor desde 2026), com validação rigorosa de dígitos verificadores e inativação lógica.
- **Gestão de Veículos**: Vínculo obrigatório com cliente ativo, suporte a placas no padrão antigo e padrão Mercosul.
- **Catálogo de Serviços e Produtos**: Cadastro de serviços (com preço e tempo estimado) e produtos (`PART` - peças de reposição ou `SUPPLY` - insumos consumíveis) com saldo de estoque e inativação lógica.
- **Gestão de Estoque com Auditoria**: Ledger imutável de movimentações de estoque (`ENTRY`, `EXIT`, `LOSS`, `REVERSAL`), permitindo ajustes manuais, baixa automática vinculada à OS e estorno rastreável com atualização atômica de saldo.
- **Ciclo Completo de Ordem de Serviço (OS)**: Abertura, início de diagnóstico, composição de orçamento com fotografia congelada de preços/descrições, envio de orçamento ao cliente, aprovação ou reprovação, início e conclusão de execuções de serviços individuais, finalização e entrega.
- **Portal de Acompanhamento Público Seguro**: O cliente pode consultar o status do seu veículo e aprovar/reprovar o orçamento através de uma projeção de dados segura, sem necessidade de login administrativo, utilizando um token exclusivo da OS (`X-Tracking-Token`) sem expor dados sensíveis (PII).
- **Métricas Operacionais**: Cálculo do tempo médio de execução por serviço com suporte a filtros de período.

---

## 🔄 Ciclo de Vida da Ordem de Serviço

Toda Ordem de Serviço transita rigorosamente pelos seguintes estados:

```
                  ┌─────────────────┐
                  │    RECEIVED     │ (Veículo recebido na oficina)
                  └────────┬────────┘
                           │ Iniciar Diagnóstico
                           ▼
                  ┌─────────────────┐
                  │  IN_DIAGNOSIS   │ (Em diagnóstico e composição do orçamento)
                  └────────┬────────┘
                           │ Enviar Orçamento
                           ▼
                  ┌─────────────────┐
                  │AWAITING_APPROVAL│ (Aguardando decisão do cliente)
                  └────┬───────┬────┘
      Cliente Aprova   │       │   Cliente Reprova
    ┌──────────────────┘       └──────────────────┐
    ▼                                             ▼
┌───────────────┐                         ┌───────────────┐
│  IN_PROGRESS  │ (Serviços em execução)  │   CANCELED    │ (OS cancelada)
└───────┬───────┘                         └───────────────┘
        │ Todas execuções concluídas + Finalização
        ▼
┌───────────────┐
│   COMPLETED   │ (Serviços finalizados)
└───────┬───────┘
        │ Entrega do Veículo
        ▼
┌───────────────┐
│   DELIVERED   │ (Veículo entregue ao cliente)
└───────────────┘
```

---

## 🏗 Arquitetura e Padrões

O projeto foi construído seguindo os princípios de **Vertical Slice Architecture** (monólito modular orientado a funcionalidades):

- **Fatias Verticais Independentes (`internal/features/`)**: Cada funcionalidade de negócio (`auth`, `customer`, `vehicle`, `service-catalog`, `product`, `service-order`, `service-order-tracking`) encapsula suas próprias camadas de **Handler (HTTP)**, **Service (Regras de Domínio/Aplicação)**, **Repository (Acesso a Dados SQL)** e **Models/DTOs**.
- **Desacoplamento entre Features**: Uma feature nunca importa diretamente outra feature. Quando é necessária comunicação ou validação cruzada (ex: criação de veículo validando se o cliente existe e está ativo), é utilizada a técnica de **Dependency Inversion** com *Adapters* definidos no ponto de composição (`cmd/api/main.go`).
- **Núcleo Compartilhado (`internal/shared/`)**: Reservado estritamente para utilitários agnósticos de domínio: pool de conexões com o PostgreSQL, middlewares de autenticação, validação algorítmica de documentos, emissão/verificação de tokens e tratamento padronizado de erros.
- **Composition Root Limpo (`cmd/api/main.go`)**: Responsável unicamente por ler configurações, abrir conexões, instanciar as dependências, montar os adaptadores e registrar as rotas no multiplexador HTTP nativo.

---

## 🧰 Tecnologias, Bibliotecas e Dependências

A aplicação foi projetada com foco em alta performance, baixo acoplamento e dependências externas mínimas e deliberadas:

| Tecnologia / Dependência | Versão | Finalidade e Justificativa |
| :--- | :---: | :--- |
| **Go** | `1.25` | Linguagem compilada com excelente performance, tipagem estática e segurança de memória. |
| **`net/http` (stdlib)** | Nativo | Roteamento HTTP nativo utilizando as melhorias de padrões por método do Go (`http.ServeMux`), dispensando frameworks externos como Gin ou Fiber. |
| **PostgreSQL** | `16` | Banco de dados relacional robusto com uso de UUID nativo (`pgcrypto`), ENUMs nativos, índices otimizados e transações ACID. |
| **`github.com/jackc/pgx/v5`** | `v5.10.0` | Driver e pool de conexões (`pgxpool`) de alta performance para PostgreSQL em Go, sem overhead de ORMs pesados. |
| **`github.com/golang-jwt/jwt/v5`** | `v5.2.2` | Geração, assinatura criptográfica (HS256) e validação de tokens JWT para autenticação administrativa. |
| **`golang.org/x/crypto/bcrypt`** | `v0.55.0` | Hashing unidirecional com *salt* automático para armazenamento seguro de senhas no banco. |
| **`github.com/google/uuid`** | `v1.6.0` | Manipulação e validação segura de identificadores únicos universais (UUID v4). |
| **`github.com/stretchr/testify`** | `v1.11.1` | Biblioteca de asserções fluídas (`assert`, `require`) para garantir legibilidade nos testes unitários e de integração. |
| **`gosec`** | `v2.28.0` | Scanner SAST (Static Application Security Testing) para análise de segurança e boas práticas no código Go. |
| **`govulncheck`** | `v1.7.0` | Ferramenta oficial do time do Go para detecção de vulnerabilidades conhecidas em dependências e stdlib. |
| **Docker & Docker Compose** | — | Conteinerização completa da aplicação, banco de dados, cliente web do banco (Adminer) e documentação interativa (Swagger UI). |

---

## 🚀 Como Rodar o Projeto com Docker (Passo a Passo)

### Pré-requisitos
- [Docker](https://docs.docker.com/get-docker/) instalado e em execução.
- [Docker Compose](https://docs.docker.com/compose/install/) (ou plugin `docker compose` v2).
- [Git](https://git-scm.com/) para clonar o repositório.
- *(Opcional)* `make` para executar comandos simplificados via Makefile.

---

### Passo 1: Clonar o Repositório
```bash
git clone https://github.com/Os-Fiapudos/automotive-workshop-api.git
cd automotive-workshop-api
```

---

### Passo 2: Configurar o Arquivo de Variáveis de Ambiente
Copie o arquivo de exemplo `.env.example` para `.env`:

```bash
cp .env.example .env
```

> **Nota de Configuração**: O arquivo `.env` já vem pré-configurado com portas e credenciais funcionais para desenvolvimento:
> - `POSTGRES_USER=workshop`
> - `POSTGRES_PASSWORD=workshop`
> - `POSTGRES_DB=automotive_workshop`
> - `POSTGRES_PORT=5432`
> - `API_PORT=8080`
> - `ADMINER_PORT=8081`
> - `SWAGGER_UI_PORT=8082`
> - `JWT_SECRET=change-me-dev-only-please-generate-a-real-random-value`
> - `JWT_TTL=1h`

---

### Passo 3: Iniciar os Containers

Execute o comando do Docker Compose para construir a imagem da API e subir todos os serviços em segundo plano:

```bash
docker compose up -d
```

*Ou utilizando o Makefile:*
```bash
make start
```

O Docker iniciará 4 serviços automaticamente:
1. **`db`** (PostgreSQL 16): Aplica os scripts de schema (`docs/schema.sql`) e carga inicial (`docs/seed.sql`) na inicialização do volume.
2. **`api`** (Go 1.25): Constrói e inicializa a API na porta `8080` assim que o banco estiver saudável (`healthcheck`).
3. **`swagger-ui`**: Interface interativa com a documentação OpenAPI 3.0 na porta `8082`.
4. **`adminer`**: Interface web para navegação e consultas no PostgreSQL na porta `8081`.

---

### Passo 4: Verificar a Execução dos Serviços

Confira se todos os containers estão saudáveis:

```bash
docker compose ps
```

Teste o endpoint de verificação de saúde da API:

```bash
curl http://localhost:8080/health
```
**Resposta esperada:**
```json
{"status":"ok"}
```

---

### 🌐 URLs e Acessos Disponíveis

| Serviço | URL de Acesso | Descrição |
| :--- | :--- | :--- |
| **API REST** | `http://localhost:8080` | Ponto de entrada da API. |
| **Swagger UI** | `http://localhost:8082` | Documentação interativa completa (OpenAPI 3.0). |
| **Adminer (DB Web)** | `http://localhost:8081` | Gerenciador do Postgres (Servidor: `db`, Usuário: `workshop`, Senha: `workshop`, Base: `automotive_workshop`). |
| **Postgres Database** | `localhost:5432` | Conexão direta com o banco de dados. |

---

### 🛠 Comandos Úteis do Makefile e Docker

| Ação | Comando com Makefile | Comando equivalente com Docker Compose |
| :--- | :--- | :--- |
| **Subir tudo** | `make start` | `docker compose up -d` |
| **Ver logs da API e serviços** | `make logs` | `docker compose logs -f` |
| **Parar os containers** | `make stop` | `docker compose stop` |
| **Derrubar containers** | `make down` | `docker compose down` |
| **Resetar banco do zero (apaga dados + reseed)** | `make reset` | `docker compose down -v && docker compose up -d` |
| **Reaplicar dados de seed** | `make seed` | `docker compose cp docs/seed.sql db:/tmp/seed.sql && docker compose exec db psql -U workshop -d automotive_workshop -f /tmp/seed.sql` |
| **Testar login via curl** | `make login` | *(via curl no endpoint `/api/v1/auth/login`)* |
| **Abrir terminal psql no banco** | `make psql` | `docker compose exec db psql -U workshop -d automotive_workshop` |

---

## 🔑 Credenciais de Teste e Dados Iniciais (Seed)

O banco é inicializado automaticamente com dados de exemplo ([docs/seed.sql](docs/seed.sql)) contendo clientes, veículos, produtos, serviços, ordens de serviço e usuários administrativos:

| Usuário / E-mail | Senha | Perfil |
| :--- | :--- | :--- |
| `admin@workshop.local` | `admin123` | Administrador da Oficina |
| `soat-architecture@workshop.local` | `soat-architecture` | Administrador / Avaliação SOAT |

---

## 📖 Documentação dos Endpoints da API

A API possui **47 rotas registradas**, todas com 100% de paridade no OpenAPI 3.0 ([docs/openapi.yaml](docs/openapi.yaml)).

### Modelos de Autenticação:
- 🔒 **`JWT (Bearer)`**: Requer cabeçalho `Authorization: Bearer <token>` obtido através do login administrativo.
- 🎟️ **`X-Tracking-Token`**: Rota pública do cliente; não aceita JWT administrativo e exige o cabeçalho `X-Tracking-Token: <token>` emitido exclusivamente na abertura da OS.
- 🟢 **`Pública`**: Rotas abertas sem necessidade de credenciais.

---

### 1. Saúde e Autenticação
| Método | Endpoint | Proteção | Descrição |
| :--- | :--- | :---: | :--- |
| `GET` | `/health` | 🟢 Pública | Verificação de liveness e status da API. |
| `POST` | `/api/v1/auth/login` | 🟢 Pública | Autentica usuário administrativo e retorna o token JWT. |
| `GET` | `/api/v1/auth/me` | 🔒 JWT | Retorna os dados do usuário autenticado. |

### 2. Gestão de Clientes (`/customers`)
| Método | Endpoint | Proteção | Descrição |
| :--- | :--- | :---: | :--- |
| `POST` | `/api/v1/customers` | 🔒 JWT | Cadastra um novo cliente (CPF ou CNPJ validado). |
| `GET` | `/api/v1/customers` | 🔒 JWT | Lista clientes de forma paginada (`page`, `pageSize`). |
| `GET` | `/api/v1/customers/{id}` | 🔒 JWT | Busca cliente por UUID. |
| `GET` | `/api/v1/customers/document/{document}` | 🔒 JWT | Busca cliente por CPF ou CNPJ normalizado. |
| `PATCH` | `/api/v1/customers/{id}` | 🔒 JWT | Atualiza dados cadastrais de um cliente. |
| `DELETE` | `/api/v1/customers/{id}` | 🔒 JWT | Inativação lógica do cliente (`status = INACTIVE`). |

### 3. Gestão de Veículos (`/vehicles`)
| Método | Endpoint | Proteção | Descrição |
| :--- | :--- | :---: | :--- |
| `POST` | `/api/v1/vehicles` | 🔒 JWT | Cadastra veículo vinculado a um cliente ativo (placa antiga ou Mercosul). |
| `GET` | `/api/v1/vehicles` | 🔒 JWT | Lista veículos de forma paginada. |
| `GET` | `/api/v1/vehicles/{id}` | 🔒 JWT | Busca veículo por UUID. |
| `GET` | `/api/v1/vehicles/plate/{plate}` | 🔒 JWT | Busca veículo por placa normalizada. |
| `GET` | `/api/v1/vehicles/customer/{customerId}` | 🔒 JWT | Lista veículos vinculados a um cliente específico. |
| `PATCH` | `/api/v1/vehicles/{id}` | 🔒 JWT | Atualiza marca, modelo, ano ou cor do veículo. |
| `DELETE` | `/api/v1/vehicles/{id}` | 🔒 JWT | Inativação lógica do veículo (`status = INACTIVE`). |

### 4. Catálogo de Serviços (`/services`)
| Método | Endpoint | Proteção | Descrição |
| :--- | :--- | :---: | :--- |
| `POST` | `/api/v1/services` | 🔒 JWT | Cria um serviço no catálogo (preço obrigatório, tempo estimado opcional). |
| `GET` | `/api/v1/services` | 🔒 JWT | Lista serviços ativos (suporta filtro `?active=true/false`). |
| `GET` | `/api/v1/services/{id}` | 🔒 JWT | Busca serviço por UUID. |
| `PATCH` | `/api/v1/services/{id}` | 🔒 JWT | Atualiza preço, descrição, nome ou tempo estimado. |
| `DELETE` | `/api/v1/services/{id}` | 🔒 JWT | Inativação lógica do serviço (`active = false`). |

### 5. Catálogo de Produtos e Estoque (`/products`)
| Método | Endpoint | Proteção | Descrição |
| :--- | :--- | :---: | :--- |
| `POST` | `/api/v1/products` | 🔒 JWT | Cadastra peça (`PART`) ou insumo (`SUPPLY`). |
| `GET` | `/api/v1/products` | 🔒 JWT | Lista produtos com filtros (`page`, `pageSize`, `type`, `status`). |
| `GET` | `/api/v1/products/{id}` | 🔒 JWT | Busca produto por UUID. |
| `PATCH` | `/api/v1/products/{id}` | 🔒 JWT | Atualiza dados cadastrais e preço do produto. |
| `DELETE` | `/api/v1/products/{id}` | 🔒 JWT | Inativação lógica do produto (`status = INACTIVE`). |
| `POST` | `/api/v1/products/{id}/stock/adjustments` | 🔒 JWT | Realiza ajuste manual de estoque (`ENTRY`, `EXIT`, `LOSS`) com auditoria. |
| `GET` | `/api/v1/products/{id}/stock` | 🔒 JWT | Consulta saldo atual de estoque do produto. |
| `GET` | `/api/v1/products/{id}/movements` | 🔒 JWT | Lista o histórico de movimentações de estoque do produto. |

### 6. Ordens de Serviço — Ciclo de Atendimento (`/service-orders`)
| Método | Endpoint | Proteção | Descrição |
| :--- | :--- | :---: | :--- |
| `POST` | `/api/v1/service-orders` | 🔒 JWT | Abre uma nova OS com status `RECEIVED` e emite o `trackingToken`. |
| `GET` | `/api/v1/service-orders` | 🔒 JWT | Lista OSs com filtros avançados (`code`, `status`, `document`, `licensePlate`, período). |
| `GET` | `/api/v1/service-orders/{id}` | 🔒 JWT | Detalha a OS completa (aceita UUID ou código sequencial numérico). |
| `POST` | `/api/v1/service-orders/{id}/diagnosis` | 🔒 JWT | Inicia o diagnóstico da OS (`RECEIVED → IN_DIAGNOSIS`). |
| `PUT` | `/api/v1/service-orders/{id}/quote` | 🔒 JWT | Compõe/versiona o orçamento com itens de produtos e serviços. |
| `GET` | `/api/v1/service-orders/{id}/quote` | 🔒 JWT | Consulta o orçamento vinculado à OS. |
| `POST` | `/api/v1/service-orders/{id}/quote/send` | 🔒 JWT | Registra o envio do orçamento ao cliente (`IN_DIAGNOSIS → AWAITING_APPROVAL`). |
| `POST` | `/api/v1/service-orders/{id}/executions` | 🔒 JWT | Registra o início de execução de um serviço específico da OS. |
| `POST` | `/api/v1/service-orders/{id}/executions/{executionId}/finish` | 🔒 JWT | Registra a conclusão da execução do serviço. |
| `POST` | `/api/v1/service-orders/{id}/finalize` | 🔒 JWT | Finaliza a OS (`IN_PROGRESS → COMPLETED`) após concluir as execuções. |
| `POST` | `/api/v1/service-orders/{id}/deliver` | 🔒 JWT | Registra a entrega do veículo ao cliente (`COMPLETED → DELIVERED`). |
| `POST` | `/api/v1/service-orders/{id}/stock-movements` | 🔒 JWT | Dá baixa no estoque de peças/insumos consumidos na OS. |
| `GET` | `/api/v1/service-orders/{id}/stock-movements` | 🔒 JWT | Lista as peças e movimentações de estoque vinculadas à OS. |
| `POST` | `/api/v1/service-orders/{id}/stock-movements/{movementId}/reversal` | 🔒 JWT | Estorna uma movimentação de estoque da OS, restaurando o saldo. |
| `GET` | `/api/v1/service-orders/metrics/average-execution-time` | 🔒 JWT | Retorna a métrica de tempo médio de execução agrupado por serviço. |

### 7. Acompanhamento Público e Decisão pelo Cliente (`/acompanhamento`)
| Método | Endpoint | Proteção | Descrição |
| :--- | :--- | :---: | :--- |
| `GET` | `/api/v1/acompanhamento/{codigo}` | 🎟️ Token | Consulta pública reduzida da OS (não expõe PII nem dados internos). |
| `POST` | `/api/v1/acompanhamento/{codigo}/orcamento/aprovar` | 🎟️ Token | Cliente aprova o orçamento (`AWAITING_APPROVAL → IN_PROGRESS`). |
| `POST` | `/api/v1/acompanhamento/{codigo}/orcamento/reprovar` | 🎟️ Token | Cliente reprova o orçamento (`AWAITING_APPROVAL → CANCELED`). |

---

## 📬 Coleções para Testes (Bruno, Postman e Insomnia)

O projeto disponibiliza coleções completas e pré-configuradas para todas as 47 rotas, com encadeamento automático de variáveis:

1. **Bruno (Recomendado)**: Localizado na pasta [`bruno/`](bruno). Abra o diretório diretamente no aplicativo Bruno. Selecione o ambiente `Local`.
2. **Postman**: Importe o arquivo [`docs/postman-collection.json`](docs/postman-collection.json).
3. **Insomnia**: Importe o arquivo [`docs/insomnia-collection.json`](docs/insomnia-collection.json).

### 💡 Fluxo de Execução Encadeado (sem copiar e colar IDs):
1. Execute **`Auth -> Login`**: o script armazena o JWT na variável `token`.
2. Execute **`Customers -> Create Customer`**: salva `customerId` e `document`.
3. Execute **`Vehicles -> Create Vehicle`**: salva `vehicleId` e `plate`.
4. Execute **`Services -> Create Service`** e **`Products -> Create Product`**: salvam `serviceId` e `productId`.
5. Execute **`Service Orders -> Create Service Order`**: salva `serviceOrderId`, `trackingCode` e `trackingToken`.
6. Prossiga pelo fluxo de diagnóstico, orçamento, aprovação e execução.

---

## 🧪 Testes Automatizados, Cobertura e Segurança

### Executar Testes Unitários e de Integração
Os testes de integração conectam-se ao PostgreSQL real e se auto-ignoram caso o banco não esteja disponível, mantendo o comando `go test ./...` sempre verde:

```bash
# Roda toda a suíte de testes (com Docker rodando)
DATABASE_URL='postgres://workshop:workshop@localhost:5432/automotive_workshop?sslmode=disable' JWT_SECRET=dev-secret go test ./...
```
*Ou via Makefile:*
```bash
make test
```

### Validação de Cobertura de Código (RNF06 - Mínimo 80%)
O script [`scripts/coverage.sh`](scripts/coverage.sh) afere a cobertura por pacote e aplica o *gate* estrito de 80% nos domínios críticos (`service-order`, `product`, `service-order-tracking`):

```bash
DATABASE_URL='postgres://workshop:workshop@localhost:5432/automotive_workshop?sslmode=disable' JWT_SECRET=dev-secret ./scripts/coverage.sh
```

### Análise Estática e Varredura de Segurança (SAST / Vulnerabilidades)
Executa `govulncheck` e `gosec` fixados em versões sem poluir o `go.mod`:

```bash
./scripts/security-scan.sh
```

---

## 📁 Estrutura de Pastas do Projeto

```text
automotive-workshop-api/
├── cmd/
│   └── api/
│       └── main.go                 # Entrypoint HTTP e Composition Root da aplicação
├── internal/
│   ├── features/                   # Fatias verticais (Vertical Slice Architecture)
│   │   ├── auth/                   # Autenticação administrativa e login JWT
│   │   ├── customer/               # Gestão de clientes (PF/PJ)
│   │   ├── vehicle/                # Gestão de veículos
│   │   ├── service-catalog/        # Catálogo de serviços
│   │   ├── product/                # Catálogo de produtos e movimentação de estoque
│   │   ├── service-order/          # Ciclo de vida da OS, orçamentos, execução e métricas
│   │   └── service-order-tracking/ # Acompanhamento público do cliente
│   ├── shared/                     # Código utilitário transversal e infraestrutura
│   │   ├── apierror/               # Tratamento de erro padronizado (RFC 7807)
│   │   ├── config/                 # Carregamento de variáveis de ambiente
│   │   ├── database/               # Pool de conexões PostgreSQL (pgxpool)
│   │   ├── document/               # Validadores algorítmicos de CPF e CNPJ
│   │   ├── middleware/             # Middleware de autenticação JWT
│   │   ├── token/                  # Gerenciador de emissão e verificação de JWT
│   │   └── trackingtoken/          # Gerador e hash de token de rastreamento
│   └── handlers_test/              # Testes de integração HTTP ponta a ponta
├── bruno/                          # Coleção de requisições para o cliente Bruno
├── docs/                           # Documentação arquitetural, OpenAPI, Postman e Schemas SQL
│   ├── openapi.yaml                # Contrato OpenAPI 3.0 completo (47 rotas)
│   ├── schema.sql                  # Schema DDL completo do PostgreSQL
│   ├── seed.sql                    # Dados de carga inicial para desenvolvimento e testes
│   ├── postman-collection.json     # Coleção Postman v2.1
│   └── insomnia-collection.json    # Coleção Insomnia v4
├── scripts/                        # Scripts de automação de cobertura e segurança
│   ├── coverage.sh                 # Aferição e validação do gate de cobertura (>= 80%)
│   └── security-scan.sh            # Varredura com gosec e govulncheck
├── .env.example                    # Modelo de configuração de ambiente
├── docker-compose.yml              # Orquestração local (API + Postgres + Swagger + Adminer)
├── Dockerfile                      # Build multistage da API em Go
├── Makefile                        # Atalhos de comandos para desenvolvimento
└── go.mod                          # Módulo Go e dependências
```
