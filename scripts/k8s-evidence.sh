#!/usr/bin/env bash
# Evidence for FP-34 (/k8s) and FP-35 (/infra) — specs/kubernetes-deployment/design.md §7.
#
# Usage: scripts/k8s-evidence.sh <command>
#
#   status          nodes, metrics, every object in the namespace, HPA description
#   hpa-load        in-cluster load through the API Service (k8s/load-test/), sampling
#                   replicas, HPA metrics and `kubectl top` before, during and after it;
#                   fails when the HPA never scaled up
#   pod-recovery    deletes one ready API pod and waits for its replacement; fails when
#                   the Service ran out of ready endpoints or no replacement became ready
#   db-persistence  writes a marker to a separate database (never the application one),
#                   deletes postgres-0, reads the marker back. Causes a short database
#                   outage: do not run it while recording other flows
#
# Environment (all optional):
#   KUBECONFIG         default: `terraform -chdir=infra output -raw kubeconfig_path`
#   EVIDENCE_DIR       default: evidence (git-ignored); each run writes <command>-<UTC time>/
#   LOAD_WORKERS       hpa-load: parallel load pods (4)
#   LOAD_DURATION      hpa-load: seconds of load per worker (240)
#   COOLDOWN_SECONDS   hpa-load: sampling after the load stops (300)
#   SAMPLE_INTERVAL    hpa-load: seconds between samples (15)
#   RECOVERY_TIMEOUT   pod-recovery / db-persistence: seconds to wait (180)
#
# Requires kubectl and jq (terraform only when KUBECONFIG is unset). Reads no Secret: the
# output holds object names, counts and metrics only.
set -euo pipefail

cd "$(dirname "$0")/.."

NAMESPACE=automotive-workshop
DEPLOYMENT=automotive-workshop-api
SERVICE=automotive-workshop-api
API_SELECTOR=app.kubernetes.io/name=automotive-workshop-api,app.kubernetes.io/component=api
LOAD_MANIFEST=k8s/load-test/load-generator-job.yaml
LOAD_JOB_BASE=automotive-workshop-load-generator
CHECK_DATABASE=fp35_persistence_check

LOAD_WORKERS=${LOAD_WORKERS:-4}
LOAD_DURATION=${LOAD_DURATION:-240}
COOLDOWN_SECONDS=${COOLDOWN_SECONDS:-300}
SAMPLE_INTERVAL=${SAMPLE_INTERVAL:-15}
RECOVERY_TIMEOUT=${RECOVERY_TIMEOUT:-180}
RUN_ID=$(date -u +%Y%m%dt%H%M%Sz)

log() { echo ">>> $*" >&2; }
die() {
    echo "error: $*" >&2
    exit 1
}
usage() {
    sed -n '2,/^set -euo/p' "$0" | sed '$d; s/^# \{0,1\}//' >&2
    exit 2
}
kc() { kubectl --namespace "$NAMESPACE" "$@"; }
now() { date -u +%Y-%m-%dT%H:%M:%SZ; }

require_tools() {
    local tool
    for tool in "$@"; do
        command -v "$tool" >/dev/null 2>&1 || die "missing tool: $tool"
    done
}

# The cluster Terraform created (infra/), never whatever context happens to be current.
use_cluster() {
    if [[ -z ${KUBECONFIG:-} ]]; then
        require_tools terraform
        KUBECONFIG=$(terraform -chdir=infra output -raw kubeconfig_path 2>/dev/null) ||
            die "no KUBECONFIG and no Terraform output kubeconfig_path — run terraform apply in infra/ first"
        export KUBECONFIG
    fi
    [[ -f $KUBECONFIG ]] || die "kubeconfig $KUBECONFIG does not exist"
    kubectl get namespace "$NAMESPACE" >/dev/null 2>&1 ||
        die "namespace $NAMESPACE not found — is the infrastructure (infra/) applied?"
}

new_evidence_dir() {
    OUT=${EVIDENCE_DIR:-evidence}/$1-$RUN_ID
    mkdir -p "$OUT"
    log "evidence: $OUT"
}

