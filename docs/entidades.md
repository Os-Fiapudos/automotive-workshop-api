# Entidades

## Cliente

| Campo     | Tipo   | Descrição                                                     |
| --------- | ------ | -------------------------------------------------------------- |
| id        | uuid   | Identificador técnico seguro, gerado pelo sistema.             |
| codigo    | number | Identificador legível/sequencial exibido ao usuário (ex: em telas, recibos, buscas). |
| nome      | string | Nome completo ou razão social do cliente.                      |
| documento | string | CPF ou CNPJ do cliente, usado para identificação fiscal.       |
| telefone  | string | Telefone de contato principal.                                 |
| email     | string | E-mail de contato, usado para notificações e comunicação.      |
| createdAt | string | Data/hora de criação do registro, gerada automaticamente.      |
| updatedAt | string | Data/hora da última atualização do registro, gerada automaticamente. |

## Veiculo

| Campo     | Tipo   | Descrição                                                       |
| --------- | ------ | ------------------------------------------------------------------ |
| id        | uuid   | Identificador técnico seguro, gerado pelo sistema.                 |
| codigo    | number | Identificador legível/sequencial exibido ao usuário.                |
| placa     | string | Placa de identificação do veículo.                                  |
| marca     | string | Fabricante do veículo (ex: Fiat, Volkswagen).                       |
| modelo    | string | Modelo do veículo (ex: Uno, Gol).                                   |
| ano       | number | Ano de fabricação/modelo do veículo.                                |
| cor       | string | Cor predominante do veículo.                                       |
| clienteId | uuid   | Referência ao `Cliente` proprietário do veículo.                    |
| createdAt | string | Data/hora de criação do registro, gerada automaticamente.           |
| updatedAt | string | Data/hora da última atualização do registro, gerada automaticamente. |

## Produto

| Campo         | Tipo   | Descrição                                                              |
| ------------- | ------ | -------------------------------------------------------------------------- |
| id            | uuid   | Identificador técnico seguro, gerado pelo sistema.                         |
| codigo        | number | Identificador legível/sequencial exibido ao usuário.                        |
| nome          | string | Nome do produto.                                                            |
| descricao     | string | Descrição detalhada do produto.                                            |
| valorUnitario | number | Preço de venda por unidade do produto.                                     |
| estoqueAtual  | number | Quantidade disponível em estoque no momento da consulta.                   |
| tipo          | string | Categoria do produto: `PECA` (peça de reposição) ou `INSUMO` (material de consumo). |
| createdAt     | string | Data/hora de criação do registro, gerada automaticamente.                  |
| updatedAt     | string | Data/hora da última atualização do registro, gerada automaticamente.       |

## Servico

| Campo         | Tipo    | Descrição                                                          |
| ------------- | ------- | ---------------------------------------------------------------------- |
| id            | uuid    | Identificador técnico seguro, gerado pelo sistema.                     |
| codigo        | number  | Identificador legível/sequencial exibido ao usuário.                    |
| nome          | string  | Nome do serviço oferecido (ex: troca de óleo, alinhamento).            |
| descricao     | string  | Descrição detalhada do que o serviço compreende.                       |
| valor         | number  | Preço cobrado pela execução do serviço.                                |
| tempoEstimado | number? | Tempo estimado de execução, em minutos. Campo opcional.                |
| createdAt     | string  | Data/hora de criação do registro, gerada automaticamente.              |
| updatedAt     | string  | Data/hora da última atualização do registro, gerada automaticamente.   |

## OrdemDeServico

| Campo       | Tipo      | Descrição                                                                                              |
| ----------- | --------- | ---------------------------------------------------------------------------------------------------------- |
| id          | uuid      | Identificador técnico seguro, gerado pelo sistema.                                                          |
| codigo      | number    | Identificador legível/sequencial da OS, usado para consulta e comunicação com o cliente.                     |
| clienteId   | uuid      | Referência ao `Cliente` solicitante do serviço.                                                             |
| veiculoId   | uuid      | Referência ao `Veiculo` que está sendo atendido.                                                            |
| dataCriacao | string    | Data/hora de abertura da ordem de serviço.                                                                  |
| status      | string    | Situação atual da OS: `RECEBIDA` (veículo recebido) → `EM_DIAGNOSTICO` (em avaliação) → `AGUARDANDO_APROVACAO` (orçamento enviado ao cliente) → `EM_EXECUCAO` (serviço em andamento) → `FINALIZADA` (serviço concluído) → `ENTREGUE` (veículo devolvido ao cliente). |
| orcamento   | Orcamento | Orçamento vinculado a esta ordem de serviço.                                                                |
| observacoes | string    | Anotações livres sobre o atendimento (ex: relato do cliente, condições do veículo).                        |
| createdAt   | string    | Data/hora de criação do registro, gerada automaticamente.                                                   |
| updatedAt   | string    | Data/hora da última atualização do registro, gerada automaticamente.                                        |

