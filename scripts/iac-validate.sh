#!/usr/bin/env bash
# Static validation of /infra (FP-35) and /k8s (FP-34) — run by .github/workflows/iac.yml
# and by `make iac-validate`. Nothing here touches a cluster.
#
# Usage: scripts/iac-validate.sh [terraform|manifests|all]   (default: all)
#
#   terraform  terraform fmt -check, init -backend=false, validate (no state, no cluster)
#   manifests  renders /k8s exactly like FP-36's scripts/deploy.sh (kustomize: namespace,
#              image tag + digest, Job name suffix), validates every object with
#              kubeconform -strict, and checks the repository rules of
#              specs/kubernetes-deployment/ (no replicas, no Namespace, no populated Secret)
#
# Environment: KUBERNETES_VERSION (kubeconform schemas, default 1.37.0 — the kind node of
# infra/variables.tf). Requires terraform; kubectl (built-in kustomize) and kubeconform.
set -euo pipefail

cd "$(dirname "$0")/.."

KUBERNETES_VERSION=${KUBERNETES_VERSION:-1.37.0}
NAMESPACE=automotive-workshop
API_IMAGE_NAME=ghcr.io/os-fiapudos/automotive-workshop-api
# A syntactically complete reference, as the pipeline would render it.
SAMPLE_TAG=0123456789abcdef0123456789abcdef01234567
SAMPLE_DIGEST=sha256:$(printf '0%.0s' {1..64})
ROLLOUT_FILES="configmap.yaml deployment.yaml service.yaml hpa.yaml"
JOB_FILES="migration-job.yaml bootstrap-admin-job.yaml"

log() { echo ">>> $*" >&2; }
fail() {
    echo "error: $*" >&2
    FAILED=1
}
FAILED=0

require_tools() {
    local tool
    for tool in "$@"; do
        command -v "$tool" >/dev/null 2>&1 || {
            echo "error: missing tool: $tool" >&2
            exit 1
        }
    done
}

validate_terraform() {
    require_tools terraform
    log "terraform fmt -check"
    terraform fmt -check -recursive -diff infra || fail "run: terraform fmt -recursive infra"
    log "terraform init -backend=false && validate"
    # A throwaway data dir, so validating never mixes with a real local state.
    local data_dir
    data_dir=$(mktemp -d)
    TF_DATA_DIR=$data_dir terraform -chdir=infra init -backend=false -input=false >/dev/null
    TF_DATA_DIR=$data_dir terraform -chdir=infra validate -no-color || fail "terraform validate failed"
    rm -rf "$data_dir"
    if grep -nE 'resource[[:space:]]+"kubernetes_manifest"' infra/*.tf; then
        fail "infra/ must not use kubernetes_manifest (needs the cluster API at plan time — design.md §2)"
    fi
}

# render OUTPUT SUFFIX FILE... — the same kustomization FP-36's deploy.sh writes.
render() {
    local output=$1 suffix=$2 dir file
    shift 2
    dir=$(mktemp -d)
    for file in "$@"; do
        cp "k8s/$file" "$dir/$file"
    done
    {
        echo "apiVersion: kustomize.config.k8s.io/v1beta1"
        echo "kind: Kustomization"
        echo "namespace: $NAMESPACE"
        echo "resources:"
        for file in "$@"; do
            echo "  - $file"
        done
        [[ -z $suffix ]] || echo "nameSuffix: $suffix"
        echo "commonAnnotations:"
        echo "  automotive-workshop/deploy-id: \"ci-validate\""
        echo "images:"
        echo "  - name: $API_IMAGE_NAME"
        echo "    newTag: \"$SAMPLE_TAG\""
        echo "    digest: $SAMPLE_DIGEST"
    } >"$dir/kustomization.yaml"
    kubectl kustomize "$dir" >"$output"
    rm -rf "$dir"
}

validate_manifests() {
    require_tools kubectl kubeconform
    local work file
    work=$(mktemp -d)

    log "rendering /k8s like scripts/deploy.sh (FP-36 design.md §5)"
    # shellcheck disable=SC2086 # space-separated file lists
    render "$work/app.yaml" "" $ROLLOUT_FILES
    render "$work/migration.yaml" "-ci-validate" migration-job.yaml
    render "$work/bootstrap.yaml" "-ci-validate" bootstrap-admin-job.yaml

    log "kubeconform -strict (Kubernetes $KUBERNETES_VERSION)"
    kubeconform -strict -summary -kubernetes-version "$KUBERNETES_VERSION" \
        "$work/app.yaml" "$work/migration.yaml" "$work/bootstrap.yaml" \
        k8s/templates/secrets.example.yaml k8s/load-test/load-generator-job.yaml ||
        fail "kubeconform rejected a manifest"

    log "repository rules (specs/kubernetes-deployment/)"
    grep -qF "image: $API_IMAGE_NAME:$SAMPLE_TAG@$SAMPLE_DIGEST" "$work/app.yaml" ||
        fail "deployment.yaml does not name the API image $API_IMAGE_NAME (the deploy could not set it)"
    grep -qF "image: $API_IMAGE_NAME:$SAMPLE_TAG@$SAMPLE_DIGEST" "$work/bootstrap.yaml" ||
        fail "bootstrap-admin-job.yaml does not name the API image $API_IMAGE_NAME"
    if grep -nE '^[[:space:]]*replicas:' k8s/deployment.yaml; then
        fail "k8s/deployment.yaml must not set replicas — the HPA owns them (design.md §3)"
    fi
    for file in k8s/*.yaml; do
        if grep -qE '^kind:[[:space:]]*(Namespace|Secret)[[:space:]]*$' "$file"; then
            fail "$file: Namespace is Terraform's and Secrets are created at deploy time — not in k8s/"
        fi
        if grep -nE '^[[:space:]-]*image:.*[<>]' "$file"; then
            fail "$file: placeholder image reference"
        fi
    done
    if grep -nE '^[[:space:]]+[A-Z_]+:' k8s/templates/secrets.example.yaml |
        grep -vE ': "(REPLACE_ME)?"$|ADMIN_NAME:'; then
        fail "k8s/templates/secrets.example.yaml must hold placeholders only (REPLACE_ME or empty)"
    fi
    for file in $JOB_FILES; do
        grep -q 'backoffLimit:' "k8s/$file" || fail "k8s/$file: backoffLimit must be explicit"
    done
    grep -q 'backoffLimit: 0' k8s/migration-job.yaml ||
        fail "k8s/migration-job.yaml: backoffLimit must be 0 (a failed migration leaves the database dirty)"
    rm -rf "$work"
}

case ${1:-all} in
terraform) validate_terraform ;;
manifests) validate_manifests ;;
all)
    validate_terraform
    validate_manifests
    ;;
*)
    echo "usage: $0 [terraform|manifests|all]" >&2
    exit 2
    ;;
esac

if ((FAILED)); then
    echo "validation failed" >&2
    exit 1
fi
log "ok"