# snapshot FILE TITLE — the namespace's objects and metrics at this moment.
snapshot() {
    {
        echo "## $2 — $(now)"
        echo
        kubectl get nodes -o wide 2>&1 || true
        echo
        kubectl top nodes 2>&1 || true
        echo
        kc top pods 2>&1 || true
        echo
        kc get deployments,replicasets,pods,services,hpa,jobs,statefulsets,pvc -o wide 2>&1 || true
        echo
    } >>"$1"
}

desired_replicas() { kc get deployment "$DEPLOYMENT" -o jsonpath='{.spec.replicas}'; }
ready_replicas() {
    local ready
    ready=$(kc get deployment "$DEPLOYMENT" -o jsonpath='{.status.readyReplicas}')
    echo "${ready:-0}"
}
ready_endpoints() {
    kc get endpointslices -l "kubernetes.io/service-name=$SERVICE" \
        -o jsonpath='{range .items[*].endpoints[*]}{.conditions.ready}{"\n"}{end}' |
        grep -c '^true$' || true
}

# --- status -----------------------------------------------------------------------------

cmd_status() {
    new_evidence_dir status
    snapshot "$OUT/status.txt" "status"
    kc describe hpa "$DEPLOYMENT" >>"$OUT/status.txt" 2>&1 || true
    cat "$OUT/status.txt"
}

# --- hpa-load ---------------------------------------------------------------------------

# One TSV row: time, phase, desired/ready replicas, HPA CPU/memory utilization, load pods.
sample() {
    local phase=$1 job=$2 hpa cpu memory active
    hpa=$(kc get hpa "$DEPLOYMENT" -o json)
    cpu=$(jq -r '[.status.currentMetrics[]? | select(.resource.name == "cpu") | .resource.current.averageUtilization][0] // "unknown"' <<<"$hpa")
    memory=$(jq -r '[.status.currentMetrics[]? | select(.resource.name == "memory") | .resource.current.averageUtilization][0] // "unknown"' <<<"$hpa")
    active=$(kc get job "$job" -o jsonpath='{.status.active}' 2>/dev/null || true)
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$(now)" "$phase" "$(desired_replicas)" "$(ready_replicas)" \
        "$cpu" "$memory" "${active:-0}" | tee -a "$OUT/timeline.tsv" >&2
    { echo "## $(now) ($phase)"; kc top pods -l "$API_SELECTOR" 2>&1 || true; echo; } >>"$OUT/top.txt"
}

job_finished() {
    kc get job "$1" -o jsonpath='{range .status.conditions[?(@.status=="True")]}{.type}{" "}{end}' 2>/dev/null |
        grep -qE '(^| )(Complete|Failed) '
}

