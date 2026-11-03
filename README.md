# Automotive Workshop API

> **Tech Challenge — Fases 1 e 2 | Pós-Tech FIAP (Software Architecture)**  
> REST API robusta para gerenciamento completo do ciclo de atendimento em uma oficina mecânica: cadastro de clientes e veículos, catálogo de serviços, controle de peças/insumos com estoque, abertura de Ordens de Serviço (OS), diagnóstico, composição e aprovação de orçamentos, execução de serviços com auditoria, baixa de peças, acompanhamento público pelo cliente e métricas operacionais.

---

## Integrantes do Grupo

- **Jean Ferreira dos Santos Cruz Junior** — RM376169
- **João Victor dos Santos Cerqueira** — RM376742
- **Giovane Kenuy Soares Colognesi Rubira Cardona** — RM377047
- **Augusto José Rizzi** — RM376119
- **Lucas Guimarães Fabris** — RM376444

---

## Sumário

1. [Visão Geral do Projeto](#visão-geral-do-projeto)
2. [Ciclo de Vida da Ordem de Serviço](#ciclo-de-vida-da-ordem-de-serviço)
3. [Fase 2 — Escalabilidade, Infraestrutura e Automação](#fase-2--escalabilidade-infraestrutura-e-automação)
   - [Situação dos requisitos](#situação-dos-requisitos) · [Arquitetura proposta](#arquitetura-proposta) · [Evidências](#evidências-da-execução) · [Vídeo](#vídeo-demonstrativo) · [Pendências](#pendências)
4. [Arquitetura e Padrões](#arquitetura-e-padrões)
5. [Tecnologias, Bibliotecas e Dependências](#tecnologias-bibliotecas-e-dependências)
6. [Como Rodar o Projeto com Docker (Passo a Passo)](#como-rodar-o-projeto-com-docker-passo-a-passo)
7. [Credenciais de Teste e Dados Iniciais (Seed)](#credenciais-de-teste-e-dados-iniciais-seed)
8. [Migrações do Banco de Dados](#migrações-do-banco-de-dados)
9. [Infraestrutura com Terraform e Deploy em Kubernetes](#infraestrutura-com-terraform-e-deploy-em-kubernetes)
10. [Documentação dos Endpoints da API](#documentação-dos-endpoints-da-api)
11. [Coleções para Testes (Bruno, Postman e Insomnia)](#coleções-para-testes-bruno-postman-e-insomnia)
12. [Testes Automatizados, Cobertura e Segurança](#testes-automatizados-cobertura-e-segurança)
13. [Pipeline de CI/CD](#pipeline-de-cicd)
14. [Estrutura de Pastas do Projeto](#estrutura-de-pastas-do-projeto)

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

## Fase 2 — Escalabilidade, Infraestrutura e Automação

### Objetivo e escopo desta fase

Com o aumento da demanda e a expansão para novas unidades, a Fase 2 evolui a aplicação da Fase 1 para ter qualidade, resiliência e escalabilidade:

- **Infraestrutura escalável**: a API roda em Kubernetes com réplicas e um `HorizontalPodAutoscaler` que escala por CPU e memória (2 a 8 pods).
- **Provisionamento e deploy automatizados**: o Terraform cria o cluster, o metrics-server e o PostgreSQL; um pipeline no GitHub Actions valida cada commit, publica a imagem e implanta banco, schema e aplicação no cluster.
- **Qualidade do código**: o `service-order`, núcleo do domínio, foi refatorado para Clean Architecture com a regra de dependência verificada por teste; os demais módulos seguem como fatias verticais independentes, preparando a migração para microsserviços.
- **Código e APIs**: abertura de OS com cliente, veículo, serviços e peças; consulta de status; decisão externa do orçamento; listagem ordenada por prioridade (sem OS finalizadas ou entregues); atualização de status por uma ferramenta externa (n8n); valores monetários exatos.
- **Banco versionado**: o schema evolui só por migrations (`golang-migrate`), aplicadas por um Job antes de cada rollout.

### Situação dos requisitos

| Requisito do Tech Challenge | Situação | Onde |
| --- | --- | --- |
| APIs: abertura de OS, consulta de status, aprovação externa, listagem ordenada, atualização de status por ferramenta | ✅ | [Endpoints](#6-ordens-de-serviço--ciclo-de-atendimento-service-orders), [`tools/n8n/`](tools/n8n/) |
| Testes automatizados nos fluxos críticos | ✅ cobertura ≥ 80 % exigida na CI | [Testes](#testes-automatizados-cobertura-e-segurança) |
| Refatoração com Clean Code e Clean Architecture | ✅ Clean Architecture completa no `service-order` (`domain/` ← `usecase/` ← `adapter/`, regra de dependência verificada por teste); demais módulos seguem como fatias verticais **por decisão**, para facilitar a migração para microsserviços | [Arquitetura e Padrões](#arquitetura-e-padrões) |
| Dockerfile e docker-compose revisados | ✅ | [Docker](#como-rodar-o-projeto-com-docker-passo-a-passo) |
| Manifestos Kubernetes: Deployment, Service, ConfigMap, Secret, HPA | ✅ validado em cluster real (veja [Evidências](#evidências-da-execução)) | [`k8s/`](k8s/README.md) |
| Terraform: cluster Kubernetes e banco de dados, documentados | ✅ validado em cluster real | [`infra/`](infra/README.md) |
| CI/CD: build, testes, imagem, deploy do banco, deploy no cluster, manifestos | ✅ executado no GitHub Actions: [execução na `main`](https://github.com/Os-Fiapudos/automotive-workshop-api/actions/runs/41368399908) com `publish` e `deploy` aprovados | [Pipeline de CI/CD](#pipeline-de-cicd) |
| README com arquitetura, instruções e collection | ✅ | esta seção |
| Vídeo demonstrativo (até 15 min) | ✅ [no YouTube](https://youtu.be/Cw9C57M0s0I) (não listado, 15 min) | [Vídeo](#vídeo-demonstrativo) |

### Arquitetura proposta

#### Componentes da aplicação

Monólito modular em Go, organizado em fatias verticais: cada funcionalidade tem rotas, regras e persistência próprias, e as fatias não se importam entre si. Quando uma precisa da outra, o *composition root* (`cmd/api`) injeta um adaptador. O `service-order`, núcleo do domínio e dono de todas as APIs novas da Fase 2, é organizado por dentro em camadas de Clean Architecture; os demais módulos mantêm handler, service e repository no mesmo pacote. O motivo dessa escolha está em [Arquitetura e Padrões](#arquitetura-e-padrões).

```mermaid
flowchart LR
    client["Clientes HTTP<br/>(collection, Swagger, portal do cliente)"]
    n8n["n8n<br/>(atualização de status)"]
    ext["Sistema externo<br/>(decisão do orçamento)"]
    resend["Resend<br/>(e-mail do orçamento)"]

    subgraph api["API Go — cmd/api (composition root: main.go + wiring.go, net/http)"]
        direction TB
        mw["middleware.RequireAuth<br/>(JWT HS256)"]
        subgraph slices["internal/features — fatias verticais (handler → service → repository)"]
            auth["auth"]
            customer["customer"]
            vehicle["vehicle"]
            catalog["service-catalog"]
            product["product<br/>(estoque)"]
            tracking["service-order-tracking<br/>(token do cliente)"]
        end
        subgraph order["internal/features/service-order — Clean Architecture"]
            direction TB
            httpapi["adapter/httpapi<br/>(rotas, DTOs, erro → status HTTP)"]
            usecase["usecase<br/>(abertura, orçamento, execução, listagem,<br/>status, métricas · portas em ports.go)"]
            domain["domain<br/>(agregado ServiceOrder, transições, erros)"]
            pgadapter["adapter/postgres<br/>(implementa as portas)"]
            notifier["adapter/notifier<br/>(no-op · Resend)"]
            httpapi --> usecase --> domain
            pgadapter -. "implementa" .-> usecase
            notifier -. "implementa" .-> usecase
        end
        shared["internal/shared<br/>database · token · apierror/httpx<br/>document · money · config"]
    end

    db[("PostgreSQL 16")]

    client --> mw --> slices
    mw --> httpapi
    client --> tracking
    n8n -- "X-External-Status-Secret" --> httpapi
    ext -- "X-External-Notification-Secret" --> httpapi
    notifier -. "orçamento enviado" .-> resend
    vehicle -. "adaptador no composition root" .-> customer
    slices --> shared
    pgadapter --> db
    shared --> db
```

As setas sólidas dentro do `service-order` apontam só para dentro: `domain` não conhece HTTP nem SQL, e `usecase` declara as portas que os adaptadores implementam. O teste [`architecture_test.go`](internal/features/service-order/architecture_test.go) quebra o build se essa regra for violada.

#### Infraestrutura provisionada

Tudo roda localmente num cluster `kind` (o PDF aceita cluster local ou em nuvem). O Terraform é o único dono do cluster, do namespace, do metrics-server e do banco; o deploy é dono dos objetos da API.

```mermaid
flowchart TB
    dev["Desenvolvedor / avaliador<br/>kubectl port-forward :8080"]

    subgraph kind["cluster kind"]
        subgraph ks["kube-system"]
            ms["metrics-server<br/>(Helm 3.13.1)"]
        end
        subgraph ns["automotive-workshop"]
            svc["Service automotive-workshop-api<br/>ClusterIP :8080"]
            deploy["Deployment automotive-workshop-api<br/>2–8 pods · probes /health<br/>100m/128Mi → 500m/256Mi"]
            hpa["HPA autoscaling/v2<br/>CPU 70 % · memória 80 %"]
            cm["ConfigMap da API<br/>+ ConfigMap de migrations"]
            sec["Secrets da API e do admin<br/>(criados no deploy)"]
            jobs["Jobs por execução:<br/>migração · bootstrap do admin"]
            pgsvc["Service postgres (headless)"]
            pg[("StatefulSet postgres-0<br/>PostgreSQL 16 + PVC 1 Gi")]
        end
    end

    dev --> svc --> deploy
    hpa -- "escala" --> deploy
    ms -- "métricas de CPU/memória" --> hpa
    cm --> deploy
    sec --> deploy
    jobs --> pgsvc
    deploy --> pgsvc --> pg

    classDef tf fill:#e8f0fe,stroke:#4a6fd1,color:#1a1a1a
    classDef app fill:#e9f7ef,stroke:#2e8b57,color:#1a1a1a
    class kind,ms,pgsvc,pg tf
    class svc,deploy,hpa,cm,sec,jobs app
```

O cluster `kind` (1 nó, Kubernetes 1.37) roda como container Docker na máquina local ou no runner da CI; o namespace `automotive-workshop` aplica o perfil de Pod Security `restricted`. Azul: criado pelo Terraform ([`infra/`](infra/README.md)). Verde: criado pelo deploy a partir de [`k8s/`](k8s/README.md).

#### Fluxo de deploy

```mermaid
flowchart TB
    push["push / pull request"] --> checks
    subgraph checks["Checks (todo push e PR)"]
        direction LR
        b["build · vet · test"]
        c["coverage ≥ 80 %<br/>(Postgres real)"]
        m["migrations verify"]
        s["security<br/>(govulncheck, gosec)"]
        i["image build<br/>(sem push)"]
    end
    checks -- "push na main<br/>ou execução manual" --> publish["publish<br/>GHCR :sha@digest"]
    publish --> d1
    subgraph deploy["deploy (scripts/deploy.sh)"]
        direction LR
        d1["terraform apply<br/>cluster + metrics-server + Postgres"] --> d2["Secrets + ConfigMap<br/>de migrations"]
        d2 --> d3{"Job de migração<br/>Complete?"}
        d3 -- "não" --> stop["para: sem rollout"]
        d3 -- "sim" --> d4["Job bootstrap-admin"] --> d5["Deployment + Service<br/>+ HPA · rollout"]
        d5 --> d6["smoke autenticado"] --> d7["evidências"] --> d8["terraform destroy<br/>(só no ambiente efêmero)"]
    end
```

Na CI, o ambiente é efêmero: o cluster nasce dentro do runner e é destruído no fim do job. Para o vídeo e para consumir a API, `make deploy` sobe o mesmo ambiente na máquina local e o deixa no ar ([Ambiente de demonstração](#ambiente-de-demonstração-máquina-local)).

### Como executar

| Objetivo | Onde |
| --- | --- |
| Execução local com Docker Compose | [Como Rodar o Projeto com Docker](#como-rodar-o-projeto-com-docker-passo-a-passo) |
| Provisionamento com Terraform (recursos criados, aplicar, destruir) | [Infraestrutura com Terraform e Deploy em Kubernetes](#infraestrutura-com-terraform-e-deploy-em-kubernetes) e [`infra/README.md`](infra/README.md) |
| Deploy em Kubernetes (ordem, Secrets, rollout, acesso) | [`k8s/README.md`](k8s/README.md) e [Ambiente de demonstração](#ambiente-de-demonstração-máquina-local) |
| Pipeline de CI/CD (gatilhos, execução manual, falha controlada) | [Pipeline de CI/CD](#pipeline-de-cicd) |
| Collection completa da API | [Postman](docs/postman-collection.json), [Insomnia](docs/insomnia-collection.json), [Bruno](bruno/), [OpenAPI/Swagger](docs/openapi.yaml) — veja [Coleções para Testes](#coleções-para-testes-bruno-postman-e-insomnia) |

### Evidências da execução

Ambiente real criado do zero em **2026-10-27 (UTC)**: Linux x86_64 com cgroup v2, Docker 29.9.2, 8 CPUs / 16 GiB, Terraform 1.16.6, `kubectl` 1.36.5, nó `kindest/node:v1.37.0`. A imagem da API foi construída do commit `4c731a0` e carregada no nó (veja [Sem imagem publicada](#sem-imagem-publicada-imagem-construída-localmente)). Os registros completos ficam em `evidence/` e `deploy-evidence/`, que não vão para o Git; nenhum valor secreto apareceu neles.

| Evidência | Comando | Resultado |
| --- | --- | --- |
| Validação estática | `make iac-validate`, `scripts/deploy.sh check` | Terraform válido; 9/9 manifests aprovados pelo `kubeconform -strict`; contrato `/infra` + `/k8s` ok |
| Provisionamento do zero | `scripts/deploy.sh infra` | 6 recursos criados em ~90 s num único state; Postgres pronto |
| `plan` estável | `terraform plan -detailed-exitcode` | `No changes` (código 0) |
| Deploy na ordem exigida | `scripts/deploy.sh config migrate bootstrap rollout` | Job de migração `Complete` (2 migrations) → Job de bootstrap `Complete` → rollout com 2 réplicas |
| Smoke autenticado | `scripts/deploy.sh smoke` | `/health` 200, `/auth/me` sem token 401, login 200, `/auth/me` 200, `/services` 200 |
| Métricas reais no HPA | `kubectl get hpa` | `cpu: 1%/70%, memory: 1%/80%`, mín. 2, máx. 8 |
| Autoscaling (carga leve) | `make k8s-hpa-load` (4 geradores × 240 s) | 2 → **3** réplicas com CPU a 90 %; voltou a 2 depois da carga |
| Autoscaling (carga forte) | `LOAD_WORKERS=10 LOAD_DURATION=180 make k8s-hpa-load` | 2 → 4 → **5** réplicas com CPU a 135 %; 5 → 4 → 2 depois da janela de estabilização de 120 s; `AbleToScale` e `ScalingActive` verdadeiros |
| Reposição de pod | `make k8s-pod-recovery` | novo pod pronto em **6 s**; o Service nunca ficou sem endpoint |
| Persistência do banco | `make k8s-db-persistence` | `postgres-0` recriado (UID novo, mesmo PVC); dado gravado antes foi lido depois; smoke passou de novo |
| APIs da Fase 2 no cluster | roteiro com `curl` via `port-forward` | 3 OS abertas; diagnóstico → orçamento → envio; decisão externa com segredo errado **401** e aprovada com o segredo certo (OS → `IN_PROGRESS`); status pela ferramenta externa `RECEIVED → IN_DIAGNOSIS`; listagem na ordem `IN_PROGRESS`, `IN_DIAGNOSIS`, `RECEIVED` |
| Falha de migração bloqueia o rollout | `SIMULATE_MIGRATION_FAILURE=true` num ambiente efêmero novo | Job de migração `Failed`, deploy interrompido com "the rollout is blocked", **nenhum** `Deployment`, HPA ou Service da API criado |
| Destroy de ambiente descartável | `make deploy-destroy DEPLOY_CONFIRM_DESTROY=yes` | 6 recursos destruídos; nenhum container `automotive-workshop` restante; state vazio |
| Testes da aplicação | `go build ./... && go vet ./... && go test ./...` (Go 1.26.10) | todos os pacotes ok |

**No GitHub Actions** (2026-11-02, push do merge do PR #44 na `main`, commit `f005f79`):

| Execução | Resultado |
| --- | --- |
| [CI #41368399908](https://github.com/Os-Fiapudos/automotive-workshop-api/actions/runs/41368399908) (push na `main`) | `build`, `coverage`, `migrations`, `security` e `image` aprovados → `publish` (imagem no GHCR) → `deploy`: Terraform → Secrets/ConfigMap → Job de migração → bootstrap do admin → rollout → smoke autenticado → evidências → destroy, tudo com sucesso. Pipeline completo em ~4 min (deploy ~2 min). Artefato `deploy-evidence-ci-41368399908-1` anexado à execução. |
| [CI #41360629362](https://github.com/Os-Fiapudos/automotive-workshop-api/actions/runs/41360629362) (pull request #44) | os cinco checks aprovados; `publish` e `deploy` **pulados**, como definido para pull requests |
| [IaC #41368399911](https://github.com/Os-Fiapudos/automotive-workshop-api/actions/runs/41368399911) (push na `main`) | validação estática de `infra/` e `k8s/` aprovada |

A falha controlada (`simulate_migration_failure`) foi demonstrada localmente (tabela acima). No GitHub ela ainda não foi disparada; veja [Pendências](#pendências).

### Vídeo demonstrativo

**[Automotive Workshop: Solução para o Tech Challenge da Postech FIAP | Fase 2](https://youtu.be/Cw9C57M0s0I)** (YouTube, não listado, 15 minutos).

O vídeo mostra o deploy da aplicação, a execução do CI/CD, o consumo das APIs e a escalabilidade automática, seguindo o roteiro de [specs/phase-2-deliverables/design.md](specs/phase-2-deliverables/design.md) §4.

### Pendências

> ⚠️ Itens que ainda não puderam ser concluídos:
>
> 1. **Acesso do `soat-architecture` ao repositório**: dar acesso de colaborador ao usuário do GitHub `soat-architecture` (Settings → Collaborators; não confundir com o usuário de mesmo nome do seed da aplicação) e confirmar que o convite foi aceito ou está pendente.
> 2. **Entrega no portal**: enviar o PDF de entrega (link do repositório, desenho da arquitetura, link do vídeo e integrantes) no portal do aluno dentro do prazo.
> 3. **Falha controlada no GitHub** (opcional, reforça a evidência): `gh workflow run ci.yml --ref main -f simulate_migration_failure=true`, e registrar o link da execução na tabela de evidências. O caminho feliz já rodou no push da `main`.

---

## Arquitetura e Padrões

O projeto combina duas abordagens, cada uma num nível diferente:

- **Entre módulos: Vertical Slice Architecture** (monólito modular orientado a funcionalidades). Cada funcionalidade de negócio (`auth`, `customer`, `vehicle`, `service-catalog`, `product`, `service-order`, `service-order-tracking`) fica numa pasta própria em `internal/features/`, com rotas, regras e acesso a dados.
- **Dentro do `service-order`: Clean Architecture.** É o núcleo do domínio e concentra todas as APIs novas da Fase 2 (abertura, consulta de status, decisão externa do orçamento, listagem priorizada e atualização de status pela ferramenta externa). Ele foi refatorado em camadas cujas dependências só apontam para dentro:

  | Camada | Conteúdo | Pode depender de |
  | --- | --- | --- |
  | `domain/` | agregado `ServiceOrder`, transições de status, orçamento, execuções, histórico e erros de negócio | biblioteca padrão, `uuid` e o tipo de valor `shared/money` |
  | `usecase/` | casos de uso (`Opening`, `Lifecycle`, `Quotes`, `Executions`, `Queries`, `StockUsage`, `Metrics`) e as portas que eles exigem (`ports.go`) | `domain/` |
  | `adapter/httpapi` | rotas, DTOs e o mapeamento único de erro → status HTTP | `usecase/`, `domain/` |
  | `adapter/postgres` | implementação das portas com SQL parametrizado (`pgx`) | `usecase/`, `domain/` |
  | `adapter/notifier` | e-mail de orçamento (no-op ou Resend) | `usecase/`, `domain/` |

  A composição acontece em [`cmd/api/wiring.go`](cmd/api/wiring.go). O teste [`architecture_test.go`](internal/features/service-order/architecture_test.go) resolve o grafo de imports e falha se `domain` ou `usecase` alcançarem `net/http`, `encoding/json`, `pgx`, um adaptador ou outra feature. Como roda no `go test ./...`, a CI barra qualquer regressão. A refatoração não mudou nenhum contrato HTTP: as 50 requisições da collection Postman devolveram os mesmos status e corpos antes e depois ([spec](specs/service-order-architecture-refactor/), [mapa de dependências antes/depois](docs/architecture-dependency-map.md)).

- **Clean Code** em todo o código: nomes de domínio em inglês, um `doc.go` por pacote explicando sua responsabilidade, portas pequenas e específicas (as 17 portas do `service-order` substituíram uma interface de 20 métodos) e um único ponto de tradução de erros para HTTP por feature.
- **Desacoplamento entre features**: nenhuma feature importa outra (verificado no código). Quando uma precisa da outra, como na criação de veículo, que valida se o cliente existe e está ativo, o *composition root* injeta um adaptador (inversão de dependência).
- **Núcleo compartilhado (`internal/shared/`)**: só utilitários sem regra de negócio, como pool do PostgreSQL, middleware JWT, emissão de tokens, validação de CPF/CNPJ, valores monetários exatos e envelope de erro.
- **Composition root enxuto (`cmd/api`)**: lê a configuração, abre conexões, monta adaptadores e casos de uso e registra as rotas no `http.ServeMux` nativo.

### Por que a Clean Architecture ficou só no `service-order`

Foi uma decisão do time, tomada em 12/10/2026 e registrada em [specs/service-order-architecture-refactor/requirements.md](specs/service-order-architecture-refactor/requirements.md) (§1 e §7.2) e na decisão 22 de [specs/architecture.md](specs/architecture.md).

**Mantivemos a Vertical Slice como organização principal porque ela facilita a migração da API para microsserviços nas próximas fases da Pós.** Cada fatia já funciona como uma fronteira de serviço:

- tem suas próprias rotas, regras e repositório, sem importar nenhuma outra feature;
- as dependências entre fatias passam por adaptadores no *composition root*, que são o ponto exato onde uma chamada em memória vira uma chamada HTTP ou uma mensagem quando a fatia for extraída;
- extrair um serviço passa a ser mover uma pasta para um novo repositório ou binário, com seu Dockerfile e seus manifestos, em vez de desmontar camadas técnicas compartilhadas (`handlers/`, `models/`, `services/` globais), que é o que acontece num monólito organizado por camada.

Por isso aplicamos as camadas completas onde o ganho é maior hoje, no `service-order`, que tem as regras mais complexas e todas as APIs novas desta fase. Os outros módulos ficaram como fatias planas, para não gastar esforço em camadas que serão redesenhadas na extração. Mesmo assim, eles receberam a única correção de Clean Architecture que o inventário encontrou: a interface do repositório de `customer`, `vehicle` e `product` saiu do `repository.go` e foi para o `service.go`, que é quem a consome. Cada módulo receberá as camadas completas, com a mesma receita do `service-order`, quando for extraído como microsserviço.

**Ponto de atenção para a extração:** as fatias não compartilham código, mas compartilham o banco. O `service-order` lê `customers`, `vehicles`, `products` e `services` e grava em `stock_movements` com SQL próprio. Separar os dados de cada serviço (uma API ou um evento no lugar da leitura direta da tabela) faz parte do trabalho das próximas fases.

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
| **Docker & Docker Compose** | — | Conteinerização completa da aplicação, banco de dados, cliente web do banco (Adminer), documentação interativa (Swagger UI) e ferramenta de status (n8n). |
| **golang-migrate** | `v4.20.1` | Migrations versionadas do schema, executadas pela imagem oficial no Compose, na CI e no Job do Kubernetes (não entra no `go.mod`). |
| **Kubernetes (kind)** | nó `v1.37.0` | Cluster local onde a API roda com Deployment, Service, ConfigMap, Secrets e HPA. |
| **Terraform** | `1.16.6` | Provisiona o cluster kind (`tehcyx/kind` 0.13.0), o metrics-server (chart 3.13.1) e o PostgreSQL em StatefulSet. |
| **GitHub Actions** | — | CI/CD: build, testes, cobertura, migrations, segurança, imagem no GHCR e deploy no cluster (`kubectl` `v1.36.5`). |
| **n8n** | `2.43.8` | Ferramenta externa de atualização de status da OS (formulário do operador). |
| **Resend** | API HTTP | E-mail de "orçamento disponível" ao cliente (opcional, chamada `net/http` sem SDK). |

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

## Infraestrutura com Terraform e Deploy em Kubernetes

Ambiente Kubernetes local da Fase 2, provisionado por código. Specs: [specs/infrastructure-provisioning/](specs/infrastructure-provisioning/) (FP-35) e [specs/kubernetes-deployment/](specs/kubernetes-deployment/) (FP-34).

```text
terraform apply (infra/)                      deploy (k8s/, FP-36 scripts/deploy.sh ou comandos manuais)
├─ cluster kind (1 nó)                         ├─ Secrets da API e do admin (nunca no Git)
├─ metrics-server (Helm)                       ├─ ConfigMap de migrations ← migrations/
├─ namespace automotive-workshop (restricted)  ├─ Job de migração ──► Complete (senão para aqui)
└─ Postgres 16: StatefulSet + PVC + Service    ├─ Job /bootstrap-admin ──► Complete
                                               └─ ConfigMap + Deployment + Service + HPA (2–8 pods, CPU 70 % / memória 80 %)
```

| Etapa | Comandos | Detalhes |
| --- | --- | --- |
| Provisionar cluster e banco | `export TF_VAR_postgres_password=$(openssl rand -hex 32)` e `make infra-init infra-apply` | [infra/README.md](infra/README.md): recursos criados, outputs, state, destroy, limites |
| Implantar a API | `make deploy` (FP-36) ou os comandos do [k8s/README.md](k8s/README.md) | ordem obrigatória; nunca `kubectl apply -f k8s/` |
| Acessar | `kubectl -n automotive-workshop port-forward svc/automotive-workshop-api 8080:8080` | collection com `baseUrl` `http://localhost:8080` |
| Demonstrar escala e resiliência | `make k8s-hpa-load`, `make k8s-pod-recovery`, `make k8s-db-persistence` | evidências em `evidence/` (git-ignored) |
| Validar sem cluster | `make iac-validate` | o mesmo que o workflow [`.github/workflows/iac.yml`](.github/workflows/iac.yml) roda |
| Derrubar | `make infra-destroy INFRA_CONFIRM_DESTROY=yes` | só em ambiente descartável: apaga os dados |

Pré-requisitos: Docker, Terraform ≥ 1.16 e `kubectl`. Em host com cgroup v1, veja a nota sobre `node_image` em [infra/README.md](infra/README.md#pré-requisitos).

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

O projeto disponibiliza coleções completas e pré-configuradas para todas as 49 rotas, com encadeamento automático de variáveis:

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

#### Sem imagem publicada: imagem construída localmente

Para testar um commit que ainda não foi publicado no GHCR (o `publish` só roda na `main` ou por execução manual), ou sem acesso ao pacote, construa a imagem do commit atual e carregue-a no nó do kind entre as etapas `infra` e `config`. O `Deployment` usa `imagePullPolicy: IfNotPresent`, então o kubelet usa a imagem carregada sem tentar baixá-la:

```bash
export IMAGE=ghcr.io/os-fiapudos/automotive-workshop-api:$(git rev-parse HEAD)
docker build -t "$IMAGE" .
scripts/deploy.sh check && scripts/deploy.sh infra
# equivalente a `kind load docker-image "$IMAGE" --name automotive-workshop`, sem precisar do CLI do kind
docker save "$IMAGE" | docker exec -i automotive-workshop-control-plane \
  ctr --namespace=k8s.io images import --all-platforms --digests --snapshotter=overlayfs -
make deploy        # reaplica o Terraform (sem mudanças) e segue com config → migrate → bootstrap → rollout → smoke
```

---

## Estrutura de Pastas do Projeto

```text
automotive-workshop-api/
├── .github/workflows/
│   ├── ci.yml                      # CI/CD: checks, imagem, publish no GHCR e deploy no Kubernetes
│   └── iac.yml                     # Validação estática de infra/ e k8s/
├── cmd/
│   ├── api/
│   │   ├── main.go                 # Entrypoint HTTP e Composition Root da aplicação
│   │   └── wiring.go               # Montagem dos casos de uso e adaptadores do service-order
│   └── bootstrap-admin/            # Comando que cria o admin num banco recém-migrado
├── migrations/                     # Migrations versionadas (golang-migrate): 0001 = baseline da Fase 1
├── infra/                          # Terraform: cluster kind, metrics-server, Postgres (FP-35)
├── k8s/                            # Manifests da API: Deployment, Service, ConfigMap, HPA, Jobs (FP-34)
├── internal/
│   ├── features/                   # Fatias verticais (Vertical Slice Architecture)
│   │   ├── auth/                   # Autenticação administrativa e login JWT
│   │   ├── customer/               # Gestão de clientes (PF/PJ)
│   │   ├── vehicle/                # Gestão de veículos
│   │   ├── service-catalog/        # Catálogo de serviços
│   │   ├── product/                # Catálogo de produtos e movimentação de estoque
│   │   ├── service-order/          # Ciclo de vida da OS, em camadas de Clean Architecture:
│   │   │   ├── domain/             #   agregado ServiceOrder, transições e erros de negócio
│   │   │   ├── usecase/            #   casos de uso e portas (ports.go)
│   │   │   ├── adapter/            #   httpapi (HTTP) · postgres (SQL) · notifier (e-mail)
│   │   │   └── architecture_test.go #  garante que as dependências só apontam para dentro
│   │   ├── service-order-tracking/ # Acompanhamento público do cliente
│   │   └── user/                   # Reservado (só doc.go); não é o login, que fica em auth/
│   ├── shared/                     # Código utilitário transversal e infraestrutura
│   │   ├── apierror/               # Envelope de erro JSON {"error": {code, message, details}}
│   │   ├── httpx/                  # Respostas JSON e envelope de erro usados pelo auth
│   │   ├── config/                 # Carregamento de variáveis de ambiente
│   │   ├── database/               # Pool de conexões PostgreSQL (pgxpool)
│   │   ├── document/               # Validadores algorítmicos de CPF e CNPJ
│   │   ├── middleware/             # Middleware de autenticação JWT
│   │   ├── money/                  # Valores monetários exatos em centavos (sem float64)
│   │   ├── token/                  # Gerenciador de emissão e verificação de JWT
│   │   └── trackingtoken/          # Gerador e hash de token de rastreamento
│   └── handlers_test/              # Testes de integração HTTP ponta a ponta
├── tools/n8n/                      # Workflow do n8n para atualização de status da OS
├── specs/                          # Especificações (SDD): requisitos, design e tarefas de cada feature
├── bruno/                          # Coleção de requisições para o cliente Bruno
├── docs/                           # Documentação arquitetural, OpenAPI, Postman e Schemas SQL
│   ├── openapi.yaml                # Contrato OpenAPI 3.0 completo (49 rotas)
│   ├── schema.sql                  # Referência do schema completo (gerado pelas migrations; conferido na CI)
│   ├── seed.sql                    # Dados de exemplo para desenvolvimento local e testes
│   ├── postman-collection.json     # Coleção Postman v2.1
│   └── insomnia-collection.json    # Coleção Insomnia v4
├── scripts/                        # Scripts de automação
│   ├── coverage.sh                 # Aferição e validação do gate de cobertura (>= 80%)
│   ├── deploy.sh                   # Deploy em etapas no Kubernetes (CI e ambiente de demonstração)
│   ├── deploy-smoke.sh             # Smoke autenticado de um ambiente implantado
│   ├── iac-validate.sh             # Validação estática de infra/ e k8s/ (terraform validate, kubeconform)
│   ├── k8s-evidence.sh             # Evidências no cluster: HPA sob carga, reposição de pod, persistência do banco
│   ├── migrate.sh                  # Migrations: up, version, create, down, baseline, verify
│   ├── security-scan.sh            # Varredura com gosec e govulncheck
│   └── smoke.sh                    # Smoke test da stack local (health, login, abrir/consultar OS)
├── .env.example                    # Modelo de configuração de ambiente
├── docker-compose.yml              # Orquestração local (Postgres + migrate + seed + API + Swagger + Adminer + n8n)
├── .dockerignore                   # Contexto de build: só go.mod, go.sum, cmd/ e internal/
├── Dockerfile                      # Build multi-stage da API (não root, healthcheck)
├── Makefile                        # Atalhos de comandos para desenvolvimento
└── go.mod                          # Módulo Go e dependências
```
