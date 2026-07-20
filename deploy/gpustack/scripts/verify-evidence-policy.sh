#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
# shellcheck source=../versions.env
source "${DEPLOY_DIR}/versions.env"

ARTIFACT_DIR="${1:-${ARTIFACT_DIR:-${GITHUB_WORKSPACE:-$(pwd)}/artifacts/gpustack-baseline}}"
mode="${2:-full}"
case "${mode}" in
  full | partial) ;;
  *)
    echo "evidence policy mode must be full or partial: ${mode}" >&2
    exit 2
    ;;
esac

mkdir -p "${ARTIFACT_DIR}"

violations=()
add_violation() {
  violations+=("$1")
}

required_files=(
  ci-baseline.log
  provenance.json
  dependency-audit.json
  installed-packages.txt
  probe-results.json
  api-surface.json
  collection-summary.json
  user-before-restart.json
  user-after-restart.json
  persistence-validation.json
  baseline-assertions.json
  cleanup-summary.json
)

if [[ "${mode}" == "full" ]]; then
  for file_name in "${required_files[@]}"; do
    if [[ ! -s "${ARTIFACT_DIR}/${file_name}" ]]; then
      add_violation "missing or empty required artifact: ${file_name}"
    fi
  done
elif [[ ! -s "${ARTIFACT_DIR}/ci-baseline.log" ]]; then
  add_violation "missing or empty partial-run log: ci-baseline.log"
fi

while IFS= read -r file_path; do
  base_name="$(basename "${file_path}")"
  case "${base_name}" in
    *.json | *.txt | *.log) ;;
    *) add_violation "artifact has a disallowed file type: ${base_name}" ;;
  esac
  if [[ "${base_name}" =~ (cookie|credential|private[-_]?key|access[-_]?token|password|secret[-_]?value|env[-_]?dump) ]]; then
    add_violation "artifact filename is credential-like: ${base_name}"
  fi
done < <(find "${ARTIFACT_DIR}" -type f ! -name evidence-sha256.txt ! -name evidence-policy-report.json | sort)

while IFS= read -r json_file; do
  if ! jq empty "${json_file}" >/dev/null 2>&1; then
    add_violation "invalid JSON: $(basename "${json_file}")"
  fi
done < <(find "${ARTIFACT_DIR}" -type f -name '*.json' ! -name evidence-policy-report.json | sort)

if grep -RIlE --exclude=evidence-policy-report.json --exclude=evidence-sha256.txt \
  -e '-----BEGIN ([A-Z ]+ )?PRIVATE KEY-----|ci_only_gpustack_admin_password|ci_only_gpustack_database_password|postgresql://|GPUSTACK_(BOOTSTRAP_PASSWORD|DATABASE_URL)|(^|[[:space:]])Authorization:|(^|[[:space:]])Bearer[[:space:]]|Set-Cookie:|eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+|/home/runner|[A-Za-z]:\\Users\\|/var/lib/(docker|containerd)' \
  "${ARTIFACT_DIR}" >/dev/null 2>&1; then
  add_violation "artifact content includes a forbidden credential, token, runner path, or raw container-storage field"
fi

if [[ -s "${ARTIFACT_DIR}/installed-packages.txt" ]]; then
  if grep -Ev '^[A-Za-z0-9_.-]+==[^[:space:]]+$' "${ARTIFACT_DIR}/installed-packages.txt" >/dev/null; then
    add_violation "installed package evidence includes an unversioned or path-based entry"
  fi
  if ! grep -Fxq "gpustack==${GPUSTACK_VERSION}" "${ARTIFACT_DIR}/installed-packages.txt"; then
    add_violation "installed package evidence does not contain the pinned GPUStack version"
  fi
fi

check_json() {
  local file_name="$1"
  local expression="$2"
  local description="$3"
  if [[ -s "${ARTIFACT_DIR}/${file_name}" ]] && ! jq -e "${expression}" "${ARTIFACT_DIR}/${file_name}" >/dev/null 2>&1; then
    add_violation "${description}: ${file_name}"
  fi
}

