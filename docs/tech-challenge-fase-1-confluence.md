# Tech Challenge Fase 1 — Oficina Mecânica | Domínio, Arquitetura e Backlog

> Documento revisado a partir da implementação existente no repositório até 27/08/2026.

**Status:** MVP implementado, com pendências documentais e de integração externa descritas neste documento.

**Fonte DDD:** [Board Miro — Tech Challenge Fase 1](https://miro.com/app/board/uXjVH_w7VIA=/?share_link_id=137091402354)

**Arquitetura implementada:** monólito modular organizado por Vertical Slice Architecture.

**Objetivo:** consolidar a visão do domínio, os fluxos levantados no Event Storming, a arquitetura efetivamente implementada, os requisitos, o backlog e o estado atual do MVP de gestão de oficina mecânica.

## Membros do time

- JEAN FERREIRA DOS SANTOS CRUZ JUNIOR (RM376169)
- João Victor dos Santos Cerqueira (RM376742)
- Giovane Kenuy Soares Colognesi Rubira Cardona (RA a informar)
- Augusto José Rizzi (RA a informar)
- Lucas Guimarães Fabris (RM376444)

# 1. Visão geral da solução

## 1.1 Problema

A oficina mecânica precisa administrar atendimento, diagnóstico, orçamento, execução e entrega de veículos. Quando essas informações ficam dispersas em anotações manuais e planilhas, surgem erros de priorização, controle precário de peças e insumos, baixa visibilidade sobre o andamento dos serviços, perda de histórico e dificuldade para obter e registrar a decisão do cliente sobre o orçamento.

## 1.2 Solução

O produto é uma API REST para gestão integrada do ciclo de atendimento de uma oficina mecânica. A aplicação centraliza clientes, veículos, catálogo de serviços, peças e insumos, ordens de serviço, orçamentos, execução, estoque, acompanhamento pelo cliente e métricas operacionais.

A equipe interna usa operações administrativas protegidas por JWT. O cliente acompanha uma ordem de serviço e responde ao orçamento por uma projeção pública reduzida, protegida por um token específico da OS enviado no cabeçalho `X-Tracking-Token`.

## 1.3 Objetivos do produto

- Centralizar a operação da oficina em uma única fonte de dados.
- Rastrear alterações de status da ordem de serviço.
- Compor orçamentos a partir de serviços, peças e insumos, preservando a fotografia de descrição e preço.
- Registrar o envio do orçamento e a decisão do cliente.
- Controlar e auditar entradas, saídas e estornos de estoque.
- Registrar início e término da execução de cada serviço.
- Disponibilizar acompanhamento seguro sem expor dados pessoais do cliente.
- Fornecer métricas de tempo médio de execução por serviço.
- Manter uma base modular, testável e preparada para evolução.

## 1.4 Escopo implementado no MVP

- Autenticação administrativa com JWT e identificação do usuário autenticado.
- Cadastro, consulta, atualização e inativação lógica de clientes.
- Cadastro, consulta, atualização e inativação lógica de veículos.
- Catálogo de serviços com inativação lógica.
- Catálogo de produtos dos tipos `PART` e `SUPPLY`, saldo e movimentações de estoque.
- Criação, listagem e detalhamento de ordens de serviço.
- Registro de diagnóstico, composição versionada e envio de orçamento.
- Aprovação ou reprovação do orçamento pelo cliente.
- Acompanhamento público da OS com token próprio e projeção reduzida.
- Registro de início e término da execução de serviços.
- Finalização e entrega da ordem de serviço.
- Baixa de produtos usados em uma OS, consulta de movimentos e estorno.
- Métrica de tempo médio de execução por serviço.
- Persistência em PostgreSQL, execução com Docker Compose e documentação OpenAPI.
- Testes unitários e de integração, cobertura mínima automatizada e análise de vulnerabilidades.

## 1.5 Fora do escopo atual

- Aplicativo web ou mobile para clientes e funcionários.
- Integração real com e-mail, mensageria ou outro canal de notificação.
- Pagamento, faturamento, emissão fiscal e integração com fornecedores.
- Agendamento de serviços.
- Perfis e autorização por papel; o JWT atual autentica o usuário administrativo, mas não diferencia permissões por função.
- Decomposição em microserviços.

# 2. Estado atual e ressalvas de consistência

| Tema                         | Estado observado na aplicação                                                                                                                                                                                                                                         |
| ---------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Fluxo principal da OS        | Implementado de `RECEIVED` até `DELIVERED`. |
| Reprovação de orçamento      | Decisão resolvida: a cotação fica `REJECTED` e a OS passa para `CANCELED`. |
| Notificação do orçamento     | O envio e a data de envio são registrados, mas o notificador configurado é `NoOpQuoteNotifier`; nenhum e-mail é efetivamente enviado. |
| Proteção administrativa      | Todas as rotas administrativas usam JWT. Permanecem públicas apenas `/health`, o login e as rotas de acompanhamento protegidas pelo token específico da OS. |
| Acompanhamento do cliente    | Implementado sem JWT administrativo, usando `X-Tracking-Token`. Somente o hash do token é persistido. |
| OpenAPI                      | O arquivo descreve as 47 operações registradas pela aplicação, incluindo `/health` e as três rotas de movimentação de estoque vinculadas à OS. |
| Serviços inativos em uma OS | O catálogo possui o campo `active`, mas as consultas feitas por `service-order` verificam somente a existência do serviço. Na implementação atual, um serviço inativo ainda pode ser solicitado, orçado ou iniciado em uma execução. |
| Recomposição após envio      | `ComposeQuote` bloqueia apenas OS em `RECEIVED` e orçamento já decidido. Assim, o código ainda permite recompor um orçamento `PENDING` depois do envio, embora `SendQuote` só possa ser chamado em `IN_DIAGNOSIS`; essa combinação deve ser revista. |
| Qualidade                    | Há jobs de build, testes, cobertura e segurança na CI. Os três pacotes críticos possuem gate mínimo de 80%.                                                                                                                                                        |
| Segurança                   | `gosec` e `govulncheck` foram definidos e integrados. O relatório registra zero achados SAST e nenhuma vulnerabilidade alcançável após a atualização para Go 1.25.                                                                                            |

# 3. Linguagem ubíqua

Os identificadores de domínio, código e banco são escritos em inglês, incluindo os valores de status da ordem de serviço. As rotas públicas de acompanhamento (`/acompanhamento/{codigo}` e `orcamento/aprovar|reprovar`) permanecem como a única exceção de nomenclatura; trata-se de uma escolha de contrato HTTP, não de linguagem do domínio persistido.

| Termo                     | Nome no código         | Definição                                                                                     |
| ------------------------- | ----------------------- | ----------------------------------------------------------------------------------------------- |
| Cliente                   | `Customer`            | Pessoa física ou jurídica que solicita atendimento para um veículo.                          |
| Veículo                  | `Vehicle`             | Bem atendido pela oficina e vinculado a um cliente.                                             |
| Serviço                  | `Service`             | Atividade oferecida pela oficina, com preço e tempo estimado opcional.                         |
| Produto                   | `Product`             | Peça (`PART`) ou insumo (`SUPPLY`) mantido no catálogo e no estoque.                      |
| Ordem de serviço         | `ServiceOrder`        | Agregado central do atendimento, do recebimento à entrega ou ao cancelamento.                  |
| Serviço solicitado       | `RequestedService`    | Serviço indicado na abertura da OS; representa a demanda inicial, ainda sem preço congelado.  |
| Diagnóstico              | `Diagnosis`           | Etapa que leva a OS de `RECEIVED` para `IN_DIAGNOSIS`.                                     |
| Orçamento                | `Quote`               | Proposta comercial única da OS, versionável e composta por itens de produto e serviço.       |
| Item de orçamento        | `QuoteItem`           | Fotografia da descrição, quantidade, valor unitário e total aplicada na composição.        |
| Aprovação               | `APPROVED`            | Decisão que autoriza a execução e move a OS para `IN_PROGRESS`.                             |
| Reprovação              | `REJECTED`            | Decisão que encerra o fluxo operacional e move a OS para `CANCELED`.                         |
| Execução de serviço    | `ServiceExecution`    | Registro com identificador próprio, início e término de um serviço executado na OS.         |
| Movimentação de estoque | `StockMovement`       | Entrada ou saída rastreável, opcionalmente vinculada à OS e a um movimento estornado.        |
| Histórico da OS          | `ServiceOrderHistory` | Registro imutável de eventos e transições da OS.                                             |
| Token de acompanhamento   | `TrackingToken`       | Segredo emitido uma única vez na abertura da OS para acesso do cliente à projeção pública. |

# 4. Descoberta de domínio

## 4.1 Artefatos do Miro

O board do Tech Challenge contém:

- Domain Storytelling do processo de criação e acompanhamento da OS — caminho feliz.
- Domain Storytelling do processo de criação e acompanhamento da OS — caminho alternativo.
- Event Storming em ordem horizontal.
- Event Storming por agregado.
- Diagrama C4.

## 4.2 Caminho feliz implementado

1. A equipe registra ou consulta cliente e veículo.
2. Uma OS é aberta em `RECEIVED`, com cliente, veículo, observações e serviços inicialmente solicitados.
3. O início do diagnóstico move a OS para `IN_DIAGNOSIS`.
4. O orçamento é composto com produtos e serviços. O servidor calcula os totais e preserva descrição e preço aplicados.
5. O orçamento pode ser recomposto enquanto estiver pendente; sua versão é incrementada. A implementação também permite essa recomposição depois do primeiro envio, comportamento registrado como pendência técnica.
6. O envio registra `sentAt` e `sentVersion` e move a OS para `AWAITING_APPROVAL`.
7. O cliente aprova o orçamento com o token de acompanhamento.
8. A cotação passa para `APPROVED` e a OS para `IN_PROGRESS`.
9. A equipe registra início e término dos serviços executados.
10. Produtos consumidos são baixados do estoque em uma transação rastreável.
11. Após todas as execuções exigidas terminarem, a OS passa para `COMPLETED`.
12. A entrega do veículo move a OS para `DELIVERED`.

## 4.3 Caminho de reprovação implementado

1. O orçamento é enviado e a OS fica em `AWAITING_APPROVAL`.
2. O cliente reprova o orçamento usando o token de acompanhamento.
3. O orçamento passa de `PENDING` para `REJECTED`.
4. A OS passa de `AWAITING_APPROVAL` para `CANCELED`.
5. O histórico registra o evento de cancelamento.

## 4.4 Comandos, eventos e políticas

| Ação de negócio       | Evento ou efeito persistido | Política implementada                                                                                                                                        |
| ------------------------ | --------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Abrir OS                 | `creation`                | A OS sempre nasce em `RECEIVED`.                                                                                                                             |
| Iniciar diagnóstico     | `diagnosis_started`       | Permitido somente em `RECEIVED`.                                                                                                                             |
| Compor orçamento        | `quote_composed`          | Exige que o diagnóstico tenha começado. Produtos devem existir e estar ativos; serviços são verificados somente por existência na implementação atual. |
| Recompor orçamento      | `quote_composed`          | Permitido enquanto a cotação está `PENDING`; incrementa a versão, inclusive após o envio na implementação atual.                                      |
| Enviar orçamento        | `quote_sent`              | Move a OS para `AWAITING_APPROVAL` e registra a versão enviada.                                                                                          |
| Aprovar orçamento       | `approval`                | Move a OS para `IN_PROGRESS`.                                                                                                                                |
| Reprovar orçamento      | `cancellation`            | Move a OS para `CANCELED`.                                                                                                                                  |
| Finalizar OS             | `completion`              | Exige OS em execução e execuções obrigatórias concluídas.                                                                                               |
| Entregar veículo        | `delivery`                | Permitido somente em `COMPLETED`.                                                                                                                           |
| Registrar uso de estoque | Movimento `EXIT`          | Permitido em `IN_PROGRESS`, sem saldo negativo e de forma atômica.                                                                                          |
| Estornar uso             | Movimento `ENTRY`         | Restaura saldo e referencia o movimento original.                                                                                                             |

# 5. Modelo de domínio e persistência

## 5.1 Contextos conceituais

| Contexto            | Responsabilidade                                                                         | Implementação principal                                                            |
| ------------------- | ---------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| Identidade e acesso | Login, emissão e validação do JWT.                                                    | `auth` e componentes compartilhados de token/middleware.                           |
| Atendimento         | Clientes e veículos.                                                                    | `customer` e `vehicle`.                                                          |
| Catálogo e estoque | Serviços, produtos, saldos e movimentos.                                                | `service-catalog`, `product` e parte de `service-order`.                       |
| Ordem de serviço   | Abertura, diagnóstico, orçamento, decisão, execução, estoque, consulta e métricas. | `service-order`.                                                                   |
| Acompanhamento      | Projeção reduzida para o cliente.                                                      | `service-order-tracking` e decisões públicas de orçamento em `service-order`. |

As fronteiras acima são conceituais. No código, cada feature possui seus próprios handlers, serviços, modelos e repositórios; uma feature não importa diretamente outra feature.

## 5.2 Entidades e campos relevantes

### User

- `id`, `code`, `name`, `email`, `passwordHash`, `createdAt`, `updatedAt`.
- A senha é persistida somente como hash bcrypt.

### Customer

- `id`, `code`, `name`, `document`, `documentType`, `phone`, `email`, `status`, `createdAt`, `updatedAt`.
- `documentType`: `CPF` ou `CNPJ`.
- `status`: `ACTIVE` ou `INACTIVE`.
- O documento é normalizado, validado e único.

### Vehicle

- `id`, `code`, `licensePlate`, `brand`, `model`, `year`, `color`, `customerId`, `status`, `createdAt`, `updatedAt`.
- `status`: `ACTIVE` ou `INACTIVE`.
- A placa aceita os formatos brasileiro antigo e Mercosul, é normalizada e única.
- Cliente e placa não podem ser trocados na atualização.

### Product

- `id`, `code`, `name`, `description`, `unitPrice`, `currentStock`, `type`, `status`, `createdAt`, `updatedAt`.
- `type`: `PART` ou `SUPPLY`.
- `status`: `ACTIVE` ou `INACTIVE`.
- Preço e estoque não podem ser negativos.

### Service

- `id`, `code`, `name`, `description`, `price`, `estimatedTime`, `active`, `createdAt`, `updatedAt`.
- `estimatedTime` é opcional e, quando informado, deve ser maior que zero.
- A exclusão é lógica por meio do campo booleano `active`.

### ServiceOrder

- `id`, `code`, `customerId`, `vehicleId`, `openedAt`, `status`, `notes`, `createdAt`, `updatedAt`.
- Serviços solicitados na abertura são persistidos na relação `service_order_requested_services`.
- O orçamento não é uma coluna embutida: a relação física 1:1 é mantida por `quotes.service_order_id`, que é único.

### Quote

- `id`, `code`, `serviceOrderId`, `totalAmount`, `status`, `version`, `generatedAt`, `sentAt`, `sentVersion`, `respondedAt`, `createdAt`, `updatedAt`.
- `status`: `PENDING`, `APPROVED` ou `REJECTED`.
- Produtos e serviços são persistidos separadamente em `quote_products` e `quote_services`.
- Descrição e valores aplicados são fotografados no momento da composição.

### ServiceOrderHistory

- `id`, `serviceOrderId`, `occurredAt`, `event`, `description`, `previousStatus`, `newStatus`.
- Eventos: `creation`, `diagnosis_started`, `quote_composed`, `quote_sent`, `approval`, `completion`, `cancellation` e `delivery`.

### ServiceExecution

- Persistido na tabela `audit_services`.
- Campos: `id`, `serviceOrderId`, `serviceId`, `startedAt`, `endedAt`.
- Cada linha representa uma execução identificável, e não dois eventos independentes.

### StockMovement

- `id`, `productId`, `serviceOrderId`, `type`, `quantity`, `previousStock`, `newStock`, `reason`, `reversedMovementId`, `occurredAt`.
- `type`: `ENTRY` ou `EXIT`.
- O mesmo ledger atende ajustes manuais de produto e movimentos vinculados a ordens de serviço.

### ServiceOrderTrackingToken

- `id`, `serviceOrderId`, `tokenHash`, `createdAt`, `revokedAt`.
- Existe um token por OS. O valor bruto é devolvido somente na resposta de criação; apenas seu hash é armazenado.

## 5.3 Regras de negócio principais

- Um veículo deve estar vinculado a um cliente existente e ativo no momento do cadastro.
- Uma OS deve referenciar cliente e veículo existentes, ativos e compatíveis entre si.
- A abertura pode identificar o cliente por UUID ou documento e o veículo por UUID ou placa, nunca pelas duas alternativas simultaneamente.
- Documento e placa devem ser normalizados e validados.
- A OS sempre nasce em `RECEIVED`; o cliente não escolhe o status inicial.
- Apenas transições de status previstas são aceitas.
- Toda transição relevante gera uma entrada no histórico.
- O orçamento contém ao menos um item válido e o servidor calcula os totais.
- Produtos inativos são rejeitados na composição; o filtro equivalente para serviços inativos ainda não foi aplicado nas consultas do módulo de OS.
- O orçamento enviado registra qual versão foi apresentada ao cliente.
- Uma decisão de orçamento é imutável.
- Uma execução não pode terminar duas vezes nem terminar antes de iniciar.
- A finalização exige que as execuções necessárias estejam concluídas.
- A baixa de estoque ocorre de forma atômica e não pode gerar saldo negativo.
- Um movimento estornado não pode ser estornado novamente.
- O acompanhamento público não expõe documento, telefone, e-mail, IDs internos, itens de orçamento ou observações livres.

# 6. Ciclo de vida da ordem de serviço

| Status atual             | Gatilho                                                  | Próximo status          |
| ------------------------ | -------------------------------------------------------- | ------------------------ |
| `RECEIVED`          | Início do diagnóstico                                  | `IN_DIAGNOSIS`       |
| `IN_DIAGNOSIS`      | Envio do orçamento                                      | `AWAITING_APPROVAL`  |
| `AWAITING_APPROVAL` | Aprovação do cliente                                    | `IN_PROGRESS`        |
| `AWAITING_APPROVAL` | Reprovação do cliente                                   | `CANCELED`           |
| `IN_PROGRESS`       | Conclusão das execuções obrigatórias e finalização     | `COMPLETED`          |
| `COMPLETED`         | Entrega do veículo                                      | `DELIVERED`          |

```text
RECEIVED
   └── diagnóstico ──> IN_DIAGNOSIS
                          └── envio ──> AWAITING_APPROVAL
                                           ├── aprovação ──> IN_PROGRESS ──> COMPLETED ──> DELIVERED
                                           └── reprovação ─> CANCELED
```

# 7. Arquitetura implementada

## 7.1 Decisão

A aplicação é um monólito modular em Go, organizado por Vertical Slice Architecture. Cada feature reúne os elementos HTTP, regras de aplicação e domínio e persistência relacionados àquela capacidade. Componentes realmente transversais ficam em `internal/shared`.

Essa organização mantém um único processo e um único banco no MVP, reduz o acoplamento entre capacidades e permite testar cada feature por interfaces próprias. As fronteiras podem orientar uma futura extração para serviços independentes, mas microserviços não fazem parte desta fase.

## 7.2 Stack

| Elemento             | Implementação                                                                 |
| -------------------- | ------------------------------------------------------------------------------- |
| Linguagem            | Go 1.25                                                                         |
| HTTP                 | Biblioteca padrão `net/http` e `http.ServeMux` com padrões por método     |
| Persistência        | PostgreSQL 16                                                                   |
| Acesso a dados       | `pgx/v5`, sem ORM                                                             |
| Autenticação       | JWT HS256 e middleware próprio                                                 |
| Senhas               | bcrypt                                                                          |
| Identificadores      | UUID técnico e código sequencial legível                                     |
| Contrato de API      | OpenAPI 3.0 em `docs/openapi.yaml`                                             |
| Infraestrutura local | Dockerfile e Docker Compose                                                     |
| Testes               | `testing` e `testify`, com testes unitários e integração HTTP/PostgreSQL |
| Segurança           | `gosec` e `govulncheck`                                                     |

## 7.3 Estrutura real do projeto

```text
cmd/api/main.go
  Ponto de entrada, configuração, conexão, composição das dependências e rotas.

internal/features/
  auth/                       Login administrativo e identidade atual.
  customer/                   Gestão de clientes.
  vehicle/                    Gestão de veículos.
  service-catalog/            Catálogo de serviços.
  product/                    Produtos, saldo e ajustes de estoque.
  service-order/              Abertura, diagnóstico, orçamento, decisão,
                              consulta, execução, estoque e métricas.
  service-order-tracking/     Acompanhamento público por token.
  user/                       Placeholder sem implementação funcional.

internal/shared/
  apierror/                   Envelope e mapeamento de erros HTTP.
  config/                     Configuração por variáveis de ambiente.
  database/                   Pool PostgreSQL compartilhado.
  document/                   Normalização e validação de CPF/CNPJ.
  httpx/                      Escrita JSON e envelope legado de erros.
  middleware/                 Autenticação JWT.
  token/                      Emissão e validação do JWT.
  trackingtoken/              Geração e hash do token de acompanhamento.

docs/
  schema.sql, seed.sql, entities.md, openapi.yaml,
  coleções Postman e Insomnia e relatório de segurança.

specs/
  Requisitos, desenho e tarefas por feature no processo SDD.
```

## 7.4 Dependências e comunicação

- `cmd/api/main.go` funciona como composition root e injeta repositórios, serviços e middleware.
- As features não importam diretamente outras features.
- Quando uma capacidade precisa consultar outra, a feature consumidora declara uma interface pequena e o `main` fornece um adaptador.
- Todas as features usam o mesmo pool PostgreSQL.
- O pacote `service-order` e o pacote `product` acessam o ledger compartilhado de estoque por seus próprios repositórios SQL.
- Há uma duplicação conhecida entre `internal/shared/httpx` e `internal/shared/apierror`; a documentação registra a situação, mas não a apresenta como padrão desejável.

## 7.5 Persistência

O PostgreSQL mantém entidades, relacionamentos, históricos, tokens com hash e movimentos de estoque. O schema usa:

- chaves técnicas UUID;
- códigos sequenciais para consulta humana;
- enums nativos para estados controlados;
- chaves estrangeiras e restrições de integridade;
- transações nas mudanças coordenadas de status, orçamento e estoque;
- índices para relacionamentos e consultas recorrentes;
- triggers de atualização de `updated_at` nas entidades cadastrais.

Não há ferramenta de migração incremental configurada. `docs/schema.sql` e `docs/seed.sql` são montados em `docker-entrypoint-initdb.d` e executados automaticamente somente na criação inicial do volume PostgreSQL. Em desenvolvimento, uma alteração de schema exige recriar o volume para reaplicar os scripts; essa operação descarta os dados locais.

# 8. Segurança e acesso

## 8.1 Modelo de autenticação atual

| Superfície                                          | Proteção atual                                  |
| ---------------------------------------------------- | ------------------------------------------------- |
| `POST /api/v1/auth/login`                          | Pública; recebe credenciais e emite JWT.         |
| `GET /api/v1/auth/me`                              | JWT administrativo.                               |
| Clientes                                             | JWT administrativo.                               |
| Veículos, serviços e produtos                      | JWT administrativo.                               |
| Criação de OS                                      | JWT administrativo.                               |
| Demais operações administrativas da OS             | JWT administrativo.                               |
| Acompanhamento e decisão de orçamento pelo cliente | Token próprio no cabeçalho `X-Tracking-Token`. |

Todas as rotas administrativas, incluindo clientes e abertura de OS, são protegidas pelo mesmo middleware JWT. As únicas exceções intencionais são o health check, o login e o acompanhamento/decisão do cliente, que valida o token específico da OS em vez do JWT administrativo.

## 8.2 Controles implementados

- Senhas armazenadas com bcrypt.
- Segredo e validade do JWT definidos por variáveis de ambiente.
- Consultas SQL parametrizadas.
- Respostas de erro sem stack trace ou detalhes internos.
- Token de acompanhamento armazenado somente como hash.
- Projeção pública sem dados pessoais e sem detalhes internos do orçamento.
- Timeouts explícitos no servidor HTTP.
- SAST com `gosec` e análise de vulnerabilidades com `govulncheck`.

## 8.3 Riscos residuais documentados

- Não há autorização por papéis ou escopos.
- As credenciais de desenvolvimento do seed não podem ser reutilizadas em produção.
- A integração de notificação ainda é nula.

# 9. Contrato HTTP implementado

## 9.1 Convenções

- Prefixo principal: `/api/v1`.
- JSON para requisições e respostas.
- `Authorization: Bearer <jwt>` nas rotas administrativas protegidas.
- `X-Tracking-Token: <token>` nas rotas públicas do cliente.
- Listagens paginadas usam `page`, `pageSize`, `total` e `totalPages`.
- Exclusões de clientes, veículos, produtos e serviços são lógicas.

## 9.2 Rotas

| Grupo          | Método e rota                                                             | Proteção                                     |
| -------------- | -------------------------------------------------------------------------- | ---------------------------------------------- |
| Saúde         | `GET /health`                                                            | Pública                                       |
| Autenticação | `POST /api/v1/auth/login`                                                | Pública                                       |
| Autenticação | `GET /api/v1/auth/me`                                                    | JWT                                            |
| Clientes       | `POST /api/v1/customers`                                                 | JWT                                            |
| Clientes       | `GET /api/v1/customers`                                                  | JWT                                            |
| Clientes       | `GET /api/v1/customers/{id}`                                             | JWT                                            |
| Clientes       | `GET /api/v1/customers/document/{document}`                              | JWT                                            |
| Clientes       | `PATCH /api/v1/customers/{id}`                                           | JWT                                            |
| Clientes       | `DELETE /api/v1/customers/{id}`                                          | JWT                                            |
| Veículos      | `POST /api/v1/vehicles`                                                  | JWT                                            |
| Veículos      | `GET /api/v1/vehicles`                                                   | JWT                                            |
| Veículos      | `GET /api/v1/vehicles/{id}`                                              | JWT                                            |
| Veículos      | `GET /api/v1/vehicles/plate/{plate}`                                     | JWT                                            |
| Veículos      | `GET /api/v1/vehicles/customer/{customerId}`                             | JWT                                            |
| Veículos      | `PATCH /api/v1/vehicles/{id}`                                            | JWT                                            |
| Veículos      | `DELETE /api/v1/vehicles/{id}`                                           | JWT                                            |
| Serviços      | `POST /api/v1/services`                                                  | JWT                                            |
| Serviços      | `GET /api/v1/services`                                                   | JWT                                            |
| Serviços      | `GET /api/v1/services/{id}`                                              | JWT                                            |
| Serviços      | `PATCH /api/v1/services/{id}`                                            | JWT                                            |
| Serviços      | `DELETE /api/v1/services/{id}`                                           | JWT                                            |
| Produtos       | `POST /api/v1/products`                                                  | JWT                                            |
| Produtos       | `GET /api/v1/products`                                                   | JWT                                            |
| Produtos       | `GET /api/v1/products/{id}`                                              | JWT                                            |
| Produtos       | `PATCH /api/v1/products/{id}`                                            | JWT                                            |
| Produtos       | `DELETE /api/v1/products/{id}`                                           | JWT                                            |
| Produtos       | `POST /api/v1/products/{id}/stock/adjustments`                           | JWT                                            |
| Produtos       | `GET /api/v1/products/{id}/stock`                                        | JWT                                            |
| Produtos       | `GET /api/v1/products/{id}/movements`                                    | JWT                                            |
| OS             | `POST /api/v1/service-orders`                                            | JWT                                            |
| OS             | `GET /api/v1/service-orders`                                             | JWT                                            |
| OS             | `GET /api/v1/service-orders/{id}`                                        | JWT; `{id}` aceita UUID ou código sequencial |
| OS             | `POST /api/v1/service-orders/{id}/diagnosis`                             | JWT                                            |
| OS             | `PUT /api/v1/service-orders/{id}/quote`                                  | JWT                                            |
| OS             | `GET /api/v1/service-orders/{id}/quote`                                  | JWT                                            |
| OS             | `POST /api/v1/service-orders/{id}/quote/send`                            | JWT                                            |
| Execução     | `POST /api/v1/service-orders/{id}/executions`                            | JWT                                            |
| Execução     | `POST /api/v1/service-orders/{id}/executions/{executionId}/finish`       | JWT                                            |
| Execução     | `POST /api/v1/service-orders/{id}/finalize`                              | JWT                                            |
| Execução     | `POST /api/v1/service-orders/{id}/deliver`                               | JWT                                            |
| Estoque da OS  | `POST /api/v1/service-orders/{id}/stock-movements`                       | JWT                                            |
| Estoque da OS  | `GET /api/v1/service-orders/{id}/stock-movements`                        | JWT                                            |
| Estoque da OS  | `POST /api/v1/service-orders/{id}/stock-movements/{movementId}/reversal` | JWT                                            |
| Métricas      | `GET /api/v1/service-orders/metrics/average-execution-time`              | JWT                                            |
| Acompanhamento | `GET /api/v1/acompanhamento/{codigo}`                                    | `X-Tracking-Token`                           |
| Acompanhamento | `POST /api/v1/acompanhamento/{codigo}/orcamento/aprovar`                 | `X-Tracking-Token`                           |
| Acompanhamento | `POST /api/v1/acompanhamento/{codigo}/orcamento/reprovar`                | `X-Tracking-Token`                           |

> São 47 rotas registradas no servidor e 47 operações descritas no OpenAPI. A paridade de caminhos e métodos foi restabelecida em 27/08/2026.

# 10. Requisitos funcionais

| ID   | Requisito                                                                                  | Estado                                     |
| ---- | ------------------------------------------------------------------------------------------ | ------------------------------------------ |
| RF01 | Cadastrar, consultar, atualizar e inativar clientes.                                       | Implementado                               |
| RF02 | Cadastrar, consultar, atualizar e inativar veículos vinculados a clientes.                | Implementado                               |
| RF03 | Gerenciar serviços, peças e insumos com saldo de estoque.                                | Implementado                               |
| RF04 | Criar OS com cliente, veículo, observações, serviços solicitados e status `RECEIVED`. | Implementado                               |
| RF05 | Registrar diagnóstico e compor orçamento com serviços, peças e insumos.                | Implementado                               |
| RF06 | Calcular o total, versionar e registrar o envio do orçamento.                             | Implementado; envio externo não integrado |
| RF07 | Registrar aprovação ou reprovação do orçamento.                                       | Implementado                               |
| RF08 | Atualizar o status da OS somente pelas transições permitidas.                            | Implementado                               |
| RF09 | Registrar início e fim de execução de cada serviço.                                    | Implementado                               |
| RF10 | Baixar, consultar e estornar estoque usado em uma OS.                                      | Implementado                               |
| RF11 | Listar e detalhar OS com cliente, veículo, orçamento, status e histórico.               | Implementado                               |
| RF12 | Permitir acompanhamento seguro da OS pelo cliente.                                         | Implementado com token próprio            |
| RF13 | Disponibilizar métrica de tempo médio de execução por serviço.                        | Implementado                               |
| RF14 | Documentar as APIs REST em OpenAPI.                                                        | Implementado; 47 de 47 rotas documentadas  |

# 11. Requisitos não funcionais

| ID    | Requisito                                                     | Evidência atual                                                            |
| ----- | ------------------------------------------------------------- | --------------------------------------------------------------------------- |
| RNF01 | Back-end monolítico com organização Vertical Slice.        | `cmd/api` e `internal/features/*`                                       |
| RNF02 | Autenticação JWT nas APIs administrativas.                  | Implementada em todas as rotas administrativas                            |
| RNF03 | Validar CPF/CNPJ, placa e entradas sensíveis.                | Validadores próprios e validação por camada                              |
| RNF04 | APIs REST JSON com códigos HTTP e erros consistentes.        | Implementado; permanece duplicação técnica entre `httpx` e `apierror` |
| RNF05 | Dockerfile, Docker Compose e README para execução local.    | Implementado                                                                |
| RNF06 | Cobertura mínima de 80% nos domínios críticos.             | Gate em CI para `service-order`, `product` e `service-order-tracking`  |
| RNF07 | Integridade transacional em alterações de estoque e status. | Transações PostgreSQL nas operações coordenadas                         |
| RNF08 | Não registrar tokens, senhas ou documentos completos.        | Regras e testes de dados sensíveis; token de tracking persistido como hash |
| RNF09 | SAST e análise de dependências com relatório.              | `gosec`, `govulncheck` e `docs/security-report.md`                    |
| RNF10 | OpenAPI atualizado junto ao código.                          | Implementado; 47 operações para as 47 rotas registradas                    |

# 12. Estratégia de testes e qualidade

| Tipo         | Cobertura                                                                                                                      |
| ------------ | ------------------------------------------------------------------------------------------------------------------------------ |
| Unitário    | Entidades, validações, transições, orçamento, execução, estoque, autenticação e serviços de aplicação.             |
| Integração | Handlers HTTP e persistência real em PostgreSQL; os testes são ignorados quando `DATABASE_URL` não está disponível.      |
| Contrato     | Códigos HTTP, JSON e cenários de autenticação cobertos pelos testes de handlers; o OpenAPI descreve as 47 operações registradas. |
| Segurança   | Testes de middleware, token, dados sensíveis, `gosec` e `govulncheck`.                                                     |
| Cobertura    | Gate de 80% por pacote crítico executado com banco real na CI.                                                                |

Resultados registrados no relatório do projeto:

- `internal/features/service-order`: pelo menos 80%.
- `internal/features/product`: acima de 80%.
- `internal/features/service-order-tracking`: acima de 80%.
- `gosec`: zero achados após correções.
- `govulncheck`: nenhuma vulnerabilidade alcançável com a linha Go 1.25 registrada.

# 13. Backlog revisado

| ID   | User story                                | Estado                                                                   |
| ---- | ----------------------------------------- | ------------------------------------------------------------------------ |
| US01 | Autenticação administrativa             | Concluída                                                               |
| US02 | Gestão de clientes                       | Concluída; rotas protegidas por JWT                                    |
| US03 | Gestão de veículos                      | Concluída                                                               |
| US04 | Catálogo de serviços                    | Concluída                                                               |
| US05 | Catálogo de peças e insumos             | Concluída                                                               |
| US06 | Ajuste e consulta de estoque              | Concluída                                                               |
| US07 | Criação de OS                           | Concluída; rota protegida por JWT                                      |
| US08 | Diagnóstico e composição de orçamento | Concluída                                                               |
| US09 | Envio e decisão de orçamento            | Concluída no domínio; notificação externa pendente                   |
| US10 | Execução e ciclo de status              | Concluída                                                               |
| US11 | Baixa e estorno de itens na OS            | Concluída; contrato OpenAPI atualizado                                  |
| US12 | Consulta administrativa de OS             | Concluída                                                               |
| US13 | Acompanhamento pelo cliente               | Concluída                                                               |
| US14 | Métrica de tempo de serviço             | Concluída                                                               |
| US15 | Documentação e ambiente local           | Concluída no repositório; metadados externos da entrega ainda pendentes |
| US16 | Qualidade e segurança verificáveis      | Concluída                                                               |

## 13.1 Pendências técnicas e documentais priorizadas

### P1 — Implementar notificação real

Substituir `NoOpQuoteNotifier` por uma integração configurável. O código registra o envio, mas não entrega mensagem por e-mail ou outro canal.

### P2 — Unificar o envelope HTTP compartilhado

Avaliar a consolidação de `internal/shared/httpx` e `internal/shared/apierror` para reduzir duplicação e garantir uma única convenção de erros.

### P3 — Concluir metadados da entrega

Preencher os RAs ainda ausentes, usernames do Discord, links definitivos, acesso ao repositório, vídeo e PDF final.

### P4 — Fechar as lacunas entre catálogo de serviços e orçamento

- Filtrar `services.active = TRUE` ao validar serviços solicitados, itens de orçamento e início de execução, caso essa seja a regra desejada.
- Restringir a recomposição ao estado `IN_DIAGNOSIS` ou definir um fluxo explícito de reenvio quando a cotação já estiver em `AWAITING_APPROVAL`.
- Atualizar testes, especificações e OpenAPI após a decisão.

## 13.2 Pendências resolvidas recentemente

- **Paridade OpenAPI:** `/health` e as três operações de estoque vinculadas à OS foram adicionadas; o contrato agora contém 47 operações para as 47 rotas registradas.
- **Política de autenticação:** as rotas de clientes e `POST /api/v1/service-orders` passaram a exigir JWT em 26/08/2026.
- **Linguagem do domínio:** os status da OS foram renomeados para inglês em 26/08/2026 e as rotas de produtos/estoque em 27/08/2026.

# 14. Execução local

## 14.1 Serviços

Com um `.env` criado a partir de `.env.example`, o Docker Compose inicia:

- API em `http://localhost:8080`;
- PostgreSQL 16 em `localhost:5432`;
- Adminer em `http://localhost:8081`;
- Swagger UI em `http://localhost:8082`.

## 14.2 Variáveis principais

- `DATABASE_URL`: obrigatória para a API.
- `JWT_SECRET`: obrigatório e com pelo menos 32 bytes; a API falha imediatamente quando ele não é informado ou é curto demais.
- `JWT_TTL`: opcional, padrão de uma hora no Compose.
- `PORT`: porta da API quando executada fora do Compose.

## 14.3 Comandos

```bash
cp .env.example .env
docker compose up -d
```

Para popular os dados de desenvolvimento:

Em um volume novo, `docs/schema.sql` e `docs/seed.sql` são executados automaticamente pelo PostgreSQL. Para reaplicar o seed idempotente em um volume já existente:

```bash
docker compose cp docs/seed.sql db:/tmp/seed.sql
docker compose exec db psql -U workshop -d automotive_workshop -f /tmp/seed.sql
```

Para reaplicar alterações de `docs/schema.sql` no ambiente local, é necessário recriar o volume:

```bash
docker compose down -v
docker compose up -d
```

Esse comando remove os dados do volume PostgreSQL local. Não há ferramenta de migração incremental configurada no projeto.

Para executar as verificações locais:

```bash
go build ./...
go vet ./...
go test ./...
```

Com PostgreSQL disponível, a cobertura crítica pode ser validada com:

```bash
scripts/coverage.sh
```

E as análises de segurança com:

```bash
scripts/security-scan.sh
```

# 15. Checklist de entrega da Fase 1

| Item                                              | Estado verificável no repositório                    |
| ------------------------------------------------- | ------------------------------------------------------ |
| Board Miro com os artefatos DDD                   | Link informado; conteúdo deve ser validado pelo grupo |
| Linguagem ubíqua e domínio revisados            | Atualizados neste documento                            |
| Repositório privado com acesso ao avaliador      | Validação externa pendente                           |
| APIs REST implementadas                           | Concluído                                             |
| APIs REST documentadas                            | Concluído: 47 de 47 rotas no OpenAPI                   |
| Dockerfile e Docker Compose                       | Concluído                                             |
| README com instruções                           | Concluído                                             |
| Testes automatizados                              | Concluído                                             |
| Evidência de cobertura crítica                  | Concluído no relatório e na CI                       |
| Relatório de vulnerabilidades                    | Concluído                                             |
| Coleções para teste manual                      | Postman, Insomnia e Bruno disponíveis                 |
| Vídeo de até 15 minutos                         | Validação externa pendente                           |
| PDF final com equipe, Discord, links e relatório | Validação externa pendente                           |

# 16. Decisões consolidadas e pendentes

## 16.1 Consolidadas

- Monólito modular com Vertical Slice Architecture.
- Go 1.25, `net/http`, PostgreSQL 16 e `pgx/v5` sem ORM.
- Identificadores e status do domínio em inglês; somente as rotas públicas de acompanhamento permanecem em português.
- Reprovação leva a cotação para `REJECTED` e a OS para `CANCELED`.
- Acompanhamento do cliente com token específico da OS.
- Serviços solicitados na abertura separados dos itens precificados do orçamento.
- Ledger único de movimentações para ajustes manuais e uso em OS.
- `gosec` para SAST e `govulncheck` para vulnerabilidades alcançáveis.

## 16.2 Pendentes

- Escolher e implementar o canal real de notificação; e-mail é a intenção registrada, não uma integração existente.
- Decidir a política para serviços inativos e para recomposição de orçamento após o envio.
- Decidir se e quando unificar `httpx` e `apierror`.
- Preencher dados acadêmicos e links externos que não podem ser inferidos do código.