cmd_hpa_load() {
    require_tools jq
    [[ $LOAD_WORKERS =~ ^[1-9][0-9]*$ && $LOAD_DURATION =~ ^[1-9][0-9]*$ ]] ||
        die "LOAD_WORKERS and LOAD_DURATION must be positive integers"
    local job=$LOAD_JOB_BASE-$RUN_ID start max final value deadline
    new_evidence_dir hpa-load
    snapshot "$OUT/before.txt" "before the load"
    kc describe hpa "$DEPLOYMENT" >>"$OUT/before.txt" 2>&1 || true
    start=$(desired_replicas)

    log "creating Job $job: $LOAD_WORKERS workers x ${LOAD_DURATION}s against svc/$SERVICE"
    kubectl create --dry-run=client -o json -f "$LOAD_MANIFEST" |
        jq --arg name "$job" --argjson workers "$LOAD_WORKERS" --arg duration "$LOAD_DURATION" '
            .metadata.name = $name
            | .spec.parallelism = $workers
            | .spec.completions = $workers
            | (.spec.template.spec.containers[0].env[] | select(.name == "DURATION_SECONDS") | .value) = $duration' |
        kc create -f - >/dev/null

    printf 'time\tphase\tdesired\tready\tcpu_pct\tmemory_pct\tload_pods\n' >"$OUT/timeline.tsv"
    deadline=$((SECONDS + LOAD_DURATION + 120))
    while ! job_finished "$job" && ((SECONDS < deadline)); do
        sample load "$job"
        sleep "$SAMPLE_INTERVAL"
    done
    snapshot "$OUT/during.txt" "end of the load"
    deadline=$((SECONDS + COOLDOWN_SECONDS))
    while ((SECONDS < deadline)); do
        sample cooldown "$job"
        sleep "$SAMPLE_INTERVAL"
    done
    snapshot "$OUT/after.txt" "after the cool-down"
    kc describe hpa "$DEPLOYMENT" >>"$OUT/after.txt" 2>&1 || true
    kc logs "job/$job" --all-containers --prefix >"$OUT/load-job.txt" 2>&1 || true

    max=$start
    while read -r value; do
        ((value > max)) && max=$value
    done < <(awk -F'\t' 'NR > 1 { print $3 }' "$OUT/timeline.tsv")
    final=$(desired_replicas)
    {
        echo "# HPA load evidence — $(now)"
        echo
        echo "| Item | Value |"
        echo "| --- | --- |"
        echo "| Load | $LOAD_WORKERS workers x ${LOAD_DURATION}s, Job \`$job\`, through \`svc/$SERVICE\` |"
        echo "| Replicas before | $start |"
        echo "| Max replicas during the run | $max |"
        echo "| Replicas after ${COOLDOWN_SECONDS}s of cool-down | $final |"
        echo "| Scaled up | $( ((max > start)) && echo yes || echo no) |"
        echo "| Scaled down after the load | $( ((final < max)) && echo yes || echo "not yet (scale-down window 120s, at most -50%/60s)") |"
        echo
        echo "Requests per worker: see load-job.txt. Timeline: timeline.tsv; metrics per sample: top.txt."
    } >"$OUT/summary.md"
    cat "$OUT/summary.md"
    ((max > start)) || die "the HPA did not scale up (replicas stayed at $start) — see $OUT"
}

# --- pod-recovery -----------------------------------------------------------------------