if [[ "${mode}" == "full" ]]; then
  check_json provenance.json \
    ".status == \"passed\" and .gpustack.version == \"${GPUSTACK_VERSION}\" and .gpustack.tag == \"${GPUSTACK_TAG}\" and .gpustack.commit == \"${GPUSTACK_COMMIT}\" and .gpustack.wheelSHA256 == \"${GPUSTACK_WHEEL_SHA256}\" and .gpustack.upstreamLockSHA256 == \"${GPUSTACK_UPSTREAM_LOCK_SHA256}\" and .gpustack.requirementsSHA256 == \"${GPUSTACK_REQUIREMENTS_SHA256}\" and .profile.gateway == \"disabled\" and .profile.builtinObservability == false and .profile.embeddedWorker == false and .profile.externalDatabase == true" \
    "release provenance failed"
  check_json dependency-audit.json \
    ".status == \"passed\" and .maxPackageBytes == ${GPUSTACK_MAX_PACKAGE_BYTES} and .oversizedFiles == 0 and .largestCachedFile.bytes <= ${GPUSTACK_MAX_PACKAGE_BYTES} and .installedPackages >= 100" \
    "dependency size policy failed"
  check_json probe-results.json \
    ".status == \"passed\" and .healthz == \"ok\" and .readyz == \"ok\" and (.version == \"${GPUSTACK_VERSION}\" or .version == \"${GPUSTACK_TAG}\")" \
    "probe validation failed"
  check_json api-surface.json \
    '.status == "passed" and (.gs04.clusters | index("get")) != null and (.gs04.clusters | index("post")) != null and (.gs07.instances | index("get")) != null and (.gs07.instances | index("post")) != null and (.gs07.instance | index("delete")) != null and (.gs07.stop | index("put")) != null and (.gs07.start | index("put")) != null and (.gs08.sshPublicKeys | index("get")) != null and (.gs09.persistentVolumes | index("get")) != null and (.gs10.usageMeta | index("get")) != null' \
    "GS API surface validation failed"
  check_json collection-summary.json \
    '.status == "passed" and (.clusters.topLevelKeys | type) == "array" and (.gpuInstances.topLevelKeys | type) == "array" and (.sshPublicKeys.topLevelKeys | type) == "array" and (.persistentVolumes.topLevelKeys | type) == "array" and (.usageMeta.topLevelKeys | type) == "array"' \
    "collection access validation failed"
  check_json user-before-restart.json \
    '.name == "admin" and .id != null and .isAdmin == true and .isActive == true' \
    "initial admin validation failed"
  check_json user-after-restart.json \
    '.name == "admin" and .id != null and .isAdmin == true and .isActive == true' \
    "post-restart admin validation failed"
  check_json persistence-validation.json \
    '.status == "passed" and .userIDStable == true and .loginAfterRestart == true and .readyAfterRestart == true and .beforeUserID == .afterUserID' \
    "restart persistence validation failed"
  check_json baseline-assertions.json \
    '.status == "passed" and (.checks | length) == 7 and all(.checks[]; . == "passed")' \
    "baseline assertion aggregate failed"
  check_json cleanup-summary.json \
    '.status == "passed" and .processStopped == true and .serverStarts == 2' \
    "cleanup validation failed"
fi

violation_file="${ARTIFACT_DIR}/.evidence-policy-violations.jsonl"
: >"${violation_file}"
for violation in "${violations[@]}"; do
  jq -cn --arg violation "${violation}" '$violation' >>"${violation_file}"
done

checked_files="$(find "${ARTIFACT_DIR}" -type f ! -name evidence-sha256.txt ! -name evidence-policy-report.json ! -name .evidence-policy-violations.jsonl | awk 'END {print NR + 0}')"
jq -s \
  --arg generatedAt "$(date -u +'%Y-%m-%dT%H:%M:%SZ')" \
  --arg mode "${mode}" \
  --argjson checkedFiles "${checked_files}" \
  '{status:(if length == 0 then "passed" else "failed" end),mode:$mode,generatedAt:$generatedAt,checkedFiles:$checkedFiles,violations:.}' \
  "${violation_file}" >"${ARTIFACT_DIR}/evidence-policy-report.json"
rm -f "${violation_file}"

(
  cd "${ARTIFACT_DIR}"
  find . -type f ! -name evidence-sha256.txt -print0 | sort -z | xargs -0 sha256sum >evidence-sha256.txt
)

if [[ "${#violations[@]}" -ne 0 ]]; then
  jq -r '.violations[]' "${ARTIFACT_DIR}/evidence-policy-report.json" >&2
  exit 1
fi

echo "GPUStack evidence policy passed: ${ARTIFACT_DIR}/evidence-policy-report.json"
