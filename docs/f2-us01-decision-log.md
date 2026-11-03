# Log de decisões — F2-US01 (Clean Architecture no service-order)

Este log registra as decisões tomadas durante a estória F2-US01: o que foi decidido,
quando, as alternativas consideradas, por quê, e onde está a evidência. As decisões
técnicas detalhadas estão na spec ([`specs/service-order-architecture-refactor/`](../specs/service-order-architecture-refactor/),
`requirements.md` §7 e `design.md` §9); aqui fica a visão cronológica, inclusive das
decisões de processo e de branches, que não cabem na spec.

**Legenda:** ✅ em vigor · ⚠️ substituída (veja a decisão que a substitui) · ⏳ pendente

---

## 2026-10-12 — antes da implementação

### D01 ✅ Clean Architecture, e não Hexagonal
- **Decisão:** usar Clean Architecture para o requisito arquitetural da Fase 2. O enunciado aceita as duas.
- **Quem:** time, no planejamento da Fase 2.
- **Evidência:** `specs/service-order-architecture-refactor/requirements.md` §1.

## 2026-10-21 — implementação

### D02 ✅ Escopo "meio-termo"
- **Decisão:** o service-order recebe o refactor completo em camadas. Nos outros 6 módulos (auth, customer, vehicle, product, service-catalog, tracking), só as violações **por arquivo** são corrigidas. A separação em camadas **por pacote** desses módulos fica registrada como dívida técnica.
- **Alternativas:**
  - (A) só o service-order, com o resto registrado como dívida;
  - (B) refatorar os 6 módulos;
  - meio-termo (escolhido).
- **Por quê:**
  - B era muito mais trabalho e criaria conflitos com estórias em paralelo.
  - A ignorava violações baratas de corrigir.
  - A estória permite "corrigir **ou** rever explicitamente o escopo".
- **Quem:** o usuário, após a análise de trade-offs.
- **Evidência:** spec §7.2; `docs/architecture-dependency-map.md`; decisão 22 em `specs/architecture.md`.
- **Atenção:** o PO e o time devem validar no review, porque a estória diz que "só mover pastas de OS não prova conformidade da aplicação inteira".

### D03 ✅ O que conta como violação nos outros módulos
- **Decisão:** um `dto.go` importar `apierror` **não** é violação, porque DTO já é código da camada HTTP. A violação real era a porta de repositório declarada no adapter (`repository.go`), e não no consumidor (`service.go`), em customer, vehicle e product. A porta foi movida para o consumidor.
- **Por quê:** o inventário arquivo a arquivo mostrou que nenhum arquivo de regra de negócio importa infraestrutura. A proposta inicial ("tirar o apierror dos DTOs") foi corrigida com base nessa evidência.
- **Evidência:** commit `47d4e41`; `docs/architecture-dependency-map.md` §2.

### D04 ✅ Gate de cobertura agregado
- **Decisão:** a meta de 80% vale para o **agregado** de `service-order/...`, e não para cada subpacote. Os subpacotes aparecem no relatório só como informação.
- **Alternativas:** 80% por subpacote, rejeitado porque eleva a meta além da herdada e pode travar a CI por causa de um adapter pequeno.
- **Por quê:** é a migração fiel do gate antigo (mesmas instruções e mesmo denominador). O prefixo `/...` impede que uma pasta nova escape do gate.
- **Quem:** o usuário.
- **Evidência:** spec §7.4; `scripts/coverage.sh` (commit `66a57af`).

### D05 ✅ Verificação de dependências transitiva, como teste Go
- **Decisão:** o `architecture_test.go` resolve o grafo com `go list -deps` e roda dentro do `go test ./...`, sem job novo de CI. Ele segue apenas os imports que saem do **código deste módulo**; bibliotecas externas e a stdlib são folhas.
- **Por quê:**
  - A estória exige transitividade, e um grep de imports diretos, como previa a spec original, não cobre isso.
  - Folhas porque o `github.com/google/uuid`, permitido no domínio, importa `encoding/json` internamente.
