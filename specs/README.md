# specs — Specification-Driven Development (SDD)

This folder is the **source of truth for the expected behavior** of the
`automotive-workshop-api`. No feature should be implemented without a corresponding
specification existing here first — that is the central SDD rule defined in
[CLAUDE.md](../CLAUDE.md).

If code and a specification diverge, the specification is the one considered correct until
it is formally updated. A specification should not be changed just to make the code "fit"
it — if the requirement really changed, the change is made in the specification first,
explicitly, and only then in the code.

## How specifications are organized

```
specs/
├── README.md            → this file: the project's SDD process
├── architecture.md       → current system architecture, kept in sync with the code
└── <feature>/            → one folder per business feature
    ├── requirements.md   → WHAT and WHY (requirements, acceptance criteria)
    ├── design.md          → HOW (technical design that satisfies the requirements)
    └── tasks.md            → checklist of implementation tasks derived from the design
```

- `<feature>` should use a short kebab-case name that identifies the business feature (e.g.
  `customer-registration`, `service-order-opening`, `quote-approval`), aligned with the
  domain language already used in [docs/entities.md](../docs/entities.md).
- Each `<feature>/` generally corresponds to a vertical slice in
  `internal/features/<feature>/` in the code (see [specs/architecture.md](architecture.md)
  and [CLAUDE.md](../CLAUDE.md)) — but the specification is written and approved **before**
  the code exists.
- No feature folder exists yet under `specs/` — this repository is ready to receive its
  first specification, but none has been written yet.

## The SDD flow used in this project

```
Requirement → Design → Tasks → Implementation → Tests → Review
```

Each stage only begins after the previous one is defined and (where applicable) approved.
Skipping stages or implementing ahead of them breaks the process.

### 1. Requirement (`requirements.md`)

- Defines **what** the feature must do and **why**, in business terms: user stories,
  business rules, acceptance criteria.
- Written **before** any technical design decision.
- Must not contain implementation details (table names, endpoints, Go structs).
- If there is ambiguity in the requirements, it must be clarified with whoever requested
  the feature **before** moving on to design — never resolved by assumption.
- Requirements not described here must not be invented during design or implementation.

### 2. Design (`design.md`)

- Defines **how** the requirements will be satisfied: components involved, data model,
  endpoints/contracts, flow across layers.
- Only written after the requirements in `requirements.md` are defined.
- Must follow the architectural patterns already established in the project (vertical
  slice, handler/service/repository/model layers within the feature — see
  [specs/architecture.md](architecture.md) and [CLAUDE.md](../CLAUDE.md)) instead of
  introducing a new pattern without justification.
- Every design decision must reference the requirement(s) it satisfies — design without an
  associated requirement should not exist.
- Before proposing the design, analyze the code and patterns already adopted in the project
  to reuse them, instead of reinventing a different approach to the same problem.

### 3. Tasks (`tasks.md`)

- Breaks the design down into an ordered, actionable list of implementation tasks.
- Each task should be small enough to be implemented and tested in isolation, and must
  reference the `design.md` section (and, transitively, the requirement) it implements.
- This is the artifact that guides implementation step by step — implementation must not
  "jump ahead" of what is described in the tasks.

### 4. Implementation

- Implements exactly what is described in `tasks.md`/`design.md`, following the code
  conventions already identified in [CLAUDE.md](../CLAUDE.md) (organization by feature,
  domain naming, database conventions, etc.).
- Does not include changes outside the scope of the feature in question.
- If, during implementation, it becomes clear that the design or requirements need to
  change, the change is made in the specification documents first (explicitly), not
  silently in the code alone.

### 5. Tests

- Every new feature needs tests that validate the acceptance criteria defined in
  `requirements.md`.
- Tests follow the organization described in [CLAUDE.md](../CLAUDE.md): unit tests
  alongside the feature (`internal/features/<feature>/*_test.go`), handler/integration
  tests in `internal/handlers_test/`.
- `go test ./...`, together with `go build ./...` and `go vet ./...`, must pass before the
  feature is considered ready.

### 6. Review

- After implementation, the feature is checked against `requirements.md`: were all
  acceptance criteria met? Were all tasks in `tasks.md` completed?
- Confirms that no change outside the specification's scope was introduced.
- Confirms that the specification was not silently changed to justify what was
  implemented — any divergence found is resolved by explicitly updating the specification
  (with justification) or fixing the code, not both in an ambiguous way.

## Relationship between requirements, design, and tasks

The three parts form a traceability chain that must stay intact from requirement to code:

```
requirement  (what / why)
    ↓
design       (how, referencing the requirement)
    ↓
tasks        (concrete steps, referencing the design)
    ↓
code + tests (implementation, referencing the tasks)
```

- `design.md` should not contain a technical decision that does not serve some item in
  `requirements.md`.
- `tasks.md` should not contain a task that does not come from some section of
  `design.md`.
- The implemented code should not contain behavior that does not come from some task —
  that is what prevents "inventing" unspecified functionality.

## How a feature evolves from requirement to implementation

1. Create the `specs/<feature>/` folder.
2. Write `requirements.md` — if something is not clear, ask before moving on.
3. Only then write `design.md`, aligned with the architecture in
   [specs/architecture.md](architecture.md) and reusing patterns already present in the
   code.
4. Break the design down into `tasks.md`.
5. Implement each task in `internal/features/<feature>/` (or in `internal/shared/`, when
   it is genuinely cross-cutting code), following [CLAUDE.md](../CLAUDE.md).
6. Write the tests that validate the feature's acceptance criteria.
7. Review the implementation against `requirements.md` and `tasks.md` before considering
   the feature complete, and update [specs/architecture.md](architecture.md) if the
   implemented feature changes the documented architecture.

## The specification is the source of truth

The expected behavior of the system is defined by the content of `specs/`, not by what the
current code does. In practice, this means:

- A behavior change starts with a change to the specification, not with a direct change to
  the code.
- If code and specification diverge, that is a bug — of the code, or of an outdated
  specification — and must be resolved by explicitly updating one of the two, never ignored.
- Agents (human or Claude) implementing or changing a feature must consult
  `specs/<feature>/` first. In the absence of a specification for what is being requested,
  the correct step is to define/ask for the requirements before writing code — not to
  assume the behavior.
