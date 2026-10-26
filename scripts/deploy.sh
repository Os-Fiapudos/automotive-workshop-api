#!/usr/bin/env bash
#
# Staged deploy of the API to Kubernetes (specs/ci-cd-pipeline/design.md §4). The CI
# `deploy` job runs one stage per step against an ephemeral cluster inside the runner; on a
# developer machine, `up` builds the demonstration environment, which stays up.
#
# Usage: scripts/deploy.sh <stage>
#   check      tools, the /infra and /k8s contract (FP-34/FP-35, design.md §9), IMAGE,
#              manifest rendering — creates nothing
#   infra      terraform apply (cluster, namespace, metrics-server, Postgres) and wait until
#              Postgres accepts connections
#   config     API Secret, bootstrap Secret, migrations ConfigMap, registry pull Secret
#   migrate    migration Job until Complete — Failed or a timeout stops here (no rollout)
#   bootstrap  /bootstrap-admin Job until Complete
#   rollout    Deployment/Service/HPA with IMAGE; wait for the rollout
#   smoke      port-forward the Service and run scripts/deploy-smoke.sh
#   evidence   sanitized evidence in DEPLOY_EVIDENCE_DIR (summary.md, logs, cluster state)
#   destroy    terraform destroy (outside CI: only with DEPLOY_CONFIRM_DESTROY=yes)
#   up         check, infra, config, migrate, bootstrap, rollout, smoke, evidence — the
#              demonstration environment; never destroys
#
# Environment (each stage checks only what it uses and never prints a value):
#   IMAGE                      name:<tag>[@sha256:<digest>] or name@sha256:<digest>,
#                              lowercase, not :latest (CI: name:<sha>@<digest>)
#   DEPLOY_ID                  environment id (default local-<UTC timestamp>;
#                              CI: ci-<run id>-<attempt>)
#   DEPLOY_EPHEMERAL           "true" for CI's throwaway environment
#   POSTGRES_PASSWORD          Terraform's postgres_password and DATABASE_URL
#   JWT_SECRET, EXTERNAL_NOTIFICATION_SECRET, EXTERNAL_STATUS_UPDATE_SECRET
#   RESEND_API_KEY, RESEND_FROM_EMAIL     optional pair
#   ADMIN_EMAIL, ADMIN_PASSWORD, ADMIN_NAME (optional)   bootstrap Job and smoke test
#   REGISTRY_USERNAME, REGISTRY_PASSWORD  optional: image pull Secret for a private package
#   SIMULATE_MIGRATION_FAILURE "true": add a migration that always fails (needs
#                              DEPLOY_EPHEMERAL=true)
#   DEPLOY_EVIDENCE_DIR        default deploy-evidence/<DEPLOY_ID> (git-ignored)
#   POSTGRES_READY_TIMEOUT 180, MIGRATION_TIMEOUT 300, BOOTSTRAP_TIMEOUT 180,
#   ROLLOUT_TIMEOUT 300 (seconds); SMOKE_LOCAL_PORT 18080
#
# Requires terraform, kubectl (its built-in kustomize renders the manifests), curl and jq.

set -euo pipefail
umask 077

SELF="$(cd "$(dirname "$0")" && pwd)/$(basename "$0")"
cd "$(dirname "$0")/.."

# The integration contract with FP-34 (/k8s) and FP-35 (/infra) — design.md §9.
INFRA_DIR=infra
K8S_DIR=k8s
NAMESPACE=automotive-workshop
API_IMAGE_NAME=ghcr.io/os-fiapudos/automotive-workshop-api # as committed in /k8s
DEPLOYMENT=automotive-workshop-api
SERVICE=automotive-workshop-api
API_SECRET=automotive-workshop-api-secrets
ADMIN_SECRET=automotive-workshop-admin-bootstrap
PULL_SECRET=ghcr-pull
MIGRATIONS_CONFIGMAP=automotive-workshop-api-migrations
DEPLOY_INFO_CONFIGMAP=automotive-workshop-deploy
MIGRATION_JOB_BASE=automotive-workshop-api-migrate
BOOTSTRAP_JOB_BASE=automotive-workshop-api-bootstrap-admin
ROLLOUT_FILES="deployment.yaml service.yaml hpa.yaml"
FIELD_MANAGER=automotive-workshop-cd
SIMULATED_FAILURE_FILE=99999999_fp36_simulated_failure.up.sql

# Credentials redacted from everything the evidence stage writes (design.md §6.4).
SECRET_VARS="POSTGRES_PASSWORD JWT_SECRET EXTERNAL_NOTIFICATION_SECRET \
EXTERNAL_STATUS_UPDATE_SECRET ADMIN_PASSWORD REGISTRY_PASSWORD RESEND_API_KEY"

