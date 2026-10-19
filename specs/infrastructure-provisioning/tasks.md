# Tasks — Infrastructure Provisioning (Terraform)

## 1. Spec confirmation

- [ ] Confirm local (`kind`) vs. cloud cluster (`requirements.md` §7) before writing
      `cluster.tf` for real — the rest of the configuration (§2 onward) does not depend on
      this decision.

## 2. Terraform configuration (`design.md` §1–§5)

- [ ] `infra/cluster.tf` — `kind_cluster` resource (or the chosen cloud provider's
      equivalent), `kubernetes`/`helm` provider blocks wired to its outputs.
- [ ] `infra/namespace.tf`.
- [ ] `infra/metrics-server.tf` — `helm_release`, `--kubelet-insecure-tls` only if local.
- [ ] `infra/postgres.tf` — `StatefulSet`, `PersistentVolumeClaim`, headless `Service`,
      credentials `Secret`.
- [ ] `infra/variables.tf` — `postgres_user`/`postgres_db`/`postgres_password` (sensitive).
- [ ] `infra/outputs.tf` — `postgres_host`/`postgres_port`/`postgres_database_url_template`.
- [ ] `infra/terraform.tfvars.example` — placeholder only; confirm `.gitignore` covers
      `terraform.tfvars` and `.terraform/`.

## 3. Verification (`design.md` §8)

- [ ] `terraform validate` / `terraform plan` clean.
- [ ] `terraform apply -var-file=terraform.tfvars` from a clean state — confirm cluster,
      metrics-server (`kubectl top nodes` reports real metrics within a few minutes),
      namespace, and Postgres are all reachable.
- [ ] `terraform destroy -var-file=terraform.tfvars` — confirm no orphaned `kind`
      containers/networks/volumes remain.

## 4. Documentation (`design.md` §7)

- [ ] `infra/README.md` (or the linked root-README section,
      `specs/phase-2-deliverables/`) — resource list, exact apply/destroy commands, and the
      "populate the K8s Secret from `terraform output`" note.

## 5. Handoff to `specs/kubernetes-deployment/`

- [ ] Confirm `terraform output`'s values are sufficient to populate that spec's `Secret`
      (`DATABASE_URL`) with no additional lookup needed.

## 6. Verification

- [ ] Re-check `requirements.md` AC1–AC5.