- **Evidência:** commit `13d2843`. Testado com violações injetadas: direta, transitiva e biblioteca fora da lista permitida.

### D06 ✅ Onde ficam os arquivos criados depois da spec
- **Decisão:**
  - atualização de status → `usecase/lifecycle.go` e `adapter/httpapi`;
  - porta do notificador → `usecase/ports.go`;
  - Resend e no-op → `adapter/notifier`.
- **Evidência:** spec §7.3.

### D07 ✅ Divisão dos casos de uso e dos fakes de teste
- **Decisão:**
  - `ServiceOrderService` virou 7 structs (Opening, Lifecycle, Quotes, Executions, Queries, StockUsage, Metrics), cada uma recebendo só as portas que usa (17 portas, de 1 a 4 métodos).
  - Nos testes de usecase ficou um único fake em memória, porque os fluxos usam vários casos de uso sobre o mesmo estado.
  - Os testes HTTP ganharam um fake próprio, com só as portas das rotas testadas.
- **Evidência:** `design.md` §9.1/§9.2/§9.4.

### D08 ✅ Testes determinísticos para as guardas SQL, em vez de mexer no gate
- **Contexto:** a primeira medição com Postgres deu 85,4%, contra 85,5% antes. A investigação mostrou que a guarda de concorrência do `DecideQuote` só era coberta **por acaso**, conforme o agendamento do teste HTTP concorrente.
- **Decisão:** criar `TestRepositoryTransitionGuardsRejectStaleOrders`, que entrega ao repositório uma cópia desatualizada da OS, mais testes para o notificador nulo e o no-op. O gate não foi alterado.
- **Resultado:** 85,9% (1396/1625), contra o baseline de 85,5% (1383/1617).
- **Evidência:** commit `104ae19`; `tasks.md` §9.

### D09 ✅ Método de regressão: a mesma collection nas duas versões
- **Decisão:** rodar a Postman collection (newman 6.2.1) num banco zerado contra a imagem antiga e a nova, comparando o status **e o formato do corpo** de cada uma das 50 requisições.
- **Por quê:** só 8 das 50 requisições têm asserções, então rodar só na versão nova provaria pouco.
- **Resultado:** idêntico nas 50.
- **Achado, fora do escopo:** a collection desativa o cliente antes de criar o veículo (`409 CUSTOMER_INACTIVE`), e isso acontece nas duas versões.

### D10 ✅ Histórico de commits
- **Decisão:** um commit por etapa, cada um compilando e passando nos testes sozinho. O merge é feito com *merge commit*, nunca squash nem rebase.
- **Por quê:** facilita a revisão etapa a etapa e um eventual bisect, e mantém válidos os hashes citados no PR e no `review-guide.md`.

### D11 ⚠️ PR para uma linha paralela `fase-2/clean-architecture` (substituída pela D13)
- **Decisão na época:** o código clean arch não iria para a `develop`. Foi criada a `fase-2/clean-architecture` em `c6e4677` e aberto o PR #40 `chore/F2-US01-clean-architecture-service-order → fase-2/clean-architecture`.
- **Histórico de nomes:** antes disso, por um erro de interpretação, os commits foram publicados como `chore/refactor_to_clean_arch` e `feature/F2-US01-...`. As duas branches foram apagadas pelo usuário no GitHub. A `develop` nunca foi afetada.

### D12 ✅ Causa do check `security` vermelho no PR #40
- **Diagnóstico:** o `govulncheck` acusou uma vulnerabilidade de `crypto/x509` alcançável a partir de `ListenAndServe`. Ela já existia no código de antes do refactor. A CI usava o go1.26.9 em cache no runner. Localmente, com go1.26.10: 0 vulnerabilidades, tanto na base quanto na branch.
- **Correção:** feita na `develop` por outro membro do time (commit `6a396a3`, `check-latest: true`). Entra nesta branch pelo merge da D13.

