# Ferramenta de atualização de status da OS — n8n (FP-30 / F2-US05)

Integração demonstrável de `POST /api/v1/service-orders/{id}/status`
([spec](../../specs/service-order-status-update/)). O operador da oficina escolhe a OS, a
próxima etapa e uma observação num formulário do **n8n**; o workflow chama a API com o
segredo `X-External-Status-Secret` e mostra a resposta (sucesso ou rejeição) na tela.

```
Formulário n8n ──► HTTP Request (X-External-Status-Secret) ──► API ──► StartDiagnosis / FinalizeOrder / DeliverOrder
      ▲                                                                       │
      └──────────── tela final com o status HTTP e o corpo da resposta ◄──────┘
```

## Configuração (reproduzível)

| Item | Onde |
| --- | --- |
| Workflow versionado | [`workflows/service-order-status-update.json`](workflows/service-order-status-update.json) |
| Serviço `n8n` (imagem `n8nio/n8n:2.43.8`) | [`docker-compose.yml`](../../docker-compose.yml) |
| Segredo | `EXTERNAL_STATUS_UPDATE_SECRET` no `.env` (diferente de `EXTERNAL_NOTIFICATION_SECRET`) |

A cada start o container executa `n8n import:workflow` + `n8n publish:workflow`, então o
formulário fica publicado sem nenhum clique no editor. O workflow lê a URL da API
(`WORKSHOP_API_URL=http://api:8080`) e o segredo via `$env` — o segredo **nunca** fica no
JSON versionado.

```bash
cp .env.example .env            # ou acrescente EXTERNAL_STATUS_UPDATE_SECRET e N8N_PORT ao .env existente
docker compose up -d --build    # db, api, n8n, adminer, swagger-ui
```

- Formulário do operador: <http://localhost:5678/form/service-order-status>
- Editor do n8n (opcional, para mostrar o workflow no vídeo): <http://localhost:5678> — no
  primeiro acesso o n8n pede para criar a conta de owner local.

> Ao alterar o `path` do formulário no JSON, recrie o volume do n8n
> (`docker compose rm -sf n8n && docker volume rm <projeto>_n8n_data && docker compose up -d n8n`)
> — o registro antigo do webhook fica persistido no volume.

## Roteiro da demonstração (vídeo)

Pré-requisito: token administrativo e uma OS recém-aberta (status `RECEIVED`).

```bash
API=http://localhost:8080
TOKEN=$(curl -s $API/api/v1/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"admin@workshop.local","password":"admin123"}' | jq -r .access_token)
ORDER=$(curl -s $API/api/v1/service-orders -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"customerId":"a0000000-0000-0000-0000-000000000001","vehicleId":"b0000000-0000-0000-0000-000000000001"}' | jq -r .id)
echo $ORDER
```

1. **Rejeição de transição ilegal** — no formulário: OS = `$ORDER`, status `DELIVERED`.
   Tela: `Update rejected (HTTP 409)` com `INVALID_STATUS_TRANSITION`. Nada muda na OS.
2. **Diagnóstico** — status `IN_DIAGNOSIS`, observação `Car on lift 2`.
   Tela: `Status updated: RECEIVED -> IN_DIAGNOSIS`.
3. **Aprovação não pode ser pulada** — o orçamento segue pelo seu próprio canal:
   ```bash
   SVC=<id de um serviço do catálogo>   # GET $API/api/v1/services
   curl -s -X PUT  $API/api/v1/service-orders/$ORDER/quote -H "Authorization: Bearer $TOKEN" \
     -H 'Content-Type: application/json' -d "{\"items\":[{\"serviceId\":\"$SVC\",\"quantity\":1}]}"
   curl -s -X POST $API/api/v1/service-orders/$ORDER/quote/send -H "Authorization: Bearer $TOKEN"
   ```
   No formulário, `COMPLETED` → `409 INVALID_STATUS_TRANSITION` (OS em `AWAITING_APPROVAL`).
   Aprove pelo canal externo de orçamento (FP-28):
   ```bash
   curl -s -X POST $API/api/v1/service-orders/$ORDER/quote/decision \
     -H "X-External-Notification-Secret: $EXTERNAL_NOTIFICATION_SECRET" \
     -H 'Content-Type: application/json' -d '{"decision":"approved"}'
   ```
