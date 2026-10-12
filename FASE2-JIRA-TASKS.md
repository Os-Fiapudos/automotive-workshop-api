# Fase 2 — Tasks para o Jira (PT-BR)

Cada bloco abaixo = uma task. "Spec" aponta pro arquivo em inglês (`specs/.../tasks.md`) com
o detalhe técnico completo — cole o título e a descrição no Jira, e deixe o link da spec na
própria task pra quem for implementar.

Sugestão de convenção de título no Jira (não há épico): prefixar cada task com
`[Fase 2][<feature>]`, ex.: `[Fase 2][Listagem OS] Query: ordenação por prioridade de
status`. Para filtrar/agrupar o board inteiro, use uma Label ou Component chamado
`fase-2-tech-challenge`.

---

## 1. Listagem de OS — ordenação por status e exclusão (spec: `service-order-query`)

1. **Query: excluir status finalizados da listagem padrão**
   Em `listServiceOrders`, quando não houver filtro de `status`, adicionar exclusão de
   `COMPLETED`, `DELIVERED` e `CANCELED` na cláusula WHERE.
   Spec: `specs/service-order-query/tasks.md` §10

2. **Query: ordenação por prioridade de status**
   Trocar o `ORDER BY` por duas ramificações: prioridade de status (`IN_PROGRESS` >
   `AWAITING_APPROVAL` > `IN_DIAGNOSIS` > `RECEIVED`) + mais antigas primeiro quando não há
   filtro de status; ordenação simples por data quando há filtro de status.
   Spec: `specs/service-order-query/tasks.md` §10

3. **Testes unitários: nova ordenação e exclusão da listagem**
   Cobrir: listagem sem filtro agrupada por status/mais antigas primeiro, sem
   `COMPLETED`/`DELIVERED`/`CANCELED`; filtro explícito por esses status ainda retorna os
   registros.
   Spec: `specs/service-order-query/tasks.md` §10

4. **Testes de integração: nova ordenação e exclusão (Postgres real)**
   Popular ordens em todos os status, validar a ordem exata retornada e a exclusão; mais um
   caso de `status=COMPLETED` explícito.
   Spec: `specs/service-order-query/tasks.md` §10

5. **Atualizar OpenAPI da listagem de OS**
   Documentar a nova ordenação padrão, a exclusão e o comportamento quando `status` é
   informado.
   Spec: `specs/service-order-query/tasks.md` §10

6. **Verificação final — listagem de OS**
   Rodar `go build`/`go vet`/`go test`; reconferir os critérios de aceite AC1, AC12, AC13.
   Spec: `specs/service-order-query/tasks.md` §10

---

## 2. Abertura de OS — aceitar peças (spec: `service-order-opening`)

1. **Schema: tabela de peças solicitadas na abertura**
   Criar `service_order_requested_products` em `docs/schema.sql`; documentar em
   `docs/entities.md`.
   Spec: `specs/service-order-opening/tasks.md` §11

2. **Domínio: tipo RequestedProduct e validação de quantidade**
   Adicionar `RequestedProduct` (`ProductID`, `Quantity`) ao agregado `ServiceOrder`;
   `NewServiceOrder` rejeita quantidade ≤ 0.
   Spec: `specs/service-order-opening/tasks.md` §11

3. **Serviço: validar peça existente e ativa**
   Reaproveitar `findActiveProductByID` (já existe, usado no orçamento) para validar cada
   peça solicitada; novo erro `ErrRequestedProductNotFound`.
   Spec: `specs/service-order-opening/tasks.md` §11

4. **Repositório: persistir peças solicitadas na mesma transação**
   Inserir em `service_order_requested_products` dentro da transação de criação da OS — sem
   tocar em `current_stock`.
   Spec: `specs/service-order-opening/tasks.md` §11

5. **API: campo requestedProducts no request/response + erro 404**
   `CreateRequest.RequestedProducts` com validação; resposta inclui `requestedProducts`;
   mapear peça inexistente/inativa pra `404 PRODUCT_NOT_FOUND`.
   Spec: `specs/service-order-opening/tasks.md` §11

