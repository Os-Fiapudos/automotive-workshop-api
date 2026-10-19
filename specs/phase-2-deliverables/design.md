# Design — Fase 2 Deliverables

Satisfies: `requirements.md` (all sections). This feature is mostly sequencing/process, not
code — the "design" here is the checklist structure and the order deliverables must be
assembled in so each one is accurate when produced.

## 1. README structure

New top-level section, e.g. `## Fase 2` (placed after the existing Fase 1 content per
`requirements.md` BR1 — additive, not a replacement), with subsections:

```
## Fase 2
### Objetivo e escopo desta fase
### Arquitetura proposta
  - Componentes da aplicação (diagram — see §3)
  - Infraestrutura provisionada (diagram — see §3)
  - Fluxo de deploy (diagram — see §3)
### Execução local
  (links to the existing Fase 1 "how to run" section plus whatever's new, e.g. the
  migration workflow from specs/database-migrations/)
### Deploy em Kubernetes
  (links to specs/kubernetes-deployment/'s own README task output, not duplicated here)
### Provisionamento com Terraform
  (links to specs/infrastructure-provisioning/'s own README task output)
### Collections da API
  (links to Bruno/Postman/Insomnia/OpenAPI, confirmed current per §2)
### Vídeo demonstrativo
  (the published link, added last, §4)
```

Each sub-section with its own README task elsewhere in this plan is a short pointer here
plus whatever that spec's own task didn't already write — avoids this feature duplicating
content another spec's `tasks.md` already produces.

## 2. Collection currency check

Before linking any collection from the README, walk every Fase 2 endpoint added by this
plan's other specs against each collection format the README will link:

| Endpoint | Added by |
| --- | --- |
| `GET /api/v1/service-orders` (new default order/exclusion) | `specs/service-order-query/` |
| `POST /api/v1/service-orders` (requested products) | `specs/service-order-opening/` |
| `POST /api/v1/service-orders/{id}/quote/decision` | `specs/service-order-quote-decision/` |
| `POST /api/v1/service-orders/{id}/status` | `specs/service-order-status-update/` |

`docs/openapi.yaml` is the source of truth each of those specs' own `tasks.md` already
updates (each has its own "Documentation" task) — this feature's job is to confirm
Bruno/Postman/Insomnia (hand-maintained collections, `docs/`/`bruno/`) are regenerated or
hand-updated to match, since those do not auto-derive from `docs/openapi.yaml` in this
project today.

## 3. Architecture diagrams

Three diagrams (README §1's "Arquitetura proposta" subsection), each a simple
component/container-level diagram (Mermaid in the README itself is sufficient — this
project's README is Markdown rendered on GitHub, which supports Mermaid natively, avoiding a
dependency on an external diagramming tool for a diagram that must stay easy to keep in
sync):

1. **Application components** — the `service-order` (and other features')
   domain/use-case/adapter layers (if `specs/service-order-architecture-refactor/` has
   landed) or the flat feature packages (if not yet), the shared infra
   (`internal/shared/*`), and how `cmd/api/main.go` wires them.
2. **Infrastructure provisioned** — the Kubernetes cluster, the API `Deployment`+`Service`+
   `HPA`, the Postgres `StatefulSet`, `metrics-server`, and (if cloud, `requirements.md` §7
   dependent) the cloud provider's own managed pieces.
3. **Deploy flow** — push → CI build/test → image build/push to GHCR → Terraform
   apply → migration Job → `kubectl apply` → smoke test (`specs/ci-cd-pipeline/`'s own
   pipeline, visualized).

Each diagram is drafted only once its subject is actually implemented (`requirements.md`
BR5) — drafting diagram 1 before the architecture refactor lands, for instance, risks
describing a layout that doesn't exist yet; `tasks.md` orders this explicitly last among
this plan's specs.

## 4. Video script outline

A ~12–15 minute structure covering the PDF's four required beats (`requirements.md` BR3), in
an order that matches how a viewer would naturally follow the system:

1. (≈2 min) Brief architecture walkthrough using diagram 1/2 from §3.
2. (≈3 min) CI/CD: trigger (or show a recent) `workflow_dispatch` run
   (`specs/ci-cd-pipeline/`), narrating each stage as it completes — this doubles as the
   "Deploy da aplicação" and "Execução do CI/CD" beats, since the pipeline *is* the deploy.
3. (≈5 min) API consumption: open a service order with a vehicle/customer/services/products
   via the published collection, walk it through diagnosis → quote → the new external
   quote-decision endpoint → execution → finalize → deliver, and show the new listing
   ordering (`specs/service-order-query/`).
4. (≈3 min) Scalability: generate load against the deployed Service (a simple loop of
   `curl`/`hey`) while `kubectl get hpa -w` is visible, showing replica count increase and
   settle back down (`specs/kubernetes-deployment/`'s own verification task, re-performed on
   camera).

Recorded after every other Fase 2 spec in this plan is implemented and deployed at least
once end to end — this is deliberately the last `tasks.md` item across the whole plan.

## 5. GitHub access and PDF assembly

- Grant `soat-architecture` **repository collaborator** access via GitHub's own
  Settings → Collaborators flow (or the `gh` CLI: `gh api
  repos/<org>/<repo>/collaborators/soat-architecture -X PUT`) — a one-time administrative
  action, not a code change, and not satisfied by the application-level seed user of the
  same name (`requirements.md` BR4 — flagged explicitly here since the name collision is an
  easy, unforced error to make).
- Assemble the PDF last: repository link (with access already granted, verified by having
  someone outside the current collaborator list — or the `soat-architecture` account itself,
  if reachable — confirm they can open it), the finalized architecture diagram (§3,
  whichever version the README ends up with, exported to an image or referenced as a link if
  the submission format allows), and the published video link.

## 6. Traceability

| Requirement | Design section |
| --- | --- |
| BR1 (README Fase 2 section) | §1 |
| BR2 (collections current) | §2 |
| BR3 (video content/length/publishing) | §4 |
| BR4 (PDF contents, GitHub access distinct from seed user) | §5 |
| BR5 (diagrams match reality) | §3 |
| AC1–AC6 | §1–§5 |