4. **Execução pendente bloqueia a finalização** — inicie a execução sem finalizar:
   ```bash
   EX=$(curl -s -X POST $API/api/v1/service-orders/$ORDER/executions -H "Authorization: Bearer $TOKEN" \
     -H 'Content-Type: application/json' -d "{\"serviceId\":\"$SVC\"}" | jq -r .id)
   ```
   No formulário, `COMPLETED` → `409 EXECUTIONS_NOT_COMPLETED`. Depois finalize a execução:
   ```bash
   curl -s -X POST $API/api/v1/service-orders/$ORDER/executions/$EX/finish -H "Authorization: Bearer $TOKEN"
   ```
5. **Finalização** — `COMPLETED`, observação `All services done, road-tested` →
   `Status updated: IN_PROGRESS -> COMPLETED`.
6. **Entrega** — `DELIVERED`, observação `Picked up by owner` →
   `Status updated: COMPLETED -> DELIVERED`.
7. **Histórico resultante**:
   ```bash
   curl -s $API/api/v1/service-orders/$ORDER -H "Authorization: Bearer $TOKEN" | jq '.status, .history'
   ```
   Resultado obtido na validação local (2026-10-19):

   | event | description | previous → new |
   | --- | --- | --- |
   | creation | Service order opened. | RECEIVED → RECEIVED |
   | diagnosis_started | Diagnosis started. Notes: Car on lift 2 | RECEIVED → IN_DIAGNOSIS |
   | quote_composed | Quote composed. | IN_DIAGNOSIS → IN_DIAGNOSIS |
   | quote_sent | Quote sent to customer. | IN_DIAGNOSIS → AWAITING_APPROVAL |
   | approval | Customer approved the quote. | AWAITING_APPROVAL → IN_PROGRESS |
   | completion | Service order finalized. Notes: All services done, road-tested | IN_PROGRESS → COMPLETED |
   | delivery | Vehicle delivered to customer. Notes: Picked up by owner | COMPLETED → DELIVERED |

   As tentativas rejeitadas (passos 1, 3 e 4) não geraram nenhuma linha.
8. **Credencial e contrato** (direto na API, sem o n8n):
   ```bash
   # 401 — segredo errado, segredo do orçamento ou só o JWT
   curl -s -X POST $API/api/v1/service-orders/$ORDER/status -H 'X-External-Status-Secret: wrong' \
     -H 'Content-Type: application/json' -d '{"status":"IN_DIAGNOSIS"}'
   # 400 — status fora do contrato
   curl -s -X POST $API/api/v1/service-orders/$ORDER/status -H "X-External-Status-Secret: $EXTERNAL_STATUS_UPDATE_SECRET" \
     -H 'Content-Type: application/json' -d '{"status":"IN_PROGRESS"}'
   ```

## Request/response sanitizados

```http
POST /api/v1/service-orders/4ab04387-31c0-4c82-9bc0-5f0a126e1770/status
X-External-Status-Secret: ***
Content-Type: application/json

{"status":"COMPLETED","notes":"All services done, road-tested"}
```

```json
200 {"id":"4ab04387-31c0-4c82-9bc0-5f0a126e1770","code":6,"previousStatus":"IN_PROGRESS","status":"COMPLETED"}
409 {"error":{"code":"INVALID_STATUS_TRANSITION","message":"service order is not in a status that allows this operation"}}
409 {"error":{"code":"EXECUTIONS_NOT_COMPLETED","message":"service order has pending service executions and cannot be finalized"}}
401 {"error":{"code":"INVALID_EXTERNAL_SECRET","message":"external status secret is missing or invalid"}}
400 {"error":{"code":"VALIDATION_ERROR","message":"invalid status update","details":[{"field":"status","message":"status must be one of IN_DIAGNOSIS, COMPLETED, DELIVERED"}]}}
```