## Orcamento

| Campo          | Tipo       | Descrição                                                                       |
| -------------- | ---------- | ------------------------------------------------------------------------------------ |
| id             | uuid       | Identificador técnico seguro, gerado pelo sistema.                                   |
| codigo         | number     | Identificador legível/sequencial do orçamento.                                        |
| ordemServicoId | uuid       | Referência à `OrdemDeServico` à qual este orçamento pertence.                        |
| valor          | number     | Valor total do orçamento (soma de produtos e serviços).                              |
| status         | string     | Situação do orçamento: `PENDENTE` (aguardando resposta do cliente), `APROVADO` (cliente aceitou) ou `REPROVADO` (cliente recusou). |
| dataGeracao    | string     | Data/hora em que o orçamento foi gerado e enviado ao cliente.                        |
| dataResposta   | string?    | Data/hora em que o cliente respondeu (aprovou/reprovou). Campo opcional, preenchido só após resposta. |
| products       | Produto[]  | Lista de produtos/peças incluídos no orçamento.                                      |
| services       | Servico[]  | Lista de serviços incluídos no orçamento.                                            |
| createdAt      | string     | Data/hora de criação do registro, gerada automaticamente.                            |
| updatedAt      | string     | Data/hora da última atualização do registro, gerada automaticamente.                 |

## HistoricoOrdemServico

Registra a trilha de eventos e mudanças de status de uma ordem de serviço, para fins de auditoria e rastreabilidade.

| Campo          | Tipo   | Descrição                                                                    |
| -------------- | ------ | ---------------------------------------------------------------------------------- |
| id             | uuid   | Identificador técnico do registro de histórico.                                    |
| ordemServicoId | uuid   | Referência à `OrdemDeServico` à qual este evento pertence.                         |
| dataHora       | string | Data/hora em que o evento ocorreu.                                                 |
| evento         | string | Tipo de evento registrado: `criacao`, `aprovacao`, `finalizacao` ou `cancelamento`. |
| descricao      | string | Detalhamento do que ocorreu no evento.                                             |
| statusOld      | string | Status da OS imediatamente antes do evento.                                        |
| statusNew      | string | Status da OS imediatamente após o evento.                                          |

## AuditServices

Registra o início e o fim da execução de cada serviço dentro de uma ordem de serviço, para controle de tempo e produtividade.

| Campo          | Tipo   | Descrição                                                        |
| -------------- | ------ | --------------------------------------------------------------------- |
| id             | uuid   | Identificador técnico do registro de auditoria.                       |
| ordemServicoId | uuid   | Referência à `OrdemDeServico` em execução.                            |
| serviceId      | uuid   | Referência ao `Servico` sendo executado.                              |
| dataHora       | string | Data/hora em que o evento foi registrado.                             |
| evento         | string | Marco do evento: `inicio` (início da execução) ou `fim` (conclusão).  |

## Enums

### Enum (status)

Agrupa os valores possíveis para os campos de status usados nas entidades acima.

| Campo                    | Tipo   | Descrição                                                        |
| ------------------------ | ------ | ----------------------------------------------------------------- |
| statusOrdemServico       | string | Valores possíveis para `OrdemDeServico.status` (ver seção acima). |
| statusAprovacaoOrcamento | string | Valores possíveis para `Orcamento.status` (ver seção acima).      |

### ProdutoTipo

| Campo     | Tipo   | Descrição                                          |
| --------- | ------ | --------------------------------------------------- |
| produType | string | Valores possíveis para `Produto.tipo`: `PECA` ou `INSUMO`. |
