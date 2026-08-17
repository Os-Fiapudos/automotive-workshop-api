# auth — Requirements

Source: Jira card "Authentication" (Tech Challenge), refined on 2026-08-12.

## Context and goal

The workshop API will expose administrative operations (customers, vehicles, service
orders, quotes). These operations must not be publicly accessible. This feature introduces
authentication so that only authenticated administrative users can access internal
operations.

## User story

> As an administrative user,
> I want to authenticate against the API,
> so that I can securely access the workshop's internal operations.

## Related non-functional requirements

- **RNF02** — JWT authentication on administrative APIs.
- **RNF04** — Consistent HTTP responses and errors.
- **RNF08** — Logs must not contain passwords, tokens, or full documents.
- **RNF10** — OpenAPI documentation updated. **Out of scope for this feature** by
  explicit decision (2026-08-12): the project has no OpenAPI artifact yet; RNF10 will be
  handled as a separate feature. Recorded here so the scope cut is traceable.

## Functional requirements

- **FR1** — An administrative user can authenticate with their credentials and receive an
  access token with an expiration date.
- **FR2** — Authentication with invalid credentials is rejected. The rejection message is
  the same generic message whether the user does not exist or the password is wrong.
- **FR3** — Administrative operations are only accessible with a valid, non-expired access
  token. Requests without a token, with an invalid token, or with an expired token are
  rejected as unauthenticated.
- **FR4** — An authenticated user can retrieve their own identity information, as a way to
  validate the token (optional endpoint per the card; included in scope).
- **FR5** — The MVP does not include administrative user management (CRUD). The initial
  administrative user is created via seed data.
- **FR6** — Public operations (the ones that do not require authentication) are explicitly
  identified; everything else requires authentication by default.

## Business rules

- **BR1** — Passwords are never stored or transported in plain text at rest; only a hash
  produced by a strong, purpose-built password hashing algorithm (bcrypt, Argon2, or
  equivalent) is stored.
- **BR2** — The token signing secret is provided via environment variable and is never
  versioned in the repository.
- **BR3** — Every issued token has an expiration date.
- **BR4** — Invalid user and invalid password produce the same generic error message
  (no user enumeration).
- **BR5** — Passwords, password hashes, and tokens never appear in application logs.

## Acceptance criteria

- **AC1** — Valid credentials return HTTP 200 with a JWT that carries an expiration.
- **AC2** — Invalid credentials return HTTP 401 with the generic message.
- **AC3** — A protected operation called without a token returns HTTP 401.
- **AC4** — A protected operation called with an invalid or expired token returns HTTP 401.
- **AC5** — Passwords exist in the database exclusively as hashes.
- **AC6** — The JWT secret is not versioned in the repository.
- **AC7** — No log line contains a password, a full token, or a password hash.
- **AC8** — Integration tests exist covering login and access to protected routes.

Criteria from the original card left out of this feature's scope:

- HTTP 403 for unauthorized operations — **not applicable in the MVP**: there is no
  permission/role model yet. To be specified together with the first feature that needs
  role-based access.
- Swagger/Bearer token support — out of scope with RNF10 (see above).