## 2026-10-28 — integração na `develop`

### D13 ✅ Manter as duas versões: clean arch na `develop` e vertical slice em `vertical-slice-arch`
- **Decisão:**
  1. Preservar `develop@6779547` como a branch permanente `vertical-slice-arch`, seguindo o padrão de nomes do time.
  2. Mergear a clean arch na `develop`.
  3. Reaproveitar o PR #40, trocando a base para `develop`.
  4. A `fase-2/clean-architecture` (D11) fica obsoleta.
- **Por quê:** a Fase 2 exige a clean arch, e o time quer manter também a versão vertical slice.
- **Quem:** o usuário, por orientação do time.
- **Evidência:** a branch `vertical-slice-arch` foi criada em `6779547`, idêntica à `develop` no momento da criação.

### D14 ✅ Integração da FP-31 (valores monetários exatos) nas camadas
- **Contexto:** a `develop` recebeu a FP-31 (`float64` → `money.Cents`) depois do início do refactor. Foram 14 arquivos do service-order que o refactor tinha movido ou dividido, com 11 conflitos.
- **Decisão:**
  - Reaplicar cada mudança da FP-31 no arquivo correspondente da estrutura nova, sem alterar comportamento.
  - Nos testes, manter a versão da `develop` (valores em centavos) com os nomes do refactor.
  - O teste da FP-31 que também verificava o JSON do DTO foi **dividido**: o cálculo ficou no `usecase` e a serialização foi para o `adapter/httpapi` (`TestQuoteResponseRendersExactTotals`), com as mesmas asserções.
- **Evidência:**
  - commit de merge `1fe315c` (`develop → chore/F2-US01-...`) e tabela de mapeamento em `review-guide.md` §4.0;
  - spec §7.7;
  - os mesmos 55 comandos SQL e as mesmas conversões de dinheiro (7 × `ScanCents`, 5 × `NumericFromCents`) dos dois lados;
  - os 331 testes da `develop` presentes e verdes.

### D15 ✅ `money` no domínio: opção A, exceção documentada
- **Problema:** `internal/shared/money` importa `encoding/json` (o `Cents` sabe se serializar), o que reprovaria a regra do domínio.
- **Alternativas:**
  - (A) permitir `shared/money` como exceção: um tipo de valor tratado como folha;
  - (B) usar centavos `int64` no domínio e converter nos adapters.
- **Decisão:** A.
- **Por quê:** `money` é um tipo de valor puro, que não acessa banco nem rede. B aumentaria o código de conversão e o diff da integração, sem ganho de garantia.
- **Proteção:** `TestValueTypePackagesStayInfrastructureFree` reprova se o `money` passar a importar algo fora da stdlib, ou pacotes de rede, banco ou processo. Testado com violações injetadas: `domain → encoding/json` direto reprova; `money → net/http` reprova; `usecase → domain → net/http` reprova.
- **Quem:** o usuário.
- **Evidência:** spec §7.7; `architecture_test.go`.

### D16 ✅ Duas branches separadas, com a mesma lógica
- **Decisão (confirmada pelo usuário em 28/10):**
  - A **`develop` contém toda a lógica mantida**, com a arquitetura clean no service-order.
  - A **`vertical-slice-arch`** fica separada, com a mesma lógica na arquitetura vertical slice.
- **Consequência:** o critério de aceite da integração é **comportamento idêntico ao da `vertical-slice-arch`** (`develop@6779547`), e não ao de `c6e4677`.
- **Evidência:** `review-guide.md` §7 e `tasks.md` §11.