6. **Testes unitários: peças na abertura de OS**
   Quantidade inválida rejeitada pelo construtor; casos de lookup (peça inexistente,
   inativa, sucesso) no fake de serviço.
   Spec: `specs/service-order-opening/tasks.md` §11

7. **Testes de integração: peças na abertura de OS**
   Caso de sucesso (confirmando `currentStock` inalterado), peça inexistente/inativa,
   quantidade inválida, e rollback por peça duplicada.
   Spec: `specs/service-order-opening/tasks.md` §11

8. **Atualizar OpenAPI da abertura de OS**
   Adicionar `requestedProducts` no schema de request e `PRODUCT_NOT_FOUND` nas respostas de
   erro de `POST /api/v1/service-orders` (estende o path existente, não cria um novo).
   Spec: `specs/service-order-opening/tasks.md` §11

9. **Verificação final — peças na abertura de OS**
   Rodar `go build`/`go vet`/`go test`; reconferir os novos critérios de aceite de
   `requirements.md` §6.
   Spec: `specs/service-order-opening/tasks.md` §11

---

## 3. Aprovação de orçamento — canal externo (spec: `service-order-quote-decision`)

1. **Refatorar decideQuote para receber a OS já resolvida**
   Separar a resolução da OS (por token ou por id) da lógica de decisão em si, pra reusar
   nos dois canais sem duplicar regra de negócio.
   Spec: `specs/service-order-quote-decision/tasks.md`

2. **Novo caso de uso: DecideQuoteExternal**
   Resolve a OS por id e chama a lógica de decisão compartilhada (task 1).
   Spec: `specs/service-order-quote-decision/tasks.md`

3. **Novo endpoint: POST /service-orders/{id}/quote/decision**
   Autenticação por segredo compartilhado (`X-External-Notification-Secret`, comparação em
   tempo constante); body `{"decision": "approved"|"rejected"}`.
   Spec: `specs/service-order-quote-decision/tasks.md`

4. **Validar registro da rota num ServeMux real antes de implementar o handler**
   Rodar `RegisterRoutes` contra um `*http.ServeMux` de verdade (sem banco) e confirmar que
   não há panic de conflito de padrão — o projeto já achou dois conflitos assim antes
   (`specs/architecture.md` decisão 18), então vale a checagem antes de escrever o handler.
   Spec: `specs/service-order-quote-decision/tasks.md`

5. **Mapear erro de segredo inválido para 401**
   `INVALID_EXTERNAL_SECRET` no switch de erros existente.
   Spec: `specs/service-order-quote-decision/tasks.md`

6. **Configuração do segredo externo**
   Ler `EXTERNAL_NOTIFICATION_SECRET` do ambiente em `main.go` (falha rápida se ausente);
   adicionar em `.env.example`.
   Spec: `specs/service-order-quote-decision/tasks.md`

7. **Testes unitários: canal externo de decisão**
   Sucesso (aprovar/rejeitar), conflito de decisão repetida, OS desconhecida.
   Spec: `specs/service-order-quote-decision/tasks.md`

8. **Testes de integração: canal externo de decisão**
   Segredo válido aprova/rejeita; segredo ausente/errado → 401 sem efeito colateral;
   `decision` inválido → 400; endpoints de cliente (token) continuam inalterados.
   Spec: `specs/service-order-quote-decision/tasks.md`

9. **Atualizar OpenAPI do novo endpoint**
   Documentar header, body e respostas do canal externo.
   Spec: `specs/service-order-quote-decision/tasks.md`

10. **Verificação final — canal externo de aprovação**
   Rodar `go build`/`go vet`/`go test`; reconferir AC15–AC20.
   Spec: `specs/service-order-quote-decision/tasks.md`

---

## 4. Refatoração para Clean Architecture do service-order (spec: `service-order-architecture-refactor`)

1. **Checagem de rastreabilidade da spec**
   Revisar requirements/design contra as seis specs existentes de `service-order` pra
   garantir que nenhum comportamento foi esquecido no mapeamento.
   Spec: `specs/service-order-architecture-refactor/tasks.md` §1

2. **Extrair camada de domínio**
   Criar `domain/`; mover `model.go`, `execution_model.go`, `stockusage_model.go`,
   `errors.go` e seus testes; atualizar imports no resto do pacote. Suíte verde ao final.
   Spec: `specs/service-order-architecture-refactor/tasks.md` §2

