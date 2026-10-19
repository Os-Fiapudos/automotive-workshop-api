# Requirements — Fase 2 Deliverables

Status: **Approved for implementation** (documentation/process feature — no code; depends on
every other Fase 2 feature being far enough along to describe/demo truthfully)
Touches: `README.md`, `docs/` (collection links), a recorded video, a submitted PDF, GitHub
repository access.

## 1. Context

Source: `SOAT - Fase 2 - Tech challenge.pdf` p.5–6 ("Entregáveis da fase 2") — this is the
one Fase 2 requirement area that is not a code change but is still graded and still has
concrete, checkable acceptance criteria, so it gets its own spec per this project's SDD
convention rather than being left as an informal checklist.

## 2. User story

> As the team submitting this Tech Challenge,
> I want the README, the API collection links, the demo video, and the submission PDF to
> each satisfy exactly what the PDF's "Entregáveis da fase 2" section lists,
> so that nothing required for grading is missing or left pointing at stale, Fase-1-only
> content.

## 3. Business rules

1. `README.md` gains a Fase 2 section covering: description of the Fase 2 solution/
   objectives, the proposed architecture (application components, provisioned
   infrastructure, deploy flow — diagrams), instructions for local execution, Kubernetes
   deploy, and Terraform provisioning, a link to the complete API collection
   (Postman/Swagger/equivalent), and a link to the demo video. This is additive to the
   existing Fase 1 content (`CLAUDE.md`/the current README structure), not a replacement of
   it.
2. The API collection (this project already has Bruno, Postman, Insomnia, and OpenAPI —
   `GAPS-FASE-2.md` §3.3) is updated to include every Fase 2 endpoint
   (`specs/service-order-query/`'s changed listing, `specs/service-order-opening/`'s
   requested products, `specs/service-order-quote-decision/`'s external decision endpoint,
   `specs/service-order-status-update/`'s external status endpoint) before the README links
   to it — a stale collection missing the very endpoints the video demonstrates would defeat
   its purpose.
3. The video is ≤15 minutes, published to YouTube or Vimeo (public or unlisted), and
   demonstrates: deploy of the application, execution of the CI/CD pipeline, consumption of
   the APIs, and automatic scalability (simulated load or multiple service orders) — the
   PDF's exact four required demo beats (p.5–6).
4. The submission PDF contains: the GitHub repository link, **shared with the GitHub user
   `soat-architecture`** (a repository-collaborator grant — distinct from, and not satisfied
   by, the pre-existing `soat-architecture@workshop.local` seeded *application* user in
   `docs/seed.sql`, `CLAUDE.md` §5 — that is a login for the running API, not GitHub
   repository access, and the two must not be conflated), the architecture diagram with the
   chosen resources, and the video link (≤15 min) — the PDF's exact "Entrega no portal do
   aluno" wording (p.6).
5. Every architecture diagram (README and/or PDF) reflects the actually-implemented Fase 2
   architecture at submission time — not an aspirational one that outpaced what was actually
   built, and not the Fase 1 diagram left unchanged (`GAPS-FASE-2.md` §4's Miro-vs-code
   findings apply equally here: a diagram that still shows Portuguese status names or omits
   Kubernetes/Terraform would be exactly the kind of stale-diagram problem already found
   once in this project).

## 4. Scope

In scope: README Fase 2 section and diagrams, collection-link updates, the video recording/
publishing itself, the submission PDF's content, granting `soat-architecture` GitHub
collaborator access.

Out of scope: any code change (covered by every other spec in this plan); the Miro board
itself (`GAPS-FASE-2.md` §4's recommended updates to it are a nice-to-have, not a graded
deliverable per the PDF's explicit list, which asks for diagrams in the README/PDF, not in
Miro specifically).

## 5. Acceptance criteria

1. `README.md`'s Fase 2 section exists and covers every item in BR1.
2. Every collection (Bruno/Postman/Insomnia/OpenAPI) a README link points to includes every
   Fase 2 endpoint added by the other specs in this plan, verified by a manual pass through
   each new endpoint in whichever tool(s) the README links to.
3. A video exists, is ≤15 minutes, is published (public or unlisted) on YouTube or Vimeo,
   and its content checklist (BR3's four beats) is confirmed by watching it once, end to
   end, before submission.
4. `soat-architecture` has been granted collaborator access to the GitHub repository
   (verified via the repository's collaborators/settings page, not assumed from the seed
   data).
5. The submission PDF contains the repository link, the architecture diagram, and the video
   link, and is uploaded to the student portal before the deadline.
6. The architecture diagram used in the README/PDF matches the actually-implemented system
   (BR5) — spot-checked against `specs/architecture.md` and this plan's other specs at the
   time the diagram is finalized.

## 6. Out of scope

- Any requirement already covered by another spec's own documentation task (e.g.
  `specs/database-migrations/`'s own README subsection, `specs/kubernetes-deployment/`'s own
  "Deploy em Kubernetes" section, `specs/infrastructure-provisioning/`'s own "Terraform"
  section) — this spec's README task is to make sure those sections exist and are linked
  from a coherent Fase 2 overview, not to rewrite content those specs already own.
