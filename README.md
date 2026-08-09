# automotive-workshop-api

```bash
go run ./cmd/api
```

## Stack

**Go REST API** — API Go com layout padrao cmd/ + internal/ (handlers/models/services).

## Arquitetura

**Vertical Slice (Feature-based)** — Organizado por funcionalidade; cada feature reune suas proprias camadas.

## Banco de dados

O modelo de dados está documentado em [docs/entidades.md](docs/entidades.md) e o schema PostgreSQL correspondente (tabelas, enums, índices e comentários) em [docs/schema.sql](docs/schema.sql).

Suba o banco (Postgres + Adminer + API) com Docker Compose:

```bash
cp .env.example .env
docker compose up -d
```

- **Postgres**: `localhost:5432` (credenciais em `.env`), com `docs/schema.sql` aplicado automaticamente na primeira subida.
- **Adminer**: http://localhost:8081 — sistema `PostgreSQL`, servidor `db`, usuário/senha/banco conforme `.env`.
- **API**: http://localhost:8080/health

Para recriar o banco do zero (ex: após alterar `schema.sql`), como o script só roda na criação inicial do volume:

```bash
docker compose down -v
docker compose up -d
```

### Dados de exemplo (seed)

[docs/seed.sql](docs/seed.sql) popula o banco com clientes, veículos, produtos, serviços e ordens de serviço cobrindo os principais status do fluxo. Usa IDs fixos com `ON CONFLICT DO NOTHING`, então pode ser reexecutado sem duplicar dados.

PowerShell (Windows):

```powershell
Get-Content docs/seed.sql -Raw | docker compose exec -T db psql -U workshop -d automotive_workshop
```

Bash/Git Bash/macOS/Linux:

```bash
docker compose exec -T db psql -U workshop -d automotive_workshop < docs/seed.sql
```

## Estrutura do projeto

```mermaid
flowchart TD
  n0["automotive-workshop-api/"]
  n1["cmd/"]
  n0 --> n1
  n2["api/"]
  n1 --> n2
  n3("main.go")
  n2 --> n3
  n4["internal/"]
  n0 --> n4
  n5["features/"]
  n4 --> n5
  n6["user/"]
  n5 --> n6
  n7("doc.go")
  n6 --> n7
  n8("doc.go")
  n5 --> n8
  n9["shared/"]
  n4 --> n9
  n10("doc.go")
  n9 --> n10
  n11["handlers_test/"]
  n4 --> n11
  n12(".gitkeep")
  n11 --> n12
  n21["docs/"]
  n0 --> n21
  n22("entidades.md")
  n21 --> n22
  n23("schema.sql")
  n21 --> n23
  n26("seed.sql")
  n21 --> n26
  n13("go.mod")
  n0 --> n13
  n14(".gitignore")
  n0 --> n14
  n24(".env.example")
  n0 --> n24
  n15("README.md")
  n0 --> n15
  n16("Dockerfile")
  n0 --> n16
  n25("docker-compose.yml")
  n0 --> n25
  n17[".github/"]
  n0 --> n17
  n18["workflows/"]
  n17 --> n18
  n19("ci.yml")
  n18 --> n19
  n20("LICENSE")
  n0 --> n20
```
