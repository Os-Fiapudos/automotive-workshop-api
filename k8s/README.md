# Manifests Kubernetes (`/k8s`)

Deploy da API no cluster provisionado por [`/infra`](../infra/README.md) (FP-35), com
réplicas e autoscaling por CPU/memória (FP-34). Especificação:
[`specs/kubernetes-deployment/`](../specs/kubernetes-deployment/).

| Arquivo | Objeto | Papel |
| --- | --- | --- |
| `configmap.yaml` | ConfigMap `automotive-workshop-api-config` | configuração não sensível (`PORT`, `JWT_TTL`) |
| `deployment.yaml` | Deployment `automotive-workshop-api` | API stateless, probes em `/health`, requests 100m/128Mi, limits 500m/256Mi, perfil `restricted`, **sem `replicas`** (o HPA é o dono) |
| `service.yaml` | Service `automotive-workshop-api` | `ClusterIP` 8080, distribui entre os pods prontos |
| `hpa.yaml` | HorizontalPodAutoscaler | 2–8 réplicas, CPU 70 % / memória 80 %, scale-down estabilizado em 120 s |
| `migration-job.yaml` | Job `automotive-workshop-api-migrate-<id>` | `migrate/migrate:v4.20.1 up`, migrations de um ConfigMap gerado de `migrations/`, `backoffLimit: 0` |
| `bootstrap-admin-job.yaml` | Job `automotive-workshop-api-bootstrap-admin-<id>` | `/bootstrap-admin` com a imagem da API: cria o primeiro administrador (idempotente) |
| `templates/secrets.example.yaml` | — | **referência das chaves, nunca aplicar** |
| `load-test/load-generator-job.yaml` | Job de carga | evidência do HPA, nunca parte do deploy |

O Namespace, o Postgres e o metrics-server são do Terraform. Os Secrets e o ConfigMap de
migrations são criados no deploy, nunca commitados com valores.

## Regras

1. **Nunca `kubectl apply -f k8s/`.** A ordem importa, e os Jobs precisam de nome único
   por execução. O template de Secret fica em `templates/` justamente para que um
   `apply -f k8s/` (não recursivo) não sobrescreva credenciais reais com `REPLACE_ME`.
2. **Imagem imutável.** Os arquivos nomeiam `ghcr.io/os-fiapudos/automotive-workshop-api`
   sem tag; o deploy injeta `:<sha>@sha256:<digest>` de uma imagem publicada pelo pipeline.
   Aplicado cru, o Deployment pediria `:latest`, que nunca é publicada: o pull falha em vez
   de rodar uma imagem desconhecida.
3. **Ordem:** infraestrutura → Secrets + ConfigMap de migrations → Job de migração
   `Complete` → Job de bootstrap `Complete` → ConfigMap/Deployment/Service/HPA → rollout.
   Se a migração falhar, nada da aplicação é aplicado.

## Caminho 1: `scripts/deploy.sh` (FP-36)

`make deploy` (`scripts/deploy.sh up`) executa todas as etapas acima, gera as credenciais
nos Secrets e coleta evidências; é o mesmo script que o job `deploy` da CI roda etapa por
etapa. Variáveis: seção "Ambiente de demonstração" do README raiz e
`specs/ci-cd-pipeline/design.md` §4 e §10.

## Caminho 2: comandos manuais

Pré-requisito: `terraform -chdir=infra apply` concluído (veja `infra/README.md`) e a
imagem publicada pelo pipeline.

```bash
export KUBECONFIG="$(terraform -chdir=infra output -raw kubeconfig_path)"
NS=automotive-workshop
IMAGE='ghcr.io/os-fiapudos/automotive-workshop-api:<sha-do-commit>@sha256:<digest>'  # de um run de publish
RUN=$(date -u +%Y%m%d%H%M%S)
```