cmd_pod_recovery() {
    local victim desired min_endpoints endpoints ready deadline replacement="" start
    new_evidence_dir pod-recovery
    snapshot "$OUT/before.txt" "before the deletion"
    desired=$(desired_replicas)
    victim=$(kc get pods -l "$API_SELECTOR" \
        -o jsonpath='{range .items[?(@.status.phase=="Running")]}{.metadata.name}{"\n"}{end}' | head -n 1)
    [[ -n $victim ]] || die "no running API pod to delete"
    ((desired >= 2)) || log "warning: only $desired replica — the Service will have no endpoint while it is replaced"

    log "deleting pod $victim (desired replicas: $desired)"
    start=$SECONDS
    min_endpoints=$(ready_endpoints)
    printf 'seconds\tready_endpoints\tready_replicas\n' >"$OUT/timeline.tsv"
    kc delete pod "$victim" --wait=false >/dev/null
    deadline=$((SECONDS + RECOVERY_TIMEOUT))
    while ((SECONDS < deadline)); do
        endpoints=$(ready_endpoints)
        ready=$(ready_replicas)
        ((endpoints < min_endpoints)) && min_endpoints=$endpoints
        printf '%s\t%s\t%s\n' $((SECONDS - start)) "$endpoints" "$ready" >>"$OUT/timeline.tsv"
        if ! kc get pod "$victim" >/dev/null 2>&1 && ((ready >= desired)); then
            replacement=$(kc get pods -l "$API_SELECTOR" --sort-by=.metadata.creationTimestamp \
                -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' | tail -n 1)
            break
        fi
        sleep 1
    done
    snapshot "$OUT/after.txt" "after the replacement"
    {
        echo "# Pod recovery evidence — $(now)"
        echo
        echo "| Item | Value |"
        echo "| --- | --- |"
        echo "| Deleted pod | \`$victim\` |"
        echo "| Replacement | \`${replacement:-none}\` |"
        echo "| Time to ready replicas = $desired | $( [[ -n $replacement ]] && echo "$((SECONDS - start))s" || echo "timed out after ${RECOVERY_TIMEOUT}s") |"
        echo "| Fewest ready Service endpoints observed | $min_endpoints |"
    } >"$OUT/summary.md"
    cat "$OUT/summary.md"
    [[ -n $replacement ]] || die "no replacement became ready within ${RECOVERY_TIMEOUT}s — see $OUT"
    ((min_endpoints > 0)) || die "the Service had no ready endpoint during the replacement — see $OUT"
}

# --- db-persistence ---------------------------------------------------------------------

# psql inside postgres-0 over TCP with the pod's own credentials: nothing leaves the pod.
pod_psql() {
    local database=$1 sql=$2
    # shellcheck disable=SC2016 # expanded by the pod's shell, not this one
    kc exec postgres-0 -c postgres -- sh -c \
        'PGPASSWORD="$POSTGRES_PASSWORD" psql -v ON_ERROR_STOP=1 -qtA -h 127.0.0.1 -U "$POSTGRES_USER" -d "$1" -c "$2"' \
        psql "$database" "$sql"
}

cmd_db_persistence() {
    local marker uid_before uid_after read_back
    new_evidence_dir db-persistence
    marker=fp35-$RUN_ID-$RANDOM
    snapshot "$OUT/before.txt" "before deleting postgres-0"
    uid_before=$(kc get pod postgres-0 -o jsonpath='{.metadata.uid}')

    log "writing marker to database $CHECK_DATABASE (the application database is not touched)"
    pod_psql postgres "DROP DATABASE IF EXISTS $CHECK_DATABASE" >/dev/null
    pod_psql postgres "CREATE DATABASE $CHECK_DATABASE" >/dev/null
    pod_psql "$CHECK_DATABASE" "CREATE TABLE marker (value text NOT NULL, written_at timestamptz NOT NULL DEFAULT now()); INSERT INTO marker (value) VALUES ('$marker')" >/dev/null

    log "deleting pod postgres-0"
    kc delete pod postgres-0 --wait=true --timeout="${RECOVERY_TIMEOUT}s" >/dev/null
    # The StatefulSet recreates the pod under the same name; wait for the new one to exist.
    local deadline=$((SECONDS + RECOVERY_TIMEOUT))
    until uid_after=$(kc get pod postgres-0 -o jsonpath='{.metadata.uid}' 2>/dev/null) &&
        [[ -n $uid_after && $uid_after != "$uid_before" ]]; do
        ((SECONDS < deadline)) || die "postgres-0 was not recreated within ${RECOVERY_TIMEOUT}s"
        sleep 2
    done
    kc wait --for=condition=Ready pod/postgres-0 --timeout="${RECOVERY_TIMEOUT}s" >/dev/null
    read_back=$(pod_psql "$CHECK_DATABASE" "SELECT value FROM marker")
    pod_psql postgres "DROP DATABASE $CHECK_DATABASE" >/dev/null
    snapshot "$OUT/after.txt" "after postgres-0 was recreated"
    {
        echo "# Database persistence evidence — $(now)"
        echo
        echo "| Item | Value |"
        echo "| --- | --- |"
        echo "| Pod UID before / after | \`$uid_before\` / \`$uid_after\` |"
        echo "| Marker written | \`$marker\` |"
        echo "| Marker read after the restart | \`$read_back\` |"
        echo "| PVC | \`$(kc get pod postgres-0 -o jsonpath='{.spec.volumes[?(@.name=="data")].persistentVolumeClaim.claimName}')\` |"
        echo "| Data preserved | $([[ $read_back == "$marker" && $uid_before != "$uid_after" ]] && echo yes || echo NO) |"
    } >"$OUT/summary.md"
    cat "$OUT/summary.md"
    [[ $uid_before != "$uid_after" ]] || die "postgres-0 was not recreated"
    [[ $read_back == "$marker" ]] || die "the marker was not found after the restart — see $OUT"
}

# --- main -------------------------------------------------------------------------------

[[ $# -eq 1 ]] || usage
require_tools kubectl
case $1 in
status | hpa-load | pod-recovery | db-persistence)
    use_cluster
    "cmd_${1//-/_}"
    ;;
*) usage ;;
esac