3. **Extrair camada de casos de uso (ports)**
   Criar `usecase/`; definir as interfaces menores (substituindo o `serviceOrderLookups` de
   20 métodos); mover os services; encolher os fakes de teste. Suíte verde.
   Spec: `specs/service-order-architecture-refactor/tasks.md` §3

4. **Extrair adapter Postgres**
   Criar `adapter/postgres/`; mover os repositórios; adicionar asserções de compilação
   confirmando que a implementação satisfaz cada port. Suíte verde.
   Spec: `specs/service-order-architecture-refactor/tasks.md` §4

5. **Extrair adapter HTTP**
   Criar `adapter/httpapi/`; dividir `handler.go` por caso de uso; mover DTOs e
   `httpsupport.go`; mover uso de `apierror` pra fora do usecase. Suíte verde.
   Spec: `specs/service-order-architecture-refactor/tasks.md` §5

6. **Atualizar wiring do main.go**
   Ajustar imports/construtores pros novos pacotes — `main.go` continua fino. Suíte verde.
   Spec: `specs/service-order-architecture-refactor/tasks.md` §6

7. **Checagem automática de camadas**
   Script/teste que falha se `domain/` ou `usecase/` importar `net/http`, `encoding/json`,
   `apierror` ou `pgx`.
   Spec: `specs/service-order-architecture-refactor/tasks.md` §7

8. **Atualizar CLAUDE.md e specs/architecture.md**
   Documentar a nova camada interna como padrão sancionado (só pra `service-order` por
   enquanto), sem contradizer a regra de vertical slice.
   Spec: `specs/service-order-architecture-refactor/tasks.md` §8

9. **Regressão completa da refatoração**
   Reconferir todos os critérios de aceite das seis specs de `service-order`;
   `scripts/coverage.sh` não pode cair; confirmar que nenhum arquivo fora de
   `internal/features/service-order/` mudou.
   Spec: `specs/service-order-architecture-refactor/tasks.md` §9

---

## 5. Atualização de status por ferramenta externa / e-mail (spec: `service-order-status-update`)

1. **Confirmar escopo de status com o time**
   Validar se os três status propostos (`IN_DIAGNOSIS`, `COMPLETED`, `DELIVERED`) são os
   corretos antes de implementar — item em aberto na própria spec.
   Spec: `specs/service-order-status-update/tasks.md` §1

2. **Caso de uso: UpdateStatusExternal (dispatch)**
   Função que direciona `status` pro caso de uso existente correspondente
   (`StartDiagnosis`/`FinalizeOrder`/`DeliverOrder`) — zero lógica nova de transição.
   Spec: `specs/service-order-status-update/tasks.md` §2

3. **Novo endpoint: POST /service-orders/{id}/status**
   Autenticação por segredo compartilhado (`X-External-Status-Secret`); valida `status`
   suportado; chama o dispatch.
   Spec: `specs/service-order-status-update/tasks.md` §3

4. **Validar registro da rota num ServeMux real antes de implementar o handler**
   Mesma checagem feita pro endpoint de aprovação externa — rodar `RegisterRoutes` contra um
   `*http.ServeMux` real e confirmar ausência de conflito de padrão.
   Spec: `specs/service-order-status-update/tasks.md` §3

5. **Configuração do segredo de status externo**
   `EXTERNAL_STATUS_UPDATE_SECRET` no ambiente, falha rápida se ausente, em `.env.example`.
   Spec: `specs/service-order-status-update/tasks.md` §3

6. **Testes unitários: dispatch de status externo**
   Um caso por status suportado + erro pra status não suportado.
   Spec: `specs/service-order-status-update/tasks.md` §4

7. **Testes de integração: endpoint de status externo**
   Sucesso por status, segredo ausente/errado → 401, status não suportado → 400, transição
   ilegal → mesmo erro do endpoint dedicado já existente.
   Spec: `specs/service-order-status-update/tasks.md` §4

8. **Atualizar OpenAPI do endpoint de status externo**
   Spec: `specs/service-order-status-update/tasks.md` §5

9. **Verificação final — status externo**
   `go build`/`go vet`/`go test`; reconferir AC1–AC8.
   Spec: `specs/service-order-status-update/tasks.md` §6

