// Command bootstrap-admin creates the first administrative user on a migrated
// database, from ADMIN_EMAIL/ADMIN_PASSWORD (and optional ADMIN_NAME) — the
// deployed-environment counterpart of the development seed
// (specs/database-migrations/requirements.md BR10/BR11). It is idempotent: an
// existing user with that e-mail is left unchanged. Shipped in the API image
// as /bootstrap-admin.
package main
