#!/usr/bin/env bash
#
# Runs the RNF09 scanners locally, producing the same artifacts the CI `security` job
# publishes (specs/quality-and-security/design.md §3.3).
#
# Usage:
#   scripts/security-scan.sh
#
# Output: security/govulncheck.json, security/gosec.json (both git-ignored), plus the tool
# versions on stdout so they can be quoted in docs/security-report.md (BR-Q3).
#
# Both tools are run through `go run <module>@<version>`, which resolves in its own module
# context: nothing is added to this project's go.mod/go.sum and the Go 1.22 ceiling of the
# build is untouched (BR-Q8). Building the scanners themselves needs Go >= 1.25 — that is a
# requirement of the toolchain running them, not of this module.

set -euo pipefail

cd "$(dirname "$0")/.."

# Keep in sync with .github/workflows/ci.yml and docs/security-report.md.
GOVULNCHECK_VERSION=v1.7.0
GOSEC_VERSION=v2.28.0

OUTPUT_DIR=security
mkdir -p "$OUTPUT_DIR"

echo "=== Tool versions ==="
echo "govulncheck ${GOVULNCHECK_VERSION}"
echo "gosec       ${GOSEC_VERSION}"
echo "go          $(go version)"
echo "commit      $(git rev-parse HEAD 2>/dev/null || echo 'not a git checkout')"
echo "date        $(date -u '+%Y-%m-%d %H:%M:%S UTC')"

echo
echo "=== govulncheck (dependency and vulnerability analysis) ==="
govulncheck_status=0
go run "golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}" -format json ./... \
    > "$OUTPUT_DIR/govulncheck.json" || govulncheck_status=$?
go run "golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}" ./... || govulncheck_status=$?

echo
echo "=== gosec (SAST) ==="
# -no-fail mirrors CI: this run establishes the reporting baseline rather than gating.
go run "github.com/securego/gosec/v2/cmd/gosec@${GOSEC_VERSION}" \
    -no-fail -fmt json -out "$OUTPUT_DIR/gosec.json" -stdout -verbose=text ./...

echo
echo "Reports written to $OUTPUT_DIR/"
if [[ $govulncheck_status -ne 0 ]]; then
    echo "govulncheck reported findings (exit $govulncheck_status) — triage them into docs/security-report.md"
fi
exit 0
