# automotive-workshop-api

API Go gerada automaticamente.

```bash
go run ./cmd/api
```

## Stack

**Go REST API** — API Go com layout padrao cmd/ + internal/ (handlers/models/services).

## Arquitetura

**Vertical Slice (Feature-based)** — Organizado por funcionalidade; cada feature reune suas proprias camadas.

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
  n13("go.mod")
  n0 --> n13
  n14(".gitignore")
  n0 --> n14
  n15("README.md")
  n0 --> n15
  n16("Dockerfile")
  n0 --> n16
  n17[".github/"]
  n0 --> n17
  n18["workflows/"]
  n17 --> n18
  n19("ci.yml")
  n18 --> n19
  n20("LICENSE")
  n0 --> n20
```
