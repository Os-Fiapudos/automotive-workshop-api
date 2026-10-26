# Infraestrutura com Terraform (`/infra`)

Provisiona o ambiente Kubernetes local da Fase 2 (FP-35): **cluster `kind`**,
**metrics-server** e **PostgreSQL 16**. A API e os Jobs de migração/bootstrap são aplicados
depois, a partir de [`/k8s`](../k8s/README.md) (FP-34), pelo fluxo de deploy (FP-36,
`scripts/deploy.sh`) ou pelos comandos manuais do README de `/k8s`.

Especificação: [`specs/infrastructure-provisioning/`](../specs/infrastructure-provisioning/).

## Recursos criados

| Recurso | Nome | Detalhe |
| --- | --- | --- |
| Cluster `kind` | `automotive-workshop` | 1 nó control-plane, `kindest/node:v1.37.0` fixado por digest; kubeconfig em `infra/.kube/automotive-workshop.kubeconfig` |
| Helm release | `metrics-server` (ns `kube-system`) | chart `3.13.1` (app `v0.8.1`), `--kubelet-insecure-tls` (certificado autoassinado do kubelet no kind) |
| Namespace | `automotive-workshop` | Pod Security `restricted` (enforce/audit/warn) |
| Secret | `postgres-credentials` | usuário/senha do Postgres, lido só pelo pod do banco |
| StatefulSet | `postgres` (pod `postgres-0`) | `postgres:16.15-alpine3.24` fixado por digest, não-root (UID 70), probes `pg_isready` via TCP, `max_connections=300`, requests 250m/256Mi, limits 1 CPU/512Mi |
| PersistentVolumeClaim | `data-postgres-0` | 1 Gi, `ReadWriteOnce`, StorageClass padrão do kind (`standard`), retido ao apagar o pod/StatefulSet |
| Service | `postgres` (headless) | `postgres.automotive-workshop.svc.cluster.local:5432` |

Terraform é o **único dono** desses recursos. `/k8s` não tem manifest de Namespace, e o
deploy nunca altera objetos do Terraform. Se algum deles já existir criado por outra
ferramenta, adote com `terraform import` em vez de supor que a criação será ignorada.

## Pré-requisitos

- Docker em execução (o kind cria o nó como um container).
- Terraform `>= 1.16` (CI usa 1.16.6) e `kubectl` (CI usa v1.36.5).
- Host com **cgroup v2**. O kubelet 1.35+ recusa cgroup v1. Em host com cgroup v1, use um
  nó anterior: `export TF_VAR_node_image='kindest/node:v1.34.3@sha256:08497ee19eace7b4b5348db5c6a1591d7752b164530a36f855cb0f2bdcbadd48'`.
- Capacidade sugerida: 4 CPUs e 6 GiB livres para o Docker. Com 8 réplicas, a API pede
  0,8 CPU / 1 GiB, mais Postgres, metrics-server e os pods do sistema.

## Aplicar

```bash
export TF_VAR_postgres_password="$(openssl rand -hex 32)"   # guarde: o banco fica com ela
terraform -chdir=infra init
terraform -chdir=infra plan
terraform -chdir=infra apply
```

Equivalentes no Makefile: `make infra-init`, `make infra-plan`, `make infra-apply`. Em vez da
variável de ambiente, dá para copiar `terraform.tfvars.example` para `terraform.tfvars`
(git-ignored) e preencher. Reaplicar usa **a mesma senha**: o Postgres guarda a senha
com que o volume foi inicializado.

O `apply` só termina quando o `postgres-0` passa na readiness (`pg_isready` via TCP) e o
metrics-server está pronto. Um segundo `terraform plan` logo depois deve dizer
`No changes`.

## Outputs e conexão

```bash
terraform -chdir=infra output
export KUBECONFIG="$(terraform -chdir=infra output -raw kubeconfig_path)"
kubectl get nodes
kubectl top nodes                        # métricas reais em ~1 min
kubectl -n automotive-workshop get statefulset,pods,pvc,svc
```

| Output | Uso |
| --- | --- |
| `kubeconfig_path` | caminho absoluto do kubeconfig deste cluster; nada depende do contexto atual da máquina |
| `postgres_database_url_template` | `DATABASE_URL` do Secret da API: troque o literal `<password>` pela senha *percent-encoded* |
| `postgres_host`, `postgres_port`, `postgres_database`, `postgres_user`, `namespace`, `cluster_name`, `kube_context` | informativos |

