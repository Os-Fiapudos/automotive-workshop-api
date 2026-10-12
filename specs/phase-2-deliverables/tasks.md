# Tasks — Fase 2 Deliverables

Ordered last in the overall Fase 2 plan — most items here depend on other specs' own
implementation being far enough along to describe or demo truthfully.

## 1. GitHub access (`design.md` §5) — can be done anytime, no dependency

- [ ] Grant `soat-architecture` collaborator access to the GitHub repository.

## 2. Collection currency (`design.md` §2)

- [ ] Confirm `docs/openapi.yaml` includes every endpoint listed in `design.md` §2's table
      (each already a task in its owning spec — this is a cross-check, not new work).
- [ ] Update Bruno/Postman/Insomnia collections to match.

## 3. README (`design.md` §1)

- [ ] Add the `## Fase 2` section and its subsections (`design.md` §1).
- [ ] Link each subsection to the owning spec's own README output
      (`specs/database-migrations/`, `specs/kubernetes-deployment/`,
      `specs/infrastructure-provisioning/`, `specs/ci-cd-pipeline/`) rather than duplicating
      their content.
- [ ] Link the updated collections (task 2).

## 4. Architecture diagrams (`design.md` §3)

- [ ] Diagram 1 (application components) — drafted once
      `specs/service-order-architecture-refactor/` (if implemented) has landed.
- [ ] Diagram 2 (infrastructure provisioned) — drafted once
      `specs/infrastructure-provisioning/` and `specs/kubernetes-deployment/` have landed.
- [ ] Diagram 3 (deploy flow) — drafted once `specs/ci-cd-pipeline/` has a working run to
      reference.
- [ ] Embed all three in the README's "Arquitetura proposta" subsection.

## 5. Video (`design.md` §4)

- [ ] Record following the four-beat outline, after every other Fase 2 spec has been
      deployed at least once end to end.
- [ ] Publish to YouTube or Vimeo (public or unlisted), confirm it is ≤15 minutes.
- [ ] Add the link to the README's "Vídeo demonstrativo" subsection.

## 6. PDF (`design.md` §5)

- [ ] Assemble: repository link (access already granted, task 1), finalized architecture
      diagram, video link.
- [ ] Upload to the student portal before the deadline.

## 7. Verification

- [ ] Re-check `requirements.md` AC1–AC6, in order, as the final sign-off before
      submission.