---

## 6. Precisão monetária — float64 → centavos exatos (spec: `monetary-value-precision`)

1. **Criar pacote internal/shared/money**
   Tipo `Cents int64`, `MarshalJSON`/`UnmarshalJSON` sem passar por `float64`, conversão
   para/de `pgtype.Numeric`.
   Spec: `specs/monetary-value-precision/tasks.md` §1

2. **Testes do pacote money**
   Round-trip JSON com valores conhecidos por perder precisão em float64; rejeição de mais
   de duas casas decimais; round-trip com NUMERIC.
   Spec: `specs/monetary-value-precision/tasks.md` §1

3. **Migrar service-catalog pra Cents**
   `Service.Price` → `money.Cents`; repositório convertendo via `pgtype.Numeric`.
   Spec: `specs/monetary-value-precision/tasks.md` §2

4. **Migrar product pra Cents**
   `Product.UnitPrice` → `money.Cents`; repositório convertendo via `pgtype.Numeric`.
   Spec: `specs/monetary-value-precision/tasks.md` §3

5. **Migrar service-order pra Cents**
   `QuoteItem.UnitPrice`/`.Total`, `Quote.TotalAmount` → `money.Cents`; `calculateTotal`
   reescrito só com aritmética inteira.
   Spec: `specs/monetary-value-precision/tasks.md` §4

6. **Testes de round-trip contra Postgres real (3 features)**
   Escrever um valor em `Cents`, ler de volta da coluna `NUMERIC(12,2)`, comparar
   exatamente — um teste por feature afetada.
   Spec: `specs/monetary-value-precision/tasks.md` §5

7. **Regressão completa dos testes de integração**
   Confirmar que todo response JSON com preço/total continua idêntico (prova de que o
   formato da API não mudou).
   Spec: `specs/monetary-value-precision/tasks.md` §5

8. **Atualizar CLAUDE.md §3 e specs/architecture.md com o novo pacote**
   Adicionar `internal/shared/money` na lista de pacotes compartilhados — ficou de fora da
   primeira passada por essa spec ter sido a que introduziu o pacote.
   Spec: `specs/monetary-value-precision/tasks.md` §6

9. **Verificação final — precisão monetária**
   `go build`/`go vet`/`go test`; diff do `docs/openapi.yaml` confirmando nenhuma mudança de
   schema; reconferir AC1–AC7.
   Spec: `specs/monetary-value-precision/tasks.md` §6

---

## 7. Containerização — Docker (spec: `containerization`)

1. **Dockerfile: cache de dependências**
   Copiar `go.mod`/`go.sum` e rodar `go mod download` antes do `COPY . .`.
   Spec: `specs/containerization/tasks.md` §1

2. **Dockerfile: usuário não-root e CGO_ENABLED=0**
   Criar usuário/grupo dedicado, `USER app` no estágio final.
   Spec: `specs/containerization/tasks.md` §1

3. **Dockerfile: HEALTHCHECK**
   Checar `GET /health` via `curl` no estágio final.
   Spec: `specs/containerization/tasks.md` §1

4. **Criar .dockerignore**
   Excluir `.git`, `*.md`, `docs/`, `specs/`, `bruno/`, `.env*`.
   Spec: `specs/containerization/tasks.md` §2

5. **Atualizar docker-compose e .env.example com os novos segredos**
   Adicionar `EXTERNAL_NOTIFICATION_SECRET`, `EXTERNAL_STATUS_UPDATE_SECRET`,
   `RESEND_API_KEY` com fail-fast (`${VAR:?...}`).
   Spec: `specs/containerization/tasks.md` §3

6. **Verificação final — Docker**
   Build duplo confirmando cache de camada; confirmar usuário não-root; confirmar
   healthcheck "healthy"; smoke test via `docker compose up`.
   Spec: `specs/containerization/tasks.md` §4

---

## 8. Ferramenta de migrations do banco (spec: `database-migrations`)

1. **Confirmar escolha da ferramenta (golang-migrate)**
   Validar com o time antes de seguir — item em aberto na spec.
   Spec: `specs/database-migrations/tasks.md` §1