**1. Secrets** (valores por arquivos temporários com `umask 077`, não por argumentos de
linha de comando; `--server-side` evita copiar os valores para a anotação
`last-applied-configuration`):

```bash
# Senha hexadecimal (openssl rand -hex) não precisa de encoding; outra senha deve ser percent-encoded.
DATABASE_URL="$(terraform -chdir=infra output -raw postgres_database_url_template | sed "s/<password>/$TF_VAR_postgres_password/")"
umask 077; api_env=$(mktemp); admin_env=$(mktemp)
cat >"$api_env" <<EOF
DATABASE_URL=$DATABASE_URL
JWT_SECRET=$(openssl rand -hex 32)
EXTERNAL_NOTIFICATION_SECRET=$(openssl rand -hex 32)
EXTERNAL_STATUS_UPDATE_SECRET=$(openssl rand -hex 32)
EOF
cat >"$admin_env" <<EOF
ADMIN_EMAIL=admin@workshop.local
ADMIN_PASSWORD=$(openssl rand -hex 16)
EOF
kubectl -n $NS create secret generic automotive-workshop-api-secrets --from-env-file="$api_env" \
  --dry-run=client -o yaml | kubectl apply --server-side --field-manager=manual-deploy -f -
kubectl -n $NS create secret generic automotive-workshop-admin-bootstrap --from-env-file="$admin_env" \
  --dry-run=client -o yaml | kubectl apply --server-side --field-manager=manual-deploy -f -
grep ADMIN_PASSWORD "$admin_env"   # anote para o login; depois:
rm -f "$api_env" "$admin_env"
```

Para o e-mail de orçamento (opcional), acrescente `RESEND_API_KEY` e `RESEND_FROM_EMAIL` ao
primeiro arquivo, sempre o par completo. Se o pacote da imagem no GHCR for privado, crie a
credencial de pull antes dos pods:

```bash
kubectl -n $NS create secret docker-registry ghcr-pull --docker-server=ghcr.io \
  --docker-username=<usuario> --docker-password=<token-read:packages>
kubectl -n $NS patch serviceaccount default -p '{"imagePullSecrets":[{"name":"ghcr-pull"}]}'
```

**2. Migrações** (o ConfigMap é gerado dos arquivos do commit em deploy, nunca de uma cópia
commitada):

```bash
kubectl -n $NS create configmap automotive-workshop-api-migrations --from-file=migrations/ \
  --dry-run=client -o yaml | kubectl apply --server-side --field-manager=manual-deploy -f -
sed "s/^  name: automotive-workshop-api-migrate$/  name: automotive-workshop-api-migrate-$RUN/" \
  k8s/migration-job.yaml | kubectl create -f -
kubectl -n $NS wait --for=condition=complete job/automotive-workshop-api-migrate-$RUN --timeout=300s
kubectl -n $NS logs job/automotive-workshop-api-migrate-$RUN
```

Se o Job ficar `Failed` (o `wait` só retorna no timeout), **pare aqui**: leia os logs e siga
"Quando uma migration falha" no README raiz. Não aplique a aplicação.

**3. Bootstrap do administrador:**

```bash
sed -e "s/^  name: automotive-workshop-api-bootstrap-admin$/  name: automotive-workshop-api-bootstrap-admin-$RUN/" \
    -e "s#image: ghcr.io/os-fiapudos/automotive-workshop-api\$#image: $IMAGE#" \
  k8s/bootstrap-admin-job.yaml | kubectl create -f -
kubectl -n $NS wait --for=condition=complete job/automotive-workshop-api-bootstrap-admin-$RUN --timeout=180s
```

**4. Aplicação:**

```bash
kubectl apply --server-side --field-manager=manual-deploy -f k8s/configmap.yaml
sed "s#image: ghcr.io/os-fiapudos/automotive-workshop-api\$#image: $IMAGE#" k8s/deployment.yaml |
  kubectl apply --server-side --field-manager=manual-deploy -f -
kubectl apply --server-side --field-manager=manual-deploy -f k8s/service.yaml -f k8s/hpa.yaml
kubectl -n $NS rollout status deployment/automotive-workshop-api --timeout=300s
kubectl -n $NS get deploy automotive-workshop-api -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
```