Nenhum output contém credencial. `sslmode=disable` é decisão documentada para este banco
interno ao cluster: sem certificado TLS, e o tráfego não sai do nó (design.md §7). Para um
banco fora do cluster, use `sslmode=require` ou mais estrito.

Acesso ao banco pela máquina local (Adminer, psql):

```bash
kubectl -n automotive-workshop port-forward svc/postgres 15432:5432
psql "postgres://workshop:$TF_VAR_postgres_password@127.0.0.1:15432/automotive_workshop?sslmode=disable"
```

## Próximo passo: a aplicação

Com a infraestrutura pronta, siga [`k8s/README.md`](../k8s/README.md) (Secrets → migração →
bootstrap do admin → rollout), ou use `make deploy` (FP-36, `scripts/deploy.sh up`), que
consome os outputs acima.

## Estado do Terraform (sensível)

- Backend **local**: `infra/terraform.tfstate` fica na máquina que rodou o `apply`.
- O state contém a senha do Postgres e a chave do cliente do cluster, apesar de a
  variável ser `sensitive`. `.gitignore` cobre `*.tfstate*`, `infra/.terraform/`,
  `infra/.kube/` e `terraform.tfvars`. Nunca commite, anexe ou compartilhe esses arquivos.
- `.terraform.lock.hcl` (checksums dos providers, gerado contra o registry público) é
  commitado. Ao subir a versão de um provider em `versions.tf`, regenere-o e commite junto:
  `terraform -chdir=infra providers lock -platform=linux_amd64 -platform=darwin_arm64 -platform=darwin_amd64 -platform=windows_amd64`.

## Persistência do banco

```bash
make k8s-db-persistence     # scripts/k8s-evidence.sh db-persistence
```

Grava um marcador num banco separado (`fp35_persistence_check`, nunca o banco da
aplicação), apaga o `postgres-0`, espera o StatefulSet recriá-lo e lê o marcador de volta.
A evidência vai para `evidence/` (git-ignored). Causa uma indisponibilidade curta do banco:
não rode durante a gravação de outros fluxos.

## Destruir

```bash
make infra-destroy INFRA_CONFIRM_DESTROY=yes    # = terraform -chdir=infra destroy
kind get clusters; docker ps -a                 # nada do automotive-workshop deve restar
```

Destroi o cluster **e os dados** (o volume vive dentro do nó kind). Use só em ambiente
descartável. Nunca use como passo de deploy de um ambiente que precisa continuar de pé,
como o da demonstração em vídeo.

## Recuperação

- **Cluster apagado fora do Terraform** (`kind delete cluster`, reset do Docker): o próximo
  `plan` falha ao ler os recursos Kubernetes. Remova o state que ficou órfão e aplique de
  novo:
  ```bash
  terraform -chdir=infra state list
  terraform -chdir=infra state rm $(terraform -chdir=infra state list)
  terraform -chdir=infra apply
  ```
- **Fallback em duas etapas**, se um provider reclamar da configuração antes de o cluster
  existir: `terraform -chdir=infra apply -target=kind_cluster.this` e depois
  `terraform -chdir=infra apply`.
- **Postgres não fica pronto**: `kubectl -n automotive-workshop logs postgres-0` e
  `kubectl -n automotive-workshop describe pod postgres-0`.

## Por que uma raiz e um state só

Os providers `kubernetes`/`helm` são configurados a partir dos atributos do `kind_cluster`.
No primeiro `apply` esses valores ainda não existem. Mesmo assim, os recursos usados
(`kubernetes_*_v1`, `helm_release`) planejam a criação sem chamar a API do cluster, e o
Terraform cria o cluster primeiro. `kubernetes_manifest` não é usado, porque exige a API
no plan e forçaria dois states (a validação da CI rejeita esse recurso). O pipeline do
FP-36 roda exatamente um `terraform apply` neste diretório.

## Limites do ambiente

- **Um nó** e **uma réplica de Postgres**: não é alta disponibilidade. O HPA (FP-34)
  aumenta pods, não nós, e não escala o banco.
- Os dados sobrevivem à recriação do pod do banco, não à destruição do cluster.
- Ambiente local: nada fica acessível fora da máquina. A API é acessada por
  `kubectl port-forward`.
- Trocar para uma cloud mudaria `cluster.tf`, os providers e, provavelmente, o Postgres
  para um banco gerenciado. Seria outra revisão da spec (requirements.md §6).