2. **Criar migração baseline 0001_init**
   `up.sql` = conteúdo atual de `docs/schema.sql`; `down.sql` = drops em ordem reversa.
   Spec: `specs/database-migrations/tasks.md` §2

3. **Criar comandos locais de migração**
   `Makefile`/`scripts/migrate.sh` com `migrate-up`, `migrate-down`, `migrate-create`,
   usando a imagem oficial `migrate/migrate` fixada numa versão.
   Spec: `specs/database-migrations/tasks.md` §3

4. **Validar baseline contra o schema atual**
   `migrate-up` num Postgres vazio e comparar via `pg_dump --schema-only` com o schema
   atual aplicado por `docker compose up`.
   Spec: `specs/database-migrations/tasks.md` §4

5. **Validar down/up e idempotência**
   `migrate-down` + `migrate-up` resulta no mesmo schema; rodar `migrate-up` duas vezes não
   tem efeito na segunda.
   Spec: `specs/database-migrations/tasks.md` §4

6. **Atualizar CLAUDE.md §14 e README com o novo fluxo**
   Resolver o "a definir" sobre estratégia de migração.
   Spec: `specs/database-migrations/tasks.md` §5

---

## 9. Manifestos Kubernetes (spec: `kubernetes-deployment`)

1. **Confirmar cluster local vs. cloud**
   Depende da mesma decisão pendente do Terraform (FIAP) — item em aberto.
   Spec: `specs/kubernetes-deployment/tasks.md` §1

2. **Criar k8s/namespace.yaml**
   Spec: `specs/kubernetes-deployment/tasks.md` §2

3. **Criar k8s/secret.yaml (template)**
   Só placeholders — nunca um segredo real commitado.
   Spec: `specs/kubernetes-deployment/tasks.md` §2

4. **Criar k8s/deployment.yaml**
   `resources.requests`/`limits`, readiness/liveness contra `/health`, variáveis via
   `envFrom` do Secret.
   Spec: `specs/kubernetes-deployment/tasks.md` §2

5. **Criar k8s/service.yaml (ClusterIP)**
   Spec: `specs/kubernetes-deployment/tasks.md` §2

6. **Criar k8s/hpa.yaml**
   CPU + memória, `minReplicas`/`maxReplicas` alinhados com o Deployment.
   Spec: `specs/kubernetes-deployment/tasks.md` §2

7. **Criar k8s/migration-job.yaml e o ConfigMap das migrations**
   Job que roda as migrations (feature 8) antes do rollout da API — usa a imagem oficial
   `migrate/migrate` direto (sem build de imagem customizada) com os arquivos de
   `migrations/` montados via ConfigMap (`kubectl create configmap ... --from-file=migrations/`).
   Lembrar de regenerar o ConfigMap sempre que uma migration nova for adicionada.
   Spec: `specs/kubernetes-deployment/tasks.md` §2

8. **Instalar metrics-server localmente (se necessário)**
   Pré-requisito pro HPA reportar métricas reais.
   Spec: `specs/kubernetes-deployment/tasks.md` §3

9. **Testar aplicação completa dos manifestos**
   `kubectl apply -f k8s/` com Secret real; confirmar API acessível e HPA com métricas.
   Spec: `specs/kubernetes-deployment/tasks.md` §3

10. **Testar resiliência (delete de pod)**
    Confirmar que o Deployment substitui o pod automaticamente, passando o readiness antes
    de receber tráfego.
    Spec: `specs/kubernetes-deployment/tasks.md` §3

11. **Testar autoscaling sob carga simulada**
    Gerar carga, confirmar aumento de réplicas no HPA, depois redução — essa é a evidência
    pro vídeo.
    Spec: `specs/kubernetes-deployment/tasks.md` §3

12. **Testar o Job de migração**
    Aplicar, aguardar `condition=complete`, confirmar que roda antes do rollout da API.
    Spec: `specs/kubernetes-deployment/tasks.md` §3

13. **Documentar deploy em Kubernetes no README**
    Spec: `specs/kubernetes-deployment/tasks.md` §4

---

## 10. Provisionamento de infraestrutura — Terraform (spec: `infrastructure-provisioning`)

1. **Confirmar cluster local (kind) vs. cloud**
    Mesmo item pendente da feature 9 — decidir antes de escrever `cluster.tf` de verdade.
    Spec: `specs/infrastructure-provisioning/tasks.md` §1