Os Jobs concluídos são removidos sozinhos após 24 h (`ttlSecondsAfterFinished`).

## Acesso à API

```bash
kubectl -n automotive-workshop port-forward svc/automotive-workshop-api 8080:8080
curl -i http://localhost:8080/health
curl -s -X POST http://localhost:8080/api/v1/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"admin@workshop.local","password":"<ADMIN_PASSWORD>"}'
```

A collection (Bruno/Postman/Insomnia) funciona apontando a `baseUrl` para
`http://localhost:8080`. O `port-forward` fixa um único pod: serve para consumir a API, não
para provar a distribuição de carga (veja abaixo).

## Autoscaling (HPA) e resiliência

```bash
kubectl -n automotive-workshop get hpa -w        # CPU/memória reais, réplicas 2–8
make k8s-hpa-load                                # scripts/k8s-evidence.sh hpa-load
make k8s-pod-recovery                            # scripts/k8s-evidence.sh pod-recovery
make k8s-status
```

- `hpa-load` cria o Job de `load-test/` com nome único. Por padrão são 4 workers × 240 s
  (`LOAD_WORKERS`, `LOAD_DURATION`), fazendo requisições a
  `automotive-workshop-api.automotive-workshop.svc.cluster.local:8080/health`, **pelo
  Service dentro do cluster**, para que o kube-proxy distribua entre todos os pods. O script
  registra réplicas, uso de CPU/memória do HPA e `kubectl top` a cada 15 s, durante a carga e
  por 300 s depois (`COOLDOWN_SECONDS`), e falha se o HPA não escalar. Resultado em
  `evidence/hpa-load-<data>/summary.md` e `timeline.tsv`. Se a máquina for muito rápida e
  não chegar a 70 % de CPU, aumente `LOAD_WORKERS`.
- O scale-up reage em 15–30 s. O scale-down espera 120 s de estabilização e reduz no
  máximo 50 % por minuto. A métrica de memória raramente dispara nesta API.
- `pod-recovery` apaga um pod pronto da API e mede o tempo até a reposição ficar pronta,
  conferindo que o Service nunca ficou sem endpoint.
- Reaplicar os manifests com o Deployment escalado **não** reduz as réplicas: o Deployment
  não declara `replicas`.

## Diagnóstico

| Sintoma | Verifique |
| --- | --- |
| `hpa` mostra `<unknown>` | `kubectl -n kube-system get pods -l app.kubernetes.io/name=metrics-server`; aguarde ~1 min após o apply |
| Pod `CreateContainerConfigError` | Secret ausente ou sem uma chave: `kubectl -n automotive-workshop describe pod <pod>` |
| `ErrImagePull` / `ImagePullBackOff` | referência da imagem ou credencial `ghcr-pull` (pacote privado) |
| Pod rejeitado com `violates PodSecurity "restricted"` | o manifest perdeu algum campo de `securityContext` |
| API em `CrashLoopBackOff` | `kubectl -n automotive-workshop logs deploy/automotive-workshop-api`; em geral `DATABASE_URL` ou segredos `EXTERNAL_*` iguais |
| Job de migração `Failed` | logs do Job; banco "dirty": siga o README raiz, seção de migrations |

## Limites

- A API não trata `SIGTERM` (graceful shutdown). O `preStop` de 5 s dá tempo para o Service
  parar de rotear antes do encerramento. Tratar o sinal no código fica como melhoria futura.
- Sem Ingress/TLS: o acesso local é por `port-forward` (fora do escopo do manual).
- O HPA escala pods. Não adiciona nós nem torna o Postgres altamente disponível (um nó, uma
  réplica do banco; veja `infra/README.md`).