### D17 ✅ A queda de achados do `gosec` (6 → 0) não é correção
- **Achado:** o `gosec` aponta 6 achados na `vertical-slice-arch` (2 × G704 SSRF no Resend, 4 × G706 log injection nos logs da notificação) e 0 na branch.
- **Explicação:** a análise de *taint* do `gosec` só rastreia dentro de um pacote. Com a origem HTTP e os destinos em pacotes diferentes depois do refactor, a ferramenta deixa de ligar os dois. As linhas apontadas são **o mesmo código**, e na prática são falsos positivos: o log só formata `order.Code`, um `int64`, e a URL do Resend é constante.
- **Decisão:** registrar como efeito da ferramenta, para ninguém ler o relatório da CI como melhoria de segurança. O `gosec` roda só como relatório na CI (`-no-fail`).
- **Evidência:** `review-guide.md` §7.

### D18 ✅ Medições de cobertura com o Go da CI (1.26.10)
- **Achado:** o mesmo código conta 1639 instruções do service-order no Go 1.27 (o local) e 1390 no Go 1.26.10 (o da CI). Os números de 21/10, medidos no 1.27, não são comparáveis com os de 28/10.
- **Decisão:** baseline e branch passam a ser medidos sempre com `GOTOOLCHAIN=go1.26.10`.
- **Resultado em 28/10:** 82,7% (1143/1382) na `vertical-slice-arch` → **83,4% (1159/1390)** na branch.

### D19 ✅ Cópia da collection só para a regressão
- **Achado:** a collection versionada não exercita o fluxo da OS. Ela desativa o cliente antes de criar o veículo e fixa a placa `ABC1D23`, que o seed já usa.
- **Decisão:** a regressão usa, além da collection original, uma **cópia temporária** (não versionada) com as requisições de desativar/excluir no final e outra placa. As duas versões rodam essa mesma cópia. A collection versionada **não foi alterada** (P5).
- **Resultado:** 50/50 status idênticos. Os corpos são idênticos, exceto o tempo de relógio de uma execução criada durante o teste. O fluxo completo da OS rodou, com orçamento de 309,70 nas duas versões.

---

## Pendências e riscos em aberto

| # | Item | Responsável | Status |
|---|---|---|---|
| P1 | Congelar merges na `develop` até o merge do PR #40 | time | ⏳ alinhar |
| P2 | Definir o papel da `vertical-slice-arch`: só referência congelada, ou recebe correções? Quem espelha mudanças entre as duas linhas? | time | ⏳ alinhar |
| P3 | Avisar o time para atualizar as branches locais depois do merge, porque quem mexe no service-order vai conflitar | usuário | ⏳ |
| P4 | Validar com o PO que o escopo do inventário (D02) atende a avaliação | PO/time | ⏳ |
| P5 | Postman collection: desativa o cliente antes de criar o veículo e fixa a placa `ABC1D23`, que o seed já usa; por isso não exercita o fluxo da OS | — | task separada |
| P6 | Compose: o `migrate` pode falhar na primeira subida de um volume novo (healthcheck durante o initdb) | — | task separada |
| P7 | Caracteres corrompidos em `specs/architecture.md` e `AGENTS.md` defasado em relação ao `CLAUDE.md` (anteriores à F2-US01) | — | task separada |
| P8 | Insomnia e Bruno não executadas; só a do Postman | — | risco baixo, rotas idênticas |

## Decisões críticas aguardando o usuário (nada disso foi executado)

| # | Ação | Por que é crítica |
|---|---|---|
| A | Push da branch com o merge `1fe315c` | Publica a integração e atualiza o PR #40 |
| B | Mudar a base do PR #40 de `fase-2/clean-architecture` para `develop` | Deixa o PR a um clique de alterar a `develop` |
| C | Mergear o PR #40 na `develop` | Na prática irreversível (só se desfaz com revert); depende do review, de P1 e de P3 |
| D | Apagar a `fase-2/clean-architecture` | Remoção no remoto; só depois do C |
| E | Proteger a `vertical-slice-arch` no GitHub (exigir PR) | Configuração do repositório |