2. **Criar infra/cluster.tf**
    Provider `kind` (ou cloud, conforme decisão), providers `kubernetes`/`helm` apontando
    pro cluster resultante.
    Spec: `specs/infrastructure-provisioning/tasks.md` §2

3. **Criar infra/namespace.tf**
    Spec: `specs/infrastructure-provisioning/tasks.md` §2

4. **Criar infra/metrics-server.tf**
    `helm_release` do metrics-server (com `--kubelet-insecure-tls` se local).
    Spec: `specs/infrastructure-provisioning/tasks.md` §2

5. **Criar infra/postgres.tf**
    StatefulSet + PVC + Service headless + Secret de credenciais.
    Spec: `specs/infrastructure-provisioning/tasks.md` §2

6. **Criar infra/variables.tf e infra/outputs.tf**
    Senha do Postgres como variável sensível; outputs com host/porta/template da
    DATABASE_URL.
    Spec: `specs/infrastructure-provisioning/tasks.md` §2

7. **Criar infra/terraform.tfvars.example**
    Só placeholder; confirmar `.gitignore` cobre `terraform.tfvars` e `.terraform/`.
    Spec: `specs/infrastructure-provisioning/tasks.md` §2

8. **Validar apply/destroy completo**
    `terraform apply` do zero confirmando cluster + metrics-server + Postgres acessíveis;
    `terraform destroy` sem deixar containers/volumes órfãos.
    Spec: `specs/infrastructure-provisioning/tasks.md` §3

9. **Documentar recursos e comandos no infra/README**
    Lista exata de recursos criados + comandos de apply/destroy (exigência literal do PDF).
    Spec: `specs/infrastructure-provisioning/tasks.md` §4

---

## 11. Pipeline de CI/CD (spec: `ci-cd-pipeline`)

1. **Confirmar disponibilidade/licença do GitHub Actions**
    Item em aberto — confirmar antes de implementar o job de deploy.
    Spec: `specs/ci-cd-pipeline/tasks.md` §1

2. **Job: build e push da imagem Docker pro GHCR**
    Tag pela SHA do commit + `latest`, usando `GITHUB_TOKEN` embutido.
    Spec: `specs/ci-cd-pipeline/tasks.md` §4

3. **Job: deploy — provisionar cluster kind efêmero**
    Via `helm/kind-action` (ou equivalente) dentro do próprio runner.
    Spec: `specs/ci-cd-pipeline/tasks.md` §4

4. **Job: deploy — terraform apply (metrics-server, namespace, Postgres)**
    Spec: `specs/ci-cd-pipeline/tasks.md` §4

5. **Job: deploy — rodar migrations**
    Via imagem fixada do `migrate/migrate`.
    Spec: `specs/ci-cd-pipeline/tasks.md` §4

6. **Job: deploy — criar Secret da API e aplicar manifestos**
    Substituir a tag da imagem no `deployment.yaml`, aplicar `k8s/`, aguardar rollout.
    Spec: `specs/ci-cd-pipeline/tasks.md` §4

7. **Job: deploy — smoke test e terraform destroy (sempre)**
    `curl /health`; `terraform destroy` com `if: always()`.
    Spec: `specs/ci-cd-pipeline/tasks.md` §4

8. **Cadastrar os secrets do pipeline no GitHub**
    `POSTGRES_PASSWORD`, `JWT_SECRET`, `EXTERNAL_NOTIFICATION_SECRET`,
    `EXTERNAL_STATUS_UPDATE_SECRET`, `RESEND_API_KEY`.
    Spec: `specs/ci-cd-pipeline/tasks.md` §3

9. **Rodar o pipeline manualmente de ponta a ponta**
    `workflow_dispatch` completo; confirmar que o destroy roda mesmo em falha simulada.
    Spec: `specs/ci-cd-pipeline/tasks.md` §5

10. **Documentar o pipeline no README**
    Spec: `specs/ci-cd-pipeline/tasks.md` §6

---

## 12. Entregáveis da Fase 2 (spec: `phase-2-deliverables`)

