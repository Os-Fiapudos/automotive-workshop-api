# Automotive Workshop API

> **Tech Challenge — Fase 1 | Pós-Tech FIAP (Software Architecture)**  
> REST API robusta para gerenciamento completo do ciclo de atendimento em uma oficina mecânica: cadastro de clientes e veículos, catálogo de serviços, controle de peças/insumos com estoque, abertura de Ordens de Serviço (OS), diagnóstico, composição e aprovação de orçamentos, execução de serviços com auditoria, baixa de peças, acompanhamento público pelo cliente e métricas operacionais.

---

## Integrantes do Grupo

- **Jean Ferreira dos Santos Cruz Junior** — RM376169
- **João Victor dos Santos Cerqueira** — RM376742
- **Giovane Kenuy Soares Colognesi Rubira Cardona** — RM376443
- **Augusto José Rizzi** — RM376822
- **Lucas Guimarães Fabris** — RM376444

---

## Sumário

1. [Visão Geral do Projeto](#visão-geral-do-projeto)
2. [Ciclo de Vida da Ordem de Serviço](#ciclo-de-vida-da-ordem-de-serviço)
3. [Arquitetura e Padrões](#arquitetura-e-padrões)
4. [Tecnologias, Bibliotecas e Dependências](#tecnologias-bibliotecas-e-dependências)
5. [Como Rodar o Projeto com Docker (Passo a Passo)](#como-rodar-o-projeto-com-docker-passo-a-passo)
6. [Credenciais de Teste e Dados Iniciais (Seed)](#credenciais-de-teste-e-dados-iniciais-seed)
7. [Migrações do Banco de Dados](#migrações-do-banco-de-dados)
8. [Documentação dos Endpoints da API](#documentação-dos-endpoints-da-api)
9. [Coleções para Testes (Bruno, Postman e Insomnia)](#coleções-para-testes-bruno-postman-e-insomnia)
10. [Testes Automatizados, Cobertura e Segurança](#testes-automatizados-cobertura-e-segurança)
11. [Pipeline de CI/CD](#pipeline-de-cicd)
12. [Estrutura de Pastas do Projeto](#estrutura-de-pastas-do-projeto)

---

## Visão Geral do Projeto

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

## Ciclo de Vida da Ordem de Serviço

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

## Arquitetura e Padrões

O projeto foi construído seguindo os princípios de **Vertical Slice Architecture** (monólito modular orientado a funcionalidades):

- **Fatias Verticais Independentes (`internal/features/`)**: Cada funcionalidade de negócio (`auth`, `customer`, `vehicle`, `service-catalog`, `product`, `service-order`, `service-order-tracking`) encapsula suas próprias camadas de **Handler (HTTP)**, **Service (Regras de Domínio/Aplicação)**, **Repository (Acesso a Dados SQL)** e **Models/DTOs**.
- **Desacoplamento entre Features**: Uma feature nunca importa diretamente outra feature. Quando é necessária comunicação ou validação cruzada (ex: criação de veículo validando se o cliente existe e está ativo), é utilizada a técnica de **Dependency Inversion** com *Adapters* definidos no ponto de composição (`cmd/api/main.go`).
- **Núcleo Compartilhado (`internal/shared/`)**: Reservado estritamente para utilitários agnósticos de domínio: pool de conexões com o PostgreSQL, middlewares de autenticação, validação algorítmica de documentos, emissão/verificação de tokens e tratamento padronizado de erros.
- **Composition Root Limpo (`cmd/api/main.go`)**: Responsável unicamente por ler configurações, abrir conexões, instanciar as dependências, montar os adaptadores e registrar as rotas no multiplexador HTTP nativo.

---

## Tecnologias, Bibliotecas e Dependências

A aplicação foi projetada com foco em alta performance, baixo acoplamento e dependências externas mínimas e deliberadas:

| Tecnologia / Dependência | Versão | Finalidade e Justificativa |
| :--- | :---: | :--- |
| **Go** | `1.26` | Linguagem compilada com excelente performance, tipagem estática e segurança de memória. |
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

## Como Rodar o Projeto com Docker (Passo a Passo)

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
> - `EXTERNAL_NOTIFICATION_SECRET=change-me-dev-only-external-notification-secret`
> - `EXTERNAL_STATUS_UPDATE_SECRET=change-me-dev-only-external-status-update-secret` (obrigatório e diferente do anterior)
> - `N8N_PORT=5678`
> - `RESEND_API_KEY` e `RESEND_FROM_EMAIL` (opcionais, sempre em par): habilitam o e-mail de
>   "orçamento disponível" via Resend. Sem a chave, a notificação fica desabilitada (no-op); chave
>   sem remetente impede a API de iniciar.
>
> **Obrigatórias:** `JWT_SECRET`, `EXTERNAL_NOTIFICATION_SECRET` e `EXTERNAL_STATUS_UPDATE_SECRET`. Sem qualquer uma delas, o
> `docker compose` se recusa a subir e diz qual variável falta (ex.: `required variable
> JWT_SECRET is missing a value`). As portas (`*_PORT`) são as do seu computador; dentro dos
> containers elas não mudam.

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

O Docker iniciará os serviços nesta ordem:
1. **`db`** (PostgreSQL 16): o banco, vazio na primeira subida — ele **não** cria o schema sozinho.
2. **`migrate`** (golang-migrate, executa e termina): aplica as [migrations](#migrações-do-banco-de-dados) pendentes de `migrations/` assim que o banco está saudável. Se falhar, `docker compose up` termina com erro e a API **não** sobe.
3. **`seed`** (executa e termina): aplica os dados de exemplo de `docs/seed.sql` depois das migrations, a cada `up` (é idempotente). Uma falha no seed não impede a API de subir.
4. **`api`** (Go 1.26): inicializa a API na porta `8080` depois que o banco está saudável e as migrations terminam com sucesso; fica `healthy` quando `GET /health` responde.
5. **`swagger-ui`** (`v5.33.1`): Interface interativa com a documentação OpenAPI 3.0 ([docs/openapi.yaml](docs/openapi.yaml)) na porta `8082`.
6. **`adminer`** (`6.1.1`): Interface web para navegação e consultas no PostgreSQL na porta `8081`, depois que o banco está saudável.
7. **`n8n`** (`2.43.8`): ferramenta externa de atualização de status da OS na porta `5678`, iniciada depois da API; importa e publica o workflow de [tools/n8n/](tools/n8n/README.md) a cada subida.

> `migrate` e `seed` aparecem como `Exited (0)` em `docker compose ps -a` — é o esperado. Para ver o que aplicaram: `docker compose logs migrate seed`.

#### Sobre a imagem da API

O [Dockerfile](Dockerfile) é multi-stage ([specs/containerization/](specs/containerization/)):

- **Build** em `golang:1.26-alpine` (mesma versão do `go.mod` e da CI). As dependências são baixadas numa camada própria, reaproveitada enquanto `go.mod`/`go.sum` não mudam.
- **Execução** em `alpine:3.24`, só com os binários `/api` e `/bootstrap-admin` (sem Go nem código-fonte).
- Roda como **usuário não root** `app` (UID/GID `10001`), sem permissão de escrita nos binários.
- **`HEALTHCHECK`** chama `GET /health` a cada 10 s; `docker compose ps` mostra `(healthy)`.
- O [.dockerignore](.dockerignore) é uma lista de permitidos: só `go.mod`, `go.sum`, `cmd/` e `internal/` entram no contexto de build. `.env`, `backups/`, estado do Terraform e qualquer arquivo novo ficam de fora.
- As versões das imagens (`alpine`, `adminer`, `swagger-ui`, `migrate`, `n8n`) são fixadas de propósito; atualize-as revisando a spec, não trocando para `latest`.

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

Para um teste completo, rode o **smoke test** ([scripts/smoke.sh](scripts/smoke.sh)): health, login com o admin do seed, abertura de uma OS autenticada e consulta dela.

```bash
make smoke          # ou: scripts/smoke.sh  (precisa só de curl)
```

A última linha mostra o id da OS criada. Para provar que os dados sobrevivem a um restart (ficam no volume `db_data`):

```bash
docker compose restart          # ou: docker compose down && docker compose up -d  (sem -v)
SMOKE_EXPECT_ORDER_ID=<id da OS> make smoke
```

O script usa por padrão a porta `API_PORT` do `.env` e os IDs fixos do seed; todos podem ser trocados por variáveis `SMOKE_*` (veja o cabeçalho do script). Cada execução abre uma OS nova no banco local.

---

### URLs e Acessos Disponíveis

| Serviço | URL de Acesso | Descrição |
| :--- | :--- | :--- |
| **API REST** | `http://localhost:8080` | Ponto de entrada da API. |
| **Swagger UI** | `http://localhost:8082` | Documentação interativa completa (OpenAPI 3.0). As collections (Bruno, Postman, Insomnia) estão em [Coleções para Testes](#coleções-para-testes-bruno-postman-e-insomnia). |
| **n8n** | `http://localhost:5678/form/service-order-status` | Ferramenta de atualização de status da OS pelo operador ([roteiro](tools/n8n/README.md)). |
| **Adminer (DB Web)** | `http://localhost:8081` | Gerenciador do Postgres (Servidor: `db`, Usuário: `workshop`, Senha: `workshop`, Base: `automotive_workshop`). |
| **Postgres Database** | `localhost:5432` | Conexão direta com o banco de dados. |

---

### Comandos Úteis do Makefile e Docker

| Ação | Comando com Makefile | Comando equivalente com Docker Compose |
| :--- | :--- | :--- |
| **Subir tudo** | `make start` | `docker compose up -d` |
| **Ver logs da API e serviços** | `make logs` | `docker compose logs -f` |
| **Parar os containers** | `make stop` | `docker compose stop` |
| **Derrubar containers** | `make down` | `docker compose down` |
| **Resetar banco do zero (apaga dados + migrations + seed)** | `make reset` | `docker compose down -v && docker compose up -d` |
| **Reaplicar dados de seed** | `make seed` | `docker compose up seed` |
| **Aplicar migrations pendentes** | `make migrate-up` | `docker compose up migrate` |
| **Ver versão do schema** | `make migrate-version` | — |
| **Testar login via curl** | `make login` | *(via curl no endpoint `/api/v1/auth/login`)* |
| **Smoke test (health, login, abrir e consultar OS)** | `make smoke` | `scripts/smoke.sh` |
| **Abrir terminal psql no banco** | `make psql` | `docker compose exec db psql -U workshop -d automotive_workshop` |

---

## Credenciais de Teste e Dados Iniciais (Seed)

No ambiente local (Docker Compose), o serviço `seed` aplica automaticamente, depois das migrations, os dados de exemplo ([docs/seed.sql](docs/seed.sql)) contendo clientes, veículos, produtos, serviços, ordens de serviço e usuários administrativos:

| Usuário / E-mail | Senha | Perfil |
| :--- | :--- | :--- |
| `admin@workshop.local` | `admin123` | Administrador da Oficina |
| `soat-architecture@workshop.local` | `soat-architecture` | Administrador / Avaliação SOAT |

> Essas senhas são públicas e só valem para desenvolvimento local e testes. Ambientes implantados (Kubernetes/CI) **não** rodam o seed: o administrador é criado pelo [bootstrap de administrador](#bootstrap-do-usuário-administrador) com credenciais vindas de um Secret.

---

## Migrações do Banco de Dados

O schema é criado e evoluído **somente** por migrations versionadas em [`migrations/`](migrations/), aplicadas com o [golang-migrate](https://github.com/golang-migrate/migrate) `v4.20.1` a partir da imagem Docker oficial (nada a instalar além do Docker). A mesma cadeia de migrations roda no Compose, na CI e no Job do Kubernetes. Spec: [specs/database-migrations/](specs/database-migrations/).

- `0001_init` é a **baseline**: o schema exato da Fase 1. Ela se recusa a rodar num banco que já tem tabelas (veja [Adotar um banco existente](#adotar-um-banco-existente-fase-1)).
- [`docs/schema.sql`](docs/schema.sql) continua sendo a **referência legível** do schema completo, mas nenhum ambiente o executa mais. A CI confere que as migrations produzem exatamente esse schema.
- Todos os comandos abaixo também existem como [`scripts/migrate.sh`](scripts/migrate.sh) (`up`, `version`, `create`, `down`, `baseline`, `verify`), que funciona contra qualquer `DATABASE_URL`.

### Criar uma migration

```bash
make migrate-create name=add_vehicle_color   # cria migrations/NNNN_add_vehicle_color.up.sql e .down.sql
```

1. Escreva o `.up.sql` e o `.down.sql` (o down desfaz exatamente o up).
2. Atualize `docs/schema.sql` **no mesmo commit** para o schema resultante.
3. Rode `make migrate-verify` (o mesmo teste do job `migrations` da CI).

Regras: nunca edite uma migration que já foi aplicada em algum lugar (crie outra); não use `BEGIN`/`COMMIT` nem comandos que não rodam em transação (ex.: `CREATE INDEX CONCURRENTLY`). Cada migration roda inteira numa transação, e é isso que garante que uma falha não deixa nada pela metade.

### Aplicar

```bash
docker compose up -d      # aplica as pendentes antes de subir a API
make migrate-up           # ou só as migrations, com o banco já rodando
make migrate-version      # versão atual (ex.: "2")
```

Rodar de novo sem nada pendente responde `no change` e não altera nada.

### Adotar um banco existente (Fase 1)

Um volume criado na Fase 1 já tem as tabelas (criadas pelo antigo `docker-entrypoint-initdb.d`), mas não tem o controle de versão. Na primeira vez que você subir o Compose, o serviço `migrate` recusa e a API não sobe — **nada é alterado**. No final de `docker compose logs migrate` aparece `0001_init refused: this database already has tables...`. Para adotar o banco:

```bash
make start-db            # o banco precisa estar rodando
make migrate-baseline    # backup + comprovação de equivalência + registra a versão
docker compose up -d     # aplica as migrations posteriores (ex.: 0002) e sobe a API
```

O `baseline`:
1. faz um backup (`pg_dump -Fc`) em `backups/<banco>-<data>.dump` (fora do Git) e confere que ele pode ser lido;
2. compara o schema do banco (`pg_dump --schema-only`) com o schema de **cada** versão das migrations, montada num Postgres descartável;
3. só registra a versão se o schema bater **exatamente** com uma delas. Nenhum `CREATE` é executado de novo e nenhum dado é tocado.

Se não bater com nenhuma versão, ele mostra as diferenças e não registra nada. Um caso conhecido é um volume criado antes de 26/08/2026, com os status da OS ainda em português. Nesse caso, ajuste o banco com os `ALTER TYPE ... RENAME VALUE` documentados em `docs/schema.sql` (seção de enums) e rode o `baseline` de novo. Para um banco onde nada disso importa, recriar do zero também resolve: `make reset` (**apaga os dados**).

### Quando uma migration falha

1. A migration que falhou é desfeita por inteiro (ela roda numa transação), então o banco continua na versão anterior. Ele fica marcado como *dirty*, e todo `up` seguinte recusa com `Dirty database version N` — inclusive no deploy, que para antes do rollout.
2. Corrija a causa. Se a migration nunca rodou com sucesso em lugar nenhum, corrija o arquivo. Se já rodou em algum ambiente, deixe-a como está e crie uma migration corretiva.
3. `make migrate-baseline` (ou `scripts/migrate.sh baseline`) — faz backup, comprova que o schema é o da versão anterior e registra essa versão como limpa.
4. `make migrate-up`.

Nenhum passo apaga dados, e não existe comando para "forçar" uma versão sem comprovação.

### Reverter (só em banco descartável)

```bash
make migrate-down MIGRATE_CONFIRM_DOWN=yes   # reverte 1 migration
```

O `down` apaga as tabelas e os dados que a migration criou. Por isso ele exige a confirmação explícita e serve só para desenvolvimento ou teste em banco descartável. Num ambiente com dados que importam, a correção é: backup + nova migration corretiva.

### Verificar a cadeia inteira

```bash
make migrate-verify   # ~1 min; usa só containers descartáveis, nunca o seu banco
```

Confere: banco vazio migrado ≡ `docs/schema.sql`; segunda execução sem mudança; `down` de tudo e `up` de novo; adoção de um banco da Fase 1 com dados preservados; recusa de um banco que não bate com nenhuma versão; e falha controlada com recuperação. É o que roda no job `migrations` da CI.

### Bootstrap do usuário administrador

Um banco recém-criado fora do ambiente local (Kubernetes, CI) não recebe o seed, e a API só tem o endpoint de login, sem cadastro de usuário. O comando `/bootstrap-admin`, que vem na mesma imagem da API, cria o administrador a partir de variáveis de ambiente:

```bash
ADMIN_EMAIL=admin@oficina.example ADMIN_PASSWORD='uma-senha-forte-aqui' make bootstrap-admin
# no cluster: Job com a imagem da API, command ["/bootstrap-admin"], variáveis vindas de um Secret
```

- `ADMIN_EMAIL` e `ADMIN_PASSWORD` são obrigatórios (senha de 12 a 72 bytes). `ADMIN_NAME` é opcional (padrão `Administrator`).
- A senha é convertida em hash bcrypt dentro do comando: só o hash chega ao banco, e nem senha nem hash aparecem na saída.
- É idempotente: se já existe um usuário com esse e-mail, nada muda (a senha existente **não** é trocada). Pode rodar em todo deploy.

---

## Documentação dos Endpoints da API

A API possui **49 rotas registradas**, todas com 100% de paridade no OpenAPI 3.0 ([docs/openapi.yaml](docs/openapi.yaml)).

### Modelos de Autenticação:
- **`JWT (Bearer)`**: Requer cabeçalho `Authorization: Bearer <token>` obtido através do login administrativo.
- **`X-Tracking-Token`**: Rota pública do cliente; não aceita JWT administrativo e exige o cabeçalho `X-Tracking-Token: <token>` emitido exclusivamente na abertura da OS.
- **`X-External-Notification-Secret`**: Canal de sistema externo para decisão de orçamento; não usa JWT e exige o segredo compartilhado configurado em `EXTERNAL_NOTIFICATION_SECRET`.
- **`X-External-Status-Secret`**: Canal da ferramenta externa (n8n) de atualização de status da OS; não usa JWT e exige o segredo `EXTERNAL_STATUS_UPDATE_SECRET`, separado do segredo de orçamento.
- **`Pública`**: Rotas abertas sem necessidade de credenciais.

---

### 1. Saúde e Autenticação
| Método | Endpoint | Proteção | Descrição |
| :--- | :--- | :---: | :--- |
| `GET` | `/health` | Pública | Verificação de liveness e status da API. |
| `POST` | `/api/v1/auth/login` | Pública | Autentica usuário administrativo e retorna o token JWT. |
| `GET` | `/api/v1/auth/me` | JWT | Retorna os dados do usuário autenticado. |

### 2. Gestão de Clientes (`/customers`)
| Método | Endpoint | Proteção | Descrição |
| :--- | :--- | :---: | :--- |
| `POST` | `/api/v1/customers` | JWT | Cadastra um novo cliente (CPF ou CNPJ validado). |
| `GET` | `/api/v1/customers` | JWT | Lista clientes de forma paginada (`page`, `pageSize`). |
| `GET` | `/api/v1/customers/{id}` | JWT | Busca cliente por UUID. |
| `GET` | `/api/v1/customers/document/{document}` | JWT | Busca cliente por CPF ou CNPJ normalizado. |
| `PATCH` | `/api/v1/customers/{id}` | JWT | Atualiza dados cadastrais de um cliente. |
| `DELETE` | `/api/v1/customers/{id}` | JWT | Inativação lógica do cliente (`status = INACTIVE`). |

### 3. Gestão de Veículos (`/vehicles`)
| Método | Endpoint | Proteção | Descrição |
| :--- | :--- | :---: | :--- |
| `POST` | `/api/v1/vehicles` | JWT | Cadastra veículo vinculado a um cliente ativo (placa antiga ou Mercosul). |
| `GET` | `/api/v1/vehicles` | JWT | Lista veículos de forma paginada. |
| `GET` | `/api/v1/vehicles/{id}` | JWT | Busca veículo por UUID. |
| `GET` | `/api/v1/vehicles/plate/{plate}` | JWT | Busca veículo por placa normalizada. |
| `GET` | `/api/v1/vehicles/customer/{customerId}` | JWT | Lista veículos vinculados a um cliente específico. |
| `PATCH` | `/api/v1/vehicles/{id}` | JWT | Atualiza marca, modelo, ano ou cor do veículo. |
| `DELETE` | `/api/v1/vehicles/{id}` | JWT | Inativação lógica do veículo (`status = INACTIVE`). |

### 4. Catálogo de Serviços (`/services`)
| Método | Endpoint | Proteção | Descrição |
| :--- | :--- | :---: | :--- |
| `POST` | `/api/v1/services` | JWT | Cria um serviço no catálogo (preço obrigatório, tempo estimado opcional). |
| `GET` | `/api/v1/services` | JWT | Lista serviços ativos (suporta filtro `?active=true/false`). |
| `GET` | `/api/v1/services/{id}` | JWT | Busca serviço por UUID. |
| `PATCH` | `/api/v1/services/{id}` | JWT | Atualiza preço, descrição, nome ou tempo estimado. |
| `DELETE` | `/api/v1/services/{id}` | JWT | Inativação lógica do serviço (`active = false`). |

### 5. Catálogo de Produtos e Estoque (`/products`)
| Método | Endpoint | Proteção | Descrição |
| :--- | :--- | :---: | :--- |
| `POST` | `/api/v1/products` | JWT | Cadastra peça (`PART`) ou insumo (`SUPPLY`). |
| `GET` | `/api/v1/products` | JWT | Lista produtos com filtros (`page`, `pageSize`, `type`, `status`). |
| `GET` | `/api/v1/products/{id}` | JWT | Busca produto por UUID. |
| `PATCH` | `/api/v1/products/{id}` | JWT | Atualiza dados cadastrais e preço do produto. |
| `DELETE` | `/api/v1/products/{id}` | JWT | Inativação lógica do produto (`status = INACTIVE`). |
| `POST` | `/api/v1/products/{id}/stock/adjustments` | JWT | Realiza ajuste manual de estoque (`ENTRY`, `EXIT`, `LOSS`) com auditoria. |
| `GET` | `/api/v1/products/{id}/stock` | JWT | Consulta saldo atual de estoque do produto. |
| `GET` | `/api/v1/products/{id}/movements` | JWT | Lista o histórico de movimentações de estoque do produto. |

### 6. Ordens de Serviço — Ciclo de Atendimento (`/service-orders`)
| Método | Endpoint | Proteção | Descrição |
| :--- | :--- | :---: | :--- |
| `POST` | `/api/v1/service-orders` | JWT | Abre uma nova OS com status `RECEIVED` e emite o `trackingToken`. |
| `GET` | `/api/v1/service-orders` | JWT | Lista OSs com filtros avançados (`code`, `status`, `document`, `licensePlate`, período). |
| `GET` | `/api/v1/service-orders/{id}` | JWT | Detalha a OS completa (aceita UUID ou código sequencial numérico). |
| `POST` | `/api/v1/service-orders/{id}/diagnosis` | JWT | Inicia o diagnóstico da OS (`RECEIVED → IN_DIAGNOSIS`). |
| `PUT` | `/api/v1/service-orders/{id}/quote` | JWT | Compõe/versiona o orçamento com itens de produtos e serviços. |
| `GET` | `/api/v1/service-orders/{id}/quote` | JWT | Consulta o orçamento vinculado à OS. |
| `POST` | `/api/v1/service-orders/{id}/quote/send` | JWT | Registra o envio do orçamento ao cliente (`IN_DIAGNOSIS → AWAITING_APPROVAL`). |
| `POST` | `/api/v1/service-orders/{id}/quote/decision` | Segredo externo | Sistema externo notifica a decisão do cliente (`{"decision":"approved"\|"rejected"}` → `IN_PROGRESS`/`CANCELED`); `409` se já decidido. |
| `POST` | `/api/v1/service-orders/{id}/status` | Segredo de status | Ferramenta externa (n8n) avança a OS para `IN_DIAGNOSIS`/`COMPLETED`/`DELIVERED` com `notes` opcional gravada no histórico; `409` se a transição for ilegal, `400` fora do contrato. |
| `POST` | `/api/v1/service-orders/{id}/executions` | JWT | Registra o início de execução de um serviço específico da OS. |
| `POST` | `/api/v1/service-orders/{id}/executions/{executionId}/finish` | JWT | Registra a conclusão da execução do serviço. |
| `POST` | `/api/v1/service-orders/{id}/finalize` | JWT | Finaliza a OS (`IN_PROGRESS → COMPLETED`) após concluir as execuções. |
| `POST` | `/api/v1/service-orders/{id}/deliver` | JWT | Registra a entrega do veículo ao cliente (`COMPLETED → DELIVERED`). |
| `POST` | `/api/v1/service-orders/{id}/stock-movements` | JWT | Dá baixa no estoque de peças/insumos consumidos na OS. |
| `GET` | `/api/v1/service-orders/{id}/stock-movements` | JWT | Lista as peças e movimentações de estoque vinculadas à OS. |
| `POST` | `/api/v1/service-orders/{id}/stock-movements/{movementId}/reversal` | JWT | Estorna uma movimentação de estoque da OS, restaurando o saldo. |
| `GET` | `/api/v1/service-orders/metrics/average-execution-time` | JWT | Retorna a métrica de tempo médio de execução agrupado por serviço. |

### 7. Acompanhamento Público e Decisão pelo Cliente (`/acompanhamento`)
| Método | Endpoint | Proteção | Descrição |
| :--- | :--- | :---: | :--- |
| `GET` | `/api/v1/acompanhamento/{codigo}` | Token | Consulta pública reduzida da OS (não expõe PII nem dados internos). |
| `POST` | `/api/v1/acompanhamento/{codigo}/orcamento/aprovar` | Token | Cliente aprova o orçamento (`AWAITING_APPROVAL → IN_PROGRESS`). |
| `POST` | `/api/v1/acompanhamento/{codigo}/orcamento/reprovar` | Token | Cliente reprova o orçamento (`AWAITING_APPROVAL → CANCELED`). |

---

## Coleções para Testes (Bruno, Postman e Insomnia)

O projeto disponibiliza coleções completas e pré-configuradas para todas as 48 rotas, com encadeamento automático de variáveis:

1. **Bruno (Recomendado)**: Localizado na pasta [`bruno/`](bruno). Abra o diretório diretamente no aplicativo Bruno. Selecione o ambiente `Local`.
2. **Postman**: Importe o arquivo [`docs/postman-collection.json`](docs/postman-collection.json).
3. **Insomnia**: Importe o arquivo [`docs/insomnia-collection.json`](docs/insomnia-collection.json).

### Fluxo de Execução Encadeado (sem copiar e colar IDs):
1. Execute **`Auth -> Login`**: o script armazena o JWT na variável `token`.
2. Execute **`Customers -> Create Customer`**: salva `customerId` e `document`.
3. Execute **`Vehicles -> Create Vehicle`**: salva `vehicleId` e `plate`.
4. Execute **`Services -> Create Service`** e **`Products -> Create Product`**: salvam `serviceId` e `productId`.
5. Execute **`Service Orders -> Create Service Order`**: salva `serviceOrderId`, `trackingCode` e `trackingToken`.
6. Prossiga pelo fluxo de diagnóstico, orçamento, aprovação e execução.

---

## Testes Automatizados, Cobertura e Segurança

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

## Pipeline de CI/CD

Um único workflow ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)) valida cada commit e entrega a versão validada num cluster Kubernetes. Spec: [specs/ci-cd-pipeline/](specs/ci-cd-pipeline/).

```text
push / pull request ─┬─ build ───────┐
                     ├─ coverage ────┤   (os quatro checks de sempre)
                     ├─ migrations ──┤
                     ├─ security ────┤
                     └─ image ───────┤   build da imagem, sem push
                                     ▼
            push na main ou execução manual (workflow_dispatch)
                                     ▼
 publish ── push no GHCR: ghcr.io/os-fiapudos/automotive-workshop-api:<sha>
                                     ▼
 deploy (ambiente efêmero dentro do runner)
   check → Terraform (cluster kind + Postgres) → Postgres pronto → Secrets/ConfigMap
   → Job de migração até Complete → Job de bootstrap do admin → Deployment/Service/HPA
   → rollout → smoke autenticado → evidências → terraform destroy (sempre)
```

**Regra de falha:** nada é publicado se `build`, `coverage`, `migrations`, `security` e `image` não passarem para aquele commit; nada é implantado se não foi publicado; nada é *rollout* se o banco ou a migração falharem. Pull requests rodam só os checks e o build da imagem — sem push, sem deploy, sem segredo.

**Imagem:** nome em minúsculas (registries rejeitam maiúsculas), tag com o SHA completo do commit; o deploy usa a referência imutável `nome:<sha>@sha256:<digest>`. Se o pacote for privado, o job cria no cluster um Secret de pull com o próprio `GITHUB_TOKEN` (somente leitura).

**Credenciais:** o ambiente efêmero não usa nenhum secret do repositório. Senha do Postgres, `JWT_SECRET`, os dois `EXTERNAL_*` e a senha do administrador são gerados a cada execução, mascarados no log e morrem com o cluster. O Terraform roda com saída `-json` filtrada (só endereços e progresso), e as evidências são sanitizadas e reconferidas antes do upload.

**Smoke test** ([`scripts/deploy-smoke.sh`](scripts/deploy-smoke.sh)): `/health` não prova schema nem autorização, então o teste também confere `401` sem token, login com o administrador criado pelo `/bootstrap-admin` no banco recém-criado, `GET /api/v1/auth/me` e `GET /api/v1/services` autenticados.

**Evidências:** cada deploy (com sucesso ou não) publica o artefato `deploy-evidence-<id>` e o mesmo resumo na página da execução — id do ambiente (`ci-<run>-<tentativa>`), SHA, imagem e digest efetivamente rodando, resultado de cada etapa, status e logs dos Jobs, rollout e saída do smoke.

### Executar manualmente (vídeo / demonstração)

Em **Actions → CI → Run workflow** (ou `gh workflow run ci.yml --ref <branch>`). A opção **simulate_migration_failure** injeta, só naquela execução, uma migration que sempre falha: a execução termina com falha, o Job de migração fica `Failed`, nenhum `Deployment` é criado, e as evidências e o destroy acontecem do mesmo jeito.

```bash
gh workflow run ci.yml --ref main                                           # caminho feliz
gh workflow run ci.yml --ref main -f simulate_migration_failure=true        # falha controlada
```

O ambiente efêmero existe só durante o job: não há promessa de disponibilidade depois da execução.

### Ambiente de demonstração (máquina local)

O mesmo [`scripts/deploy.sh`](scripts/deploy.sh) que a CI chama etapa por etapa sobe, na sua máquina, um ambiente que **continua no ar** para o vídeo e o consumo da API. Requer Docker, Terraform, `kubectl`, `curl` e `jq`, e os diretórios `infra/` (FP-35) e `k8s/` (FP-34).

```bash
export IMAGE=ghcr.io/os-fiapudos/automotive-workshop-api:<sha>   # uma imagem já publicada pela CI
export POSTGRES_PASSWORD=$(openssl rand -hex 24) JWT_SECRET=$(openssl rand -hex 32) \
       EXTERNAL_NOTIFICATION_SECRET=$(openssl rand -hex 32) \
       EXTERNAL_STATUS_UPDATE_SECRET=$(openssl rand -hex 32) \
       ADMIN_EMAIL=admin@example.com ADMIN_PASSWORD=$(openssl rand -hex 16)
make deploy                                    # scripts/deploy.sh up — não destrói nada
make deploy-destroy DEPLOY_CONFIRM_DESTROY=yes # só quando quiser derrubar
```

Guarde esses valores: execuções seguintes no mesmo ambiente precisam deles (o Postgres mantém a senha com que foi criado). Se o pacote no GHCR for privado, exporte também `REGISTRY_USERNAME` e `REGISTRY_PASSWORD` (um token com `read:packages`). As etapas também rodam isoladas (`scripts/deploy.sh check|infra|config|migrate|bootstrap|rollout|smoke|evidence|destroy`), e as evidências ficam em `deploy-evidence/<id>/`.

---

## Estrutura de Pastas do Projeto

```text
automotive-workshop-api/
├── cmd/
│   ├── api/
│   │   └── main.go                 # Entrypoint HTTP e Composition Root da aplicação
│   └── bootstrap-admin/            # Comando que cria o admin num banco recém-migrado
├── migrations/                     # Migrations versionadas (golang-migrate): 0001 = baseline da Fase 1
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
│   ├── openapi.yaml                # Contrato OpenAPI 3.0 completo (48 rotas)
│   ├── schema.sql                  # Referência do schema completo (gerado pelas migrations; conferido na CI)
│   ├── seed.sql                    # Dados de exemplo para desenvolvimento local e testes
│   ├── postman-collection.json     # Coleção Postman v2.1
│   └── insomnia-collection.json    # Coleção Insomnia v4
├── scripts/                        # Scripts de automação
│   ├── coverage.sh                 # Aferição e validação do gate de cobertura (>= 80%)
│   ├── deploy.sh                   # Deploy em etapas no Kubernetes (CI e ambiente de demonstração)
│   ├── deploy-smoke.sh             # Smoke autenticado de um ambiente implantado
│   ├── migrate.sh                  # Migrations: up, version, create, down, baseline, verify
│   ├── security-scan.sh            # Varredura com gosec e govulncheck
│   └── smoke.sh                    # Smoke test da stack local (health, login, abrir/consultar OS)
├── .env.example                    # Modelo de configuração de ambiente
├── docker-compose.yml              # Orquestração local (Postgres + migrate + seed + API + Swagger + Adminer)
├── .dockerignore                   # Contexto de build: só go.mod, go.sum, cmd/ e internal/
├── Dockerfile                      # Build multi-stage da API (não root, healthcheck)
├── Makefile                        # Atalhos de comandos para desenvolvimento
└── go.mod                          # Módulo Go e dependências
```