DEPLOY_ID=${DEPLOY_ID:-local-$(date -u +%Y%m%d%H%M%S)}
export DEPLOY_ID # `up` runs every stage as a child process of this one
DEPLOY_EPHEMERAL=${DEPLOY_EPHEMERAL:-false}
DEPLOY_COMMIT=${GITHUB_SHA:-$(git rev-parse HEAD 2>/dev/null || echo unknown)}
EVIDENCE_DIR=${DEPLOY_EVIDENCE_DIR:-deploy-evidence/$DEPLOY_ID}
POSTGRES_READY_TIMEOUT=${POSTGRES_READY_TIMEOUT:-180}
MIGRATION_TIMEOUT=${MIGRATION_TIMEOUT:-300}
BOOTSTRAP_TIMEOUT=${BOOTSTRAP_TIMEOUT:-180}
ROLLOUT_TIMEOUT=${ROLLOUT_TIMEOUT:-300}
SMOKE_LOCAL_PORT=${SMOKE_LOCAL_PORT:-18080}
MIGRATION_JOB=$MIGRATION_JOB_BASE-$DEPLOY_ID
BOOTSTRAP_JOB=$BOOTSTRAP_JOB_BASE-$DEPLOY_ID

WORK_DIR=$(mktemp -d "${TMPDIR:-/tmp}/deploy.XXXXXX")
STAGE=""
STAGE_START=0
PORT_FORWARD_PID=""

log() { echo ">>> $*" >&2; }
die() {
    echo "error: $*" >&2
    exit 1
}

usage() {
    awk 'NR > 2 && /^#/ { sub(/^# ?/, ""); print; next } NR > 2 { exit }' "$SELF" >&2
    exit 2
}

# On exit: record the running stage's outcome for the evidence, stop the port-forward,
# remove the temporary files (rendered manifests, secret files).
on_exit() {
    local status=$?
    if [[ -n $STAGE && $STAGE != evidence ]]; then
        record_stage failed
    fi
    if [[ -n $PORT_FORWARD_PID ]]; then
        kill "$PORT_FORWARD_PID" 2>/dev/null || true
    fi
    rm -rf "$WORK_DIR"
    exit "$status"
}
trap on_exit EXIT

record_stage() {
    mkdir -p "$EVIDENCE_DIR"
    printf '%s\t%s\t%ss\n' "$STAGE" "$1" $((SECONDS - STAGE_START)) >>"$EVIDENCE_DIR/stages.tsv"
}

run_stage() {
    STAGE=$1
    STAGE_START=$SECONDS
    log "[$DEPLOY_ID] $STAGE"
    "stage_$1"
    if [[ $STAGE != evidence ]]; then
        record_stage ok
    fi
    STAGE=""
}

