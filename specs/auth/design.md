# auth — Design

Satisfies [requirements.md](requirements.md). Every section references the requirements it
serves. Decisions marked **[decision]** were approved by the project owner on 2026-08-12.

## 1. Placement in the architecture

Follows the vertical slice pattern ([specs/architecture.md](../architecture.md)):

- `internal/features/auth/` — the feature slice: handler, service, repository, model.
- `internal/shared/` — genuinely cross-cutting code introduced by this feature:
  - `shared/database/` — PostgreSQL connection pool (first feature to touch the DB).
  - `shared/token/` — JWT signing/verification (used by the auth slice and by the
    middleware; lives in shared so no feature imports another feature).
  - `shared/middleware/` — HTTP authentication middleware (FR3, FR6).
  - `shared/httpx/` — standard JSON response/error envelope helpers (RNF04).
- `cmd/api/main.go` — wiring only: builds the pool, registers routes.

## 2. Dependencies **[decision]**

First external dependencies of the module (CLAUDE.md §12 requires explicit approval —
granted):

| Dependency | Purpose | Requirement |
| --- | --- | --- |
| `github.com/golang-jwt/jwt/v5` | JWT issue/verify | FR1, FR3, BR3 |
| `golang.org/x/crypto` (bcrypt) | Password hashing | BR1 |
| `github.com/jackc/pgx/v5` (pgxpool) | PostgreSQL driver/pool | repository access |

No HTTP framework: Go 1.22 `net/http` method patterns (`"POST /api/v1/auth/login"`) are
sufficient at the current route count.

## 3. Data model (FR5, BR1)

New `users` table in [docs/schema.sql](../../docs/schema.sql), following the established
conventions (snake_case, `id UUID`, `code BIGINT IDENTITY`, timestamps + `set_updated_at`
trigger, `COMMENT ON`):

```sql
CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code          BIGINT GENERATED ALWAYS AS IDENTITY,
    name          TEXT NOT NULL,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

- `users` must also be added to the table array of the `set_updated_at` trigger block at
  the end of `schema.sql` (it enumerates tables explicitly).
- `docs/entities.md` gains the corresponding `User` entity section.
- `docs/seed.sql` gains one initial administrative user (FR5):
  `admin@workshop.local`, password `admin123` **for local development only**, stored as a
  bcrypt hash (AC5), inserted idempotently (`ON CONFLICT DO NOTHING`) like the rest of the
  seed. The plaintext dev password is documented in the seed comment, never in code or logs.
- Login identifier is **email** [decision: card did not specify; email chosen].

## 4. API contract (FR1, FR2, FR4; RNF04)

### POST `/api/v1/auth/login` — public

Request:

```json
{ "email": "admin@workshop.local", "password": "..." }
```

- 200 (AC1):

```json
{ "access_token": "<jwt>", "token_type": "Bearer", "expires_in": 3600 }
```

- 401 (AC2, BR4) — same body for unknown email and wrong password:

```json
{ "error": { "code": "UNAUTHORIZED", "message": "invalid credentials" } }
```

- 400 — malformed JSON / missing fields:

```json
{ "error": { "code": "INVALID_REQUEST", "message": "<what is wrong>" } }
```

### GET `/api/v1/auth/me` — protected (FR4)

- 200 with the authenticated user's public data (no hash):

```json
{ "id": "<uuid>", "code": 1, "name": "Admin", "email": "admin@workshop.local" }
```

- 401 via middleware when the token is missing/invalid/expired (AC3, AC4).

### Error envelope (RNF04) — project-wide precedent

All error responses use `{"error": {"code": "...", "message": "..."}}`, produced by
`shared/httpx`. Future features must reuse these helpers.

## 5. Token (FR1, FR3, BR2, BR3)

- JWT HS256 signed with `JWT_SECRET` from the environment (BR2, AC6). The application
  refuses to start without it (fail-fast in `main.go`).
- TTL from `JWT_TTL` (Go duration string, default `1h`) — every token carries `exp` (BR3).
- Claims: `sub` = user id (UUID), `exp`, `iat`. No roles claim in the MVP (no permission
  model — see requirements scope note).
- `shared/token` exposes `Generate(userID) (string, error)` and
  `Verify(tokenString) (userID, error)`; verification rejects expired tokens and any
  signing method other than HS256.
- `.env.example` documents `JWT_SECRET` (placeholder value) and `JWT_TTL`.

## 6. Login flow (FR1, FR2)

```
handler: decode/validate input (400 on malformed)
  → service.Login(email, password)
      → repository.FindByEmail (pgx, parameterized query)
      → bcrypt.CompareHashAndPassword
      → token.Generate
  → 200 {access_token, ...}
unknown email OR wrong password → single ErrInvalidCredentials → 401 generic (BR4)
```

The service compares with bcrypt even when it could fail earlier only insofar as this does
not complicate the code; the guaranteed property is the identical response body and status
(BR4). Timing-attack hardening beyond bcrypt's cost is not an MVP requirement.

## 7. Middleware and public routes (FR3, FR6)

- `shared/middleware.RequireAuth(next http.Handler)`: extracts
  `Authorization: Bearer <token>`, calls `token.Verify`, injects the user id into the
  request context, or responds 401 with the standard envelope (AC3, AC4).
- Public routes are an explicit list in `main.go`: `GET /health` and
  `POST /api/v1/auth/login`. Every other route is registered wrapped in `RequireAuth`
  (FR6). In the MVP the only protected route is `GET /api/v1/auth/me`; the convention is
  set for the features that follow.

## 8. Logging (BR5, RNF08)

- No log statement may include the password, the hash, or the token — log events only
  (e.g. failed login attempt with the user's `code` or email, never the secret material).
- Enforced by review + integration test asserting log output contains no secret (AC7).

## 9. Testing (AC1–AC8)

- **Unit** (in the slice / shared packages, stdlib `testing`):
  - `service_test.go`: unknown email → generic error; wrong password → generic error;
    success → token with expiration (AC1, AC2 at the service level).
  - `token_test.go` (shared): round-trip, expired token rejected, wrong signature rejected.
- **Integration** (`internal/handlers_test/auth_test.go`, AC8) against the compose
  Postgres, skipped when `DATABASE_URL` is not set (keeps CI green without a DB service):
  - login 200 + well-formed JWT (AC1)
  - login 401 with generic body for unknown email and wrong password (AC2)
  - `/auth/me` without token → 401 (AC3)
  - `/auth/me` with tampered and with expired token → 401 (AC4)
  - `/auth/me` with valid token → 200
- AC5/AC6 are validated by inspection: seed stores only a bcrypt hash; `.env` is
  gitignored and `JWT_SECRET` appears only in `.env.example` as a placeholder.

## 10. Out of scope (traceability)

- User CRUD, roles/403, refresh tokens, OpenAPI/Swagger (see requirements.md scope notes).