1. **Dar acesso de colaborador ao soat-architecture no GitHub**
    Não confundir com o usuário de aplicação `soat-architecture@workshop.local` do seed —
    são coisas diferentes. Pode ser feito a qualquer momento.
    Spec: `specs/phase-2-deliverables/tasks.md` §1

2. **Atualizar as collections (Bruno/Postman/Insomnia) com os endpoints novos**
    Conferir contra `docs/openapi.yaml` depois que cada feature acima estiver pronta.
    Spec: `specs/phase-2-deliverables/tasks.md` §2

3. **Escrever a seção Fase 2 do README**
    Objetivo, arquitetura, execução local, deploy K8s, Terraform, collections, vídeo —
    linkando as seções que cada spec acima já escreve por conta própria.
    Spec: `specs/phase-2-deliverables/tasks.md` §3

4. **Criar os três diagramas de arquitetura**
    Componentes da aplicação, infraestrutura provisionada, fluxo de deploy — só depois que
    cada parte estiver realmente implantada.
    Spec: `specs/phase-2-deliverables/tasks.md` §4

5. **Gravar o vídeo demonstrativo (≤15 min)**
    Roteiro: arquitetura → CI/CD → consumo das APIs → escalabilidade automática.
    Spec: `specs/phase-2-deliverables/tasks.md` §5

6. **Publicar o vídeo e linkar no README**
    YouTube ou Vimeo, público ou não listado.
    Spec: `specs/phase-2-deliverables/tasks.md` §5

7. **Montar o PDF de entrega**
    Link do repositório (com acesso já concedido), diagrama final, link do vídeo.
    Spec: `specs/phase-2-deliverables/tasks.md` §6

8. **Submeter no portal do aluno antes do prazo**
    Spec: `specs/phase-2-deliverables/tasks.md` §6

---

## 13. Notificação de orçamento por e-mail — Resend (spec: `service-order-email-notification`)

> Adicionada depois de uma auditoria das specs: `RESEND_API_KEY` já era citado em Docker/K8s/
> CI-CD, mas nenhuma spec implementava o envio de fato. Esta é a que faltava.

1. **customerRef: adicionar campo Email**
   Estender `customerRef` com `Email *string`; a query que resolve o cliente da OS no envio
   do orçamento passa a selecionar `customers.email` também.
   Spec: `specs/service-order-email-notification/tasks.md` §1

2. **Implementar ResendQuoteNotifier**
   Chamada HTTP direta à API do Resend (sem SDK) implementando a interface `QuoteNotifier`
   já existente; sem cliente com e-mail cadastrado → não faz nada (não é erro); sem link de
   rastreio no corpo do e-mail (o token bruto não existe mais nesse ponto do fluxo — é um
   hash, por design de `service-order-tracking`).
   Spec: `specs/service-order-email-notification/tasks.md` §2

3. **Configurar wiring em main.go**
   `ResendQuoteNotifier` quando `RESEND_API_KEY` estiver setado (falha se `RESEND_FROM_EMAIL`
   não vier junto); `NoOpQuoteNotifier{}` com log, sem falhar o startup, quando não estiver
   configurado — diferente dos outros segredos, que falham rápido.
   Spec: `specs/service-order-email-notification/tasks.md` §3

4. **Atualizar .env.example e docker-compose**
   `RESEND_API_KEY`/`RESEND_FROM_EMAIL`, sem fail-fast (`${VAR:-}`, não `${VAR:?...}`).
   Spec: `specs/service-order-email-notification/tasks.md` §3

5. **Testes unitários: ResendQuoteNotifier**
   Com RoundTripper fake: sucesso, cliente sem e-mail (zero chamadas), resposta não-2xx,
   erro de rede.
   Spec: `specs/service-order-email-notification/tasks.md` §4

6. **Teste de integração: fallback sem RESEND_API_KEY**
   Confirmar que `SendQuote` se comporta exatamente como antes dessa feature quando a
   variável não está setada.
   Spec: `specs/service-order-email-notification/tasks.md` §4

7. **Verificação final — notificação por e-mail**
   `go build`/`go vet`/`go test`; conferir que nenhum log vaza API key, e-mail ou nome do
   cliente; reconferir AC1–AC6.
   Spec: `specs/service-order-email-notification/tasks.md` §5