# require_env NAME... — fails naming every missing variable (never a value).
require_env() {
    local name missing=()
    for name in "$@"; do
        [[ -n ${!name:-} ]] || missing+=("$name")
    done
    ((${#missing[@]} == 0)) || die "missing environment variable(s): ${missing[*]}"
}

require_tools() {
    local tool missing=()
    for tool in "$@"; do
        command -v "$tool" >/dev/null 2>&1 || missing+=("$tool")
    done
    ((${#missing[@]} == 0)) || die "missing tool(s): ${missing[*]}"
}

lowercase() { printf '%s' "$1" | tr '[:upper:]' '[:lower:]'; }

# urlencode VALUE — percent-encodes everything outside the URL-unreserved set, byte by byte.
urlencode() {
    local LC_ALL=C value=$1 out="" char i
    for ((i = 0; i < ${#value}; i++)); do
        char=${value:i:1}
        case $char in
        [a-zA-Z0-9.~_-]) out+=$char ;;
        *) out+=$(printf '%%%02X' "'$char") ;;
        esac
    done
    printf '%s' "$out"
}

# --- Terraform (design.md §6.3) -------------------------------------------------------

tf() { terraform -chdir="$INFRA_DIR" "$@"; }

# Prints only the human-readable message of each `terraform -json` line (plus a
# diagnostic's detail): resource addresses and progress, never attribute or output values.
# The kind provider does not mark the cluster's private key as sensitive, so plain plan
# output would print it.
tf_messages() {
    jq -R -r --unbuffered '
        . as $line | (try fromjson catch null) as $msg
        | if ($msg | type) == "object" then
            ($msg["@message"] // "")
            + (if $msg.diagnostic.detail then "\n    " + ($msg.diagnostic.detail | gsub("\n"; "\n    ")) else "" end)
          else $line end'
}

tf_init() { tf init -input=false -no-color >/dev/null; }

tf_output() { tf output -raw "$1" 2>/dev/null; }

# Points kubectl at the cluster Terraform created — never at whatever context the
# developer's kubeconfig happens to have selected.
use_cluster() {
    local path
    path=$(tf_output kubeconfig_path) ||
        die "Terraform output kubeconfig_path not found — run the infra stage first (contract: specs/ci-cd-pipeline/design.md §9 item 5)"
    [[ -f $path ]] || die "the kubeconfig Terraform reports does not exist — run the infra stage again"
    export KUBECONFIG=$path
}

kc() { kubectl --namespace "$NAMESPACE" "$@"; }

# Applies generated objects (Secrets, ConfigMaps) server-side: client-side apply would copy
# their values into the last-applied-configuration annotation (design.md §6.2).
apply_server_side() { kubectl apply --server-side --force-conflicts --field-manager="$FIELD_MANAGER" "$@"; }

database_url() {
    local template
    template=$(tf_output postgres_database_url_template) ||
        die "Terraform output postgres_database_url_template not found (contract: design.md §9 item 5)"
    [[ $template == *"<password>"* ]] ||
        die "postgres_database_url_template has no <password> placeholder (contract: design.md §9 item 5)"
    printf '%s' "${template/<password>/$(urlencode "$POSTGRES_PASSWORD")}"
}

# --- Image and manifests (design.md §2, §5) ---------------------------------------------

IMAGE_NAME=""
IMAGE_TAG=""
IMAGE_DIGEST=""

parse_image() {
    local rest last
    IMAGE_TAG=""
    IMAGE_DIGEST=""
    rest=$IMAGE
    if [[ $rest == *@* ]]; then
        IMAGE_DIGEST=${rest#*@}
        rest=${rest%%@*}
    fi
    last=${rest##*/}
    if [[ $last == *:* ]]; then
        IMAGE_TAG=${last##*:}
        IMAGE_NAME=${rest%:*}
    else
        IMAGE_NAME=$rest
    fi
}

validate_image() {
    require_env IMAGE
    [[ $IMAGE == "$(lowercase "$IMAGE")" ]] || die "IMAGE must be lowercase (registries reject uppercase names)"
    [[ $IMAGE != *[\<\>[:space:]]* ]] || die "IMAGE contains a placeholder or whitespace"
    parse_image
    [[ $IMAGE_NAME == */* ]] || die "IMAGE must include the registry and repository (e.g. ghcr.io/os-fiapudos/automotive-workshop-api)"
    [[ -n $IMAGE_TAG || -n $IMAGE_DIGEST ]] || die "IMAGE needs a tag (the commit SHA) and/or a digest"
    [[ $IMAGE_TAG != latest ]] || die "IMAGE must not use the mutable tag latest"
    if [[ -n $IMAGE_DIGEST && ! $IMAGE_DIGEST =~ ^sha256:[0-9a-f]{64}$ ]]; then
        die "IMAGE digest must be sha256:<64 hex characters>"
    fi
}

# render OUTPUT NAME_SUFFIX FILE... — copies the given /k8s files into a scratch directory
# and renders them with kubectl's built-in kustomize: namespace, deploy annotations, the
# API image (when IMAGE is set) and, for Jobs, a per-run name suffix. The committed
# manifests are never modified.
render() {
    local output=$1 suffix=$2 dir file
    shift 2
    dir=$(mktemp -d "$WORK_DIR/render.XXXXXX")
    for file in "$@"; do
        cp "$K8S_DIR/$file" "$dir/$file"
    done
    {
        echo "apiVersion: kustomize.config.k8s.io/v1beta1"
        echo "kind: Kustomization"
        echo "namespace: $NAMESPACE"
        echo "resources:"
        for file in "$@"; do
            echo "  - $file"
        done
        if [[ -n $suffix ]]; then
            echo "nameSuffix: $suffix"
        fi
        echo "commonAnnotations:"
        echo "  automotive-workshop/deploy-id: \"$DEPLOY_ID\""
        echo "  automotive-workshop/commit-sha: \"$DEPLOY_COMMIT\""
        if [[ -n ${IMAGE:-} ]]; then
            parse_image
            echo "images:"
            echo "  - name: $API_IMAGE_NAME"
            echo "    newName: $IMAGE_NAME"
            if [[ -n $IMAGE_TAG ]]; then
                echo "    newTag: \"$IMAGE_TAG\""
            fi
            if [[ -n $IMAGE_DIGEST ]]; then
                echo "    digest: $IMAGE_DIGEST"
            fi
        fi
    } >"$dir/kustomization.yaml"
    kubectl kustomize "$dir" >"$output" || die "could not render ${*} (kubectl kustomize)"
    # No placeholder may reach the cluster (FP-36: "sem <org> ou <tag>").
    if grep -nE '^[[:space:]-]*image:.*[<>]' "$output" >&2; then
        die "a placeholder image reached the rendered manifests — /k8s must name the API image $API_IMAGE_NAME (design.md §9 item 7)"
    fi
}

# rendered_images FILE — every image: value in a rendered file, one per line.
rendered_images() { sed -nE 's/^[[:space:]-]*image:[[:space:]]*"?([^"[:space:]]+)"?.*/\1/p' "$1"; }

# expect_api_image FILE WHAT — the rendered file must use exactly IMAGE for the API image.
expect_api_image() {
    rendered_images "$1" | grep -qxF "$IMAGE" ||
        die "$2 does not use the API image $API_IMAGE_NAME, so IMAGE could not be set (design.md §9 item 7)"
}

# --- Jobs -------------------------------------------------------------------------------

# wait_job JOB TIMEOUT — 0 when Complete, 1 when Failed, 2 on timeout, 3 when its pod cannot
# start (WAIT_JOB_REASON says why). Prints the Job's logs either way: they are the first
# thing to read when a migration fails.
WAIT_JOB_REASON=""
wait_job() {
    local job=$1 timeout=$2 start=$SECONDS conditions waiting result=2
    WAIT_JOB_REASON=""
    while ((SECONDS - start < timeout)); do
        conditions=$(kc get job "$job" -o jsonpath='{range .status.conditions[?(@.status=="True")]}{.type}{" "}{end}' 2>/dev/null) || conditions=""
        case " $conditions " in
        *" Complete "*)
            result=0
            break
            ;;
        *" Failed "*)
            result=1
            break
            ;;
        esac
        # A pod that cannot even start would otherwise burn the whole timeout. A failed pull
        # gets a minute of kubelet retries before it counts.
        waiting=$(kc get pods -l "job-name=$job" -o jsonpath='{.items[*].status.containerStatuses[*].state.waiting.reason}' 2>/dev/null) || waiting=""
        case " $waiting " in
        *" CreateContainerConfigError "* | *" InvalidImageName "* | *" ErrImageNeverPull "*)
            WAIT_JOB_REASON=$waiting
            ;;
        *" ImagePullBackOff "* | *" ErrImagePull "*)
            ((SECONDS - start < 60)) || WAIT_JOB_REASON=$waiting
            ;;
        esac
        if [[ -n $WAIT_JOB_REASON ]]; then
            result=3
            break
        fi
        sleep 3
    done
    log "logs of Job $job:"
    kc logs "job/$job" --all-containers --tail=200 2>&1 | sanitize_stream || true
    if ((result >= 2)); then
        log "pods of Job $job:"
        kc get pods -l "job-name=$job" -o wide 2>&1 || true
    fi
    return "$result"
}

# job_failure JOB RESULT — the message for a wait_job result other than 0.
job_failure() {
    case $2 in
    1) echo "Job $1 Failed" ;;
    3) echo "Job $1 could not start its pod ($WAIT_JOB_REASON)" ;;
    *) echo "Job $1 did not finish in time" ;;
    esac
}

# --- Sanitizing (design.md §6.4) --------------------------------------------------------

# Replaces every known credential value and any postgres URL password with [REDACTED].
sanitize_stream() {
    local text name value
    text=$(cat)
    for name in $SECRET_VARS; do
        value=${!name:-}
        if [[ -n $value ]]; then
            text=${text//"$value"/[REDACTED]}
        fi
    done
    printf '%s\n' "$text" | sed -E 's#(postgres(ql)?://[^:/@[:space:]]+:)[^@[:space:]]+@#\1[REDACTED]@#g'
}

sanitize_file() {
    local file=$1 clean
    clean=$(sanitize_stream <"$file")
    printf '%s\n' "$clean" >"$file"
}

# leaked_credentials DIR — lists the files that still contain a credential value.
leaked_credentials() {
    local name value
    for name in $SECRET_VARS; do
        value=${!name:-}
        if [[ -n $value ]]; then
            grep -rlF -- "$value" "$1" 2>/dev/null || true
        fi
    done
    # A URL password that is not the [REDACTED] marker.
    grep -rlE 'postgres(ql)?://[^:/@[:space:]]+:[^[@[:space:]][^@[:space:]]*@' "$1" 2>/dev/null || true
}

# --- Stages -----------------------------------------------------------------------------

check_simulation_allowed() {
    if [[ ${SIMULATE_MIGRATION_FAILURE:-false} == true && $DEPLOY_EPHEMERAL != true ]]; then
        die "SIMULATE_MIGRATION_FAILURE leaves the database dirty — it is only accepted with DEPLOY_EPHEMERAL=true"
    fi
}

stage_check() {
    require_tools terraform kubectl curl jq
    local missing=() file job
    compgen -G "$INFRA_DIR/*.tf" >/dev/null ||
        missing+=("$INFRA_DIR/*.tf (FP-35, specs/infrastructure-provisioning/)")
    for file in $ROLLOUT_FILES migration-job.yaml bootstrap-admin-job.yaml; do
        [[ -f $K8S_DIR/$file ]] || missing+=("$K8S_DIR/$file (FP-34, specs/kubernetes-deployment/)")
    done
    if ((${#missing[@]} > 0)); then
        die "the deploy contract (specs/ci-cd-pipeline/design.md §9) is not met — missing: ${missing[*]}"
    fi

    [[ $DEPLOY_ID =~ ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ ]] ||
        die "DEPLOY_ID must be lowercase letters, digits and dashes"
    for job in "$MIGRATION_JOB" "$BOOTSTRAP_JOB"; do
        ((${#job} <= 63)) || die "Job name $job is longer than 63 characters — use a shorter DEPLOY_ID"
    done
    check_simulation_allowed
    validate_image

    # Render everything now, so a contract mismatch fails before any resource exists.
    # shellcheck disable=SC2086 # ROLLOUT_FILES is a list
    render "$WORK_DIR/app.yaml" "" $ROLLOUT_FILES
    expect_api_image "$WORK_DIR/app.yaml" "$K8S_DIR/deployment.yaml"
    render "$WORK_DIR/bootstrap.yaml" "-$DEPLOY_ID" bootstrap-admin-job.yaml
    expect_api_image "$WORK_DIR/bootstrap.yaml" "$K8S_DIR/bootstrap-admin-job.yaml"
    grep -q '/bootstrap-admin' "$WORK_DIR/bootstrap.yaml" ||
        die "$K8S_DIR/bootstrap-admin-job.yaml does not run /bootstrap-admin (design.md §9 item 11)"
    grep -qF "$ADMIN_SECRET" "$WORK_DIR/bootstrap.yaml" ||
        die "$K8S_DIR/bootstrap-admin-job.yaml does not read the Secret $ADMIN_SECRET (design.md §9 item 11)"
    render "$WORK_DIR/migration.yaml" "-$DEPLOY_ID" migration-job.yaml
    grep -qF "$MIGRATIONS_CONFIGMAP" "$WORK_DIR/migration.yaml" ||
        die "$K8S_DIR/migration-job.yaml does not mount the ConfigMap $MIGRATIONS_CONFIGMAP (design.md §9 item 10)"
    for job in "$MIGRATION_JOB:$WORK_DIR/migration.yaml" "$BOOTSTRAP_JOB:$WORK_DIR/bootstrap.yaml"; do
        grep -qE "^  name: ${job%%:*}\$" "${job#*:}" ||
            die "the rendered Job is not named ${job%%:*} — check the base Job names (design.md §9 items 10-11)"
    done
    log "contract ok — image $IMAGE, environment $DEPLOY_ID"
}

stage_infra() {
    require_tools terraform kubectl jq
    require_env POSTGRES_PASSWORD
    export TF_VAR_postgres_password=$POSTGRES_PASSWORD
    tf_init
    tf apply -auto-approve -input=false -json | tf_messages
    use_cluster

    log "waiting for Postgres to accept connections (up to ${POSTGRES_READY_TIMEOUT}s)"
    kc rollout status statefulset/postgres --timeout="${POSTGRES_READY_TIMEOUT}s"
    local start=$SECONDS
    # TCP on 127.0.0.1: the official image's first start listens on the socket only, while
    # it initializes, so a socket check would pass too early.
    until kc exec statefulset/postgres -- pg_isready -q -h 127.0.0.1 -p 5432 >/dev/null 2>&1; do
        ((SECONDS - start < POSTGRES_READY_TIMEOUT)) ||
            die "Postgres did not accept connections within ${POSTGRES_READY_TIMEOUT}s"
        sleep 2
    done
    log "Postgres is ready"
}

stage_config() {
    require_tools kubectl terraform
    require_env POSTGRES_PASSWORD JWT_SECRET EXTERNAL_NOTIFICATION_SECRET \
        EXTERNAL_STATUS_UPDATE_SECRET ADMIN_EMAIL ADMIN_PASSWORD
    check_simulation_allowed
    use_cluster

    # Identifies the environment without touching what Terraform owns (annotating its
    # Namespace would be undone by the next terraform apply).
    kc create configmap "$DEPLOY_INFO_CONFIGMAP" --dry-run=client -o yaml \
        --from-literal=deploy-id="$DEPLOY_ID" --from-literal=ephemeral="$DEPLOY_EPHEMERAL" \
        --from-literal=commit-sha="$DEPLOY_COMMIT" --from-literal=image="${IMAGE:-}" \
        --from-literal=run-url="$(run_url)" --from-literal=configured-at="$(date -u +%Y-%m-%dT%H:%M:%SZ)" |
        apply_server_side -f -

    # One file per key (mode 0600): values never appear on a command line.
    local api_dir=$WORK_DIR/api-secret admin_dir=$WORK_DIR/admin-secret
    mkdir -p "$api_dir" "$admin_dir"
    database_url >"$api_dir/DATABASE_URL"
    printf '%s' "$JWT_SECRET" >"$api_dir/JWT_SECRET"
    printf '%s' "$EXTERNAL_NOTIFICATION_SECRET" >"$api_dir/EXTERNAL_NOTIFICATION_SECRET"
    printf '%s' "$EXTERNAL_STATUS_UPDATE_SECRET" >"$api_dir/EXTERNAL_STATUS_UPDATE_SECRET"
    if [[ -n ${RESEND_API_KEY:-} ]]; then
        require_env RESEND_FROM_EMAIL
        printf '%s' "$RESEND_API_KEY" >"$api_dir/RESEND_API_KEY"
        printf '%s' "$RESEND_FROM_EMAIL" >"$api_dir/RESEND_FROM_EMAIL"
    fi
    kc create secret generic "$API_SECRET" --from-file="$api_dir" --dry-run=client -o yaml |
        apply_server_side -f -

    printf '%s' "$ADMIN_EMAIL" >"$admin_dir/ADMIN_EMAIL"
    printf '%s' "$ADMIN_PASSWORD" >"$admin_dir/ADMIN_PASSWORD"
    if [[ -n ${ADMIN_NAME:-} ]]; then
        printf '%s' "$ADMIN_NAME" >"$admin_dir/ADMIN_NAME"
    fi
    kc create secret generic "$ADMIN_SECRET" --from-file="$admin_dir" --dry-run=client -o yaml |
        apply_server_side -f -

    # The migrations of this commit, never a committed copy that could drift from them.
    local migrations_dir=$WORK_DIR/migrations
    mkdir -p "$migrations_dir"
    cp migrations/*.sql "$migrations_dir/"
    if [[ ${SIMULATE_MIGRATION_FAILURE:-false} == true ]]; then
        log "SIMULATE_MIGRATION_FAILURE: adding $SIMULATED_FAILURE_FILE to this run's ConfigMap only"
        cat >"$migrations_dir/$SIMULATED_FAILURE_FILE" <<'SQL'
-- Injected by scripts/deploy.sh for the FP-36 controlled-failure run. Never committed.
DO $$ BEGIN RAISE EXCEPTION 'FP-36 simulated migration failure (workflow_dispatch input)'; END $$;
SQL
    fi
    kc create configmap "$MIGRATIONS_CONFIGMAP" --from-file="$migrations_dir" --dry-run=client -o yaml |
        apply_server_side -f -

    if [[ -n ${REGISTRY_USERNAME:-} && -n ${REGISTRY_PASSWORD:-} ]]; then
        validate_image
        local registry=${IMAGE%%/*} auth
        auth=$(printf '%s:%s' "$REGISTRY_USERNAME" "$REGISTRY_PASSWORD" | base64 | tr -d '\n')
        printf '{"auths":{"%s":{"auth":"%s"}}}' "$registry" "$auth" >"$WORK_DIR/dockerconfig.json"
        kc create secret generic "$PULL_SECRET" --type=kubernetes.io/dockerconfigjson \
            --from-file=.dockerconfigjson="$WORK_DIR/dockerconfig.json" --dry-run=client -o yaml |
            apply_server_side -f -
        kc patch serviceaccount default --type merge \
            -p "{\"imagePullSecrets\":[{\"name\":\"$PULL_SECRET\"}]}"
    else
        log "no REGISTRY_USERNAME/REGISTRY_PASSWORD — the image must be pullable without credentials"
    fi
}

stage_migrate() {
    require_tools kubectl terraform
    use_cluster
    render "$WORK_DIR/migration.yaml" "-$DEPLOY_ID" migration-job.yaml
    kubectl create -f "$WORK_DIR/migration.yaml"
    log "waiting for Job $MIGRATION_JOB (up to ${MIGRATION_TIMEOUT}s)"
    local result=0
    wait_job "$MIGRATION_JOB" "$MIGRATION_TIMEOUT" || result=$?
    ((result == 0)) || die "migration $(job_failure "$MIGRATION_JOB" "$result") — the rollout is blocked"
    log "migration Job $MIGRATION_JOB Complete"
}

stage_bootstrap() {
    require_tools kubectl terraform
    validate_image
    use_cluster
    render "$WORK_DIR/bootstrap.yaml" "-$DEPLOY_ID" bootstrap-admin-job.yaml
    expect_api_image "$WORK_DIR/bootstrap.yaml" "$K8S_DIR/bootstrap-admin-job.yaml"
    kubectl create -f "$WORK_DIR/bootstrap.yaml"
    log "waiting for Job $BOOTSTRAP_JOB (up to ${BOOTSTRAP_TIMEOUT}s)"
    local result=0
    wait_job "$BOOTSTRAP_JOB" "$BOOTSTRAP_TIMEOUT" || result=$?
    ((result == 0)) || die "administrator bootstrap $(job_failure "$BOOTSTRAP_JOB" "$result") — the rollout is blocked"
    log "administrator bootstrap Job $BOOTSTRAP_JOB Complete"
}

stage_rollout() {
    require_tools kubectl terraform
    validate_image
    use_cluster
    local files=$ROLLOUT_FILES images
    if [[ -f $K8S_DIR/configmap.yaml ]]; then
        files="configmap.yaml $files"
    fi
    # shellcheck disable=SC2086 # files is a list
    render "$WORK_DIR/app.yaml" "" $files
    expect_api_image "$WORK_DIR/app.yaml" "$K8S_DIR/deployment.yaml"
    apply_server_side -f "$WORK_DIR/app.yaml"
    if ! kc rollout status "deployment/$DEPLOYMENT" --timeout="${ROLLOUT_TIMEOUT}s"; then
        kc get pods -o wide 2>&1 || true
        die "Deployment $DEPLOYMENT did not finish its rollout within ${ROLLOUT_TIMEOUT}s"
    fi
    images=$(kc get deployment "$DEPLOYMENT" -o jsonpath='{.spec.template.spec.containers[*].image}')
    [[ " $images " == *" $IMAGE "* ]] || die "Deployment $DEPLOYMENT does not run $IMAGE (found: $images)"
    log "Deployment $DEPLOYMENT rolled out with $IMAGE"
}

stage_smoke() {
    require_tools kubectl terraform curl
    require_env ADMIN_EMAIL ADMIN_PASSWORD
    use_cluster
    mkdir -p "$EVIDENCE_DIR"
    # kubectl itself, not the kc function: $! must be kubectl's pid for on_exit to stop it.
    kubectl --namespace "$NAMESPACE" port-forward "svc/$SERVICE" "$SMOKE_LOCAL_PORT:8080" \
        >"$WORK_DIR/port-forward.log" 2>&1 &
    PORT_FORWARD_PID=$!
    # deploy-smoke.sh retries /health while the port-forward comes up.
    SMOKE_BASE_URL="http://127.0.0.1:$SMOKE_LOCAL_PORT" SMOKE_EMAIL=$ADMIN_EMAIL \
        SMOKE_PASSWORD=$ADMIN_PASSWORD scripts/deploy-smoke.sh 2>&1 | tee "$EVIDENCE_DIR/smoke.txt"
}

# Collects what happened, sanitizes it, and refuses to leave evidence behind if a
# credential survived. A missing cluster (a run that failed early) is reported, not fatal.
stage_evidence() {
    mkdir -p "$EVIDENCE_DIR"
    local dir=$EVIDENCE_DIR reachable=false path selector job file leaks
    if path=$(tf_output kubeconfig_path) && [[ -f $path ]]; then
        export KUBECONFIG=$path
        kubectl version --request-timeout=10s >/dev/null 2>&1 && reachable=true
    fi

    if $reachable; then
        {
            kubectl get nodes -o wide
            echo
            kc get jobs,pods,deployments,replicasets,services,hpa,pvc,configmaps,statefulsets -o wide
            echo
            echo "Secrets (names and types only):"
            kc get secrets -o custom-columns=NAME:.metadata.name,TYPE:.type
        } >"$dir/cluster.txt" 2>&1 || true
        for job in "$MIGRATION_JOB:migration-job.txt" "$BOOTSTRAP_JOB:bootstrap-job.txt"; do
            {
                kc describe job "${job%%:*}" && echo && echo "--- logs" &&
                    kc logs "job/${job%%:*}" --all-containers --tail=500
            } >"$dir/${job#*:}" 2>&1 || true
        done
        {
            kc rollout status "deployment/$DEPLOYMENT" --timeout=10s
            echo
            kc rollout history "deployment/$DEPLOYMENT"
            echo "pod template images: $(kc get deployment "$DEPLOYMENT" -o jsonpath='{.spec.template.spec.containers[*].image}')"
            echo "running pods (name, resolved image id):"
            kc get pods -l "$(api_selector)" -o jsonpath='{range .items[*]}{.metadata.name}{"  "}{.status.containerStatuses[*].imageID}{"\n"}{end}'
        } >"$dir/rollout.txt" 2>&1 || true
        if selector=$(api_selector) && [[ -n $selector ]]; then
            kc logs -l "$selector" --all-containers --prefix --tail=300 >"$dir/api-logs.txt" 2>&1 || true
        fi
        kc get events --sort-by=.lastTimestamp >"$dir/events.txt" 2>&1 || true
    else
        echo "cluster not reachable (not created, or already destroyed) - nothing collected from it" >"$dir/cluster.txt"
    fi

    write_summary "$reachable"
    for file in "$dir"/*; do
        [[ -f $file ]] && sanitize_file "$file"
    done
    leaks=$(leaked_credentials "$dir" | sort -u)
    if [[ -n $leaks ]]; then
        rm -rf "$dir"
        die "a credential survived sanitizing (in: $(echo "$leaks" | xargs -n1 basename | tr '\n' ' ')) — evidence withheld"
    fi
    log "evidence in $dir"
}

# The API pods' label selector, from the Deployment (FP-34 owns the labels).
api_selector() {
    kc get deployment "$DEPLOYMENT" -o json 2>/dev/null |
        jq -r '.spec.selector.matchLabels // {} | to_entries | map("\(.key)=\(.value)") | join(",")'
}

job_state() {
    kc get job "$1" -o jsonpath='{range .status.conditions[?(@.status=="True")]}{.type}{" "}{end}' 2>/dev/null ||
        echo "not created"
}

# The GitHub Actions run, when there is one.
run_url() {
    if [[ -n ${GITHUB_RUN_ID:-} ]]; then
        printf '%s' "${GITHUB_SERVER_URL:-https://github.com}/${GITHUB_REPOSITORY:-}/actions/runs/$GITHUB_RUN_ID"
    fi
}

write_summary() {
    local reachable=$1 run_url deployment="none (no rollout)" image_ids="" summary=$EVIDENCE_DIR/summary.md
    run_url=$(run_url)
    if $reachable; then
        if kc get deployment "$DEPLOYMENT" >/dev/null 2>&1; then
            deployment=$(kc get deployment "$DEPLOYMENT" -o jsonpath='{.status.readyReplicas}/{.spec.replicas} ready, image {.spec.template.spec.containers[*].image}' 2>/dev/null)
            image_ids=$(kc get pods -l "$(api_selector)" -o jsonpath='{.items[*].status.containerStatuses[*].imageID}' 2>/dev/null |
                tr ' ' '\n' | sort -u | tr '\n' ' ')
        fi
    fi
    {
        echo "# Deploy evidence - \`$DEPLOY_ID\`"
        echo
        echo "| Item | Value |"
        echo "| --- | --- |"
        echo "| Environment | \`$DEPLOY_ID\` (ephemeral: $DEPLOY_EPHEMERAL) |"
        echo "| Commit | \`$DEPLOY_COMMIT\` |"
        echo "| Image (deployed reference) | \`${IMAGE:-not set}\` |"
        echo "| Image id resolved by the API pods | \`${image_ids:-none}\` |"
        echo "| Simulated migration failure | ${SIMULATE_MIGRATION_FAILURE:-false} |"
        if $reachable; then
            echo "| Migration Job | \`$MIGRATION_JOB\`: $(job_state "$MIGRATION_JOB") |"
            echo "| Bootstrap Job | \`$BOOTSTRAP_JOB\`: $(job_state "$BOOTSTRAP_JOB") |"
            echo "| Deployment | $deployment |"
        else
            echo "| Cluster | not reachable |"
        fi
        if [[ -n $run_url ]]; then
            echo "| Run | $run_url |"
        fi
        echo "| Collected at | $(date -u +%Y-%m-%dT%H:%M:%SZ) |"
        echo
        echo "## Stages"
        echo
        echo "A stage missing below did not run."
        echo
        echo "| Stage | Result | Duration |"
        echo "| --- | --- | --- |"
        if [[ -f $EVIDENCE_DIR/stages.tsv ]]; then
            awk -F '\t' '{ printf "| %s | %s | %s |\n", $1, $2, $3 }' "$EVIDENCE_DIR/stages.tsv"
        fi
        if [[ -f $EVIDENCE_DIR/smoke.txt ]]; then
            echo
            echo "## Smoke test"
            echo
            echo '```'
            cat "$EVIDENCE_DIR/smoke.txt"
            echo '```'
        fi
    } >"$summary"
}

stage_destroy() {
    if ! compgen -G "$INFRA_DIR/*.tf" >/dev/null; then
        log "no $INFRA_DIR/*.tf — nothing to destroy"
        return 0
    fi
    require_tools terraform jq
    tf_init
    if [[ -z $(tf state list 2>/dev/null) ]]; then
        log "no Terraform state — nothing to destroy"
        return 0
    fi
    if [[ $DEPLOY_EPHEMERAL != true && ${DEPLOY_CONFIRM_DESTROY:-} != yes ]]; then
        die "this destroys the cluster and its database — run again with DEPLOY_CONFIRM_DESTROY=yes"
    fi
    require_env POSTGRES_PASSWORD
    export TF_VAR_postgres_password=$POSTGRES_PASSWORD
    tf destroy -auto-approve -input=false -json | tf_messages
}

# The demonstration environment: every stage, each in its own process like the CI steps.
# Evidence is collected whatever happens; nothing is destroyed.
stage_up() {
    local stage
    for stage in check infra config migrate bootstrap rollout smoke; do
        if ! "$SELF" "$stage"; then
            "$SELF" evidence || true
            die "stage $stage failed — evidence in $EVIDENCE_DIR"
        fi
    done
    "$SELF" evidence
    log "environment $DEPLOY_ID is up (it stays up). Reach the API with:"
    log "  KUBECONFIG=$(tf_output kubeconfig_path) kubectl -n $NAMESPACE port-forward svc/$SERVICE 8080:8080"
}

case ${1:-} in
check | infra | config | migrate | bootstrap | rollout | smoke | evidence | destroy)
    run_stage "$1"
    ;;
up)
    STAGE=""
    stage_up
    ;;
*) usage ;;
esac
