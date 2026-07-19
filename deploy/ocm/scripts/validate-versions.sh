#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

require_command awk

MATRIX_FILE="${REPO_ROOT}/config/certification/versions.yaml"

yaml_scalar() {
  local path="$1"

  awk -v wanted="${path}" '
    function trim(value) {
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
      return value
    }

    /^[[:space:]]*($|#)/ {
      next
    }

    {
      match($0, /^ */)
      indent = RLENGTH
      line = substr($0, indent + 1)
      separator = index(line, ":")
      if (separator == 0) {
        next
      }

      key = trim(substr(line, 1, separator - 1))
      level = int(indent / 2)
      keys[level] = key
      for (i = level + 1; i < 16; i++) {
        delete keys[i]
      }

      value = trim(substr(line, separator + 1))
      if (value == "") {
        next
      }

      full_path = keys[0]
      for (i = 1; i <= level; i++) {
        full_path = full_path "." keys[i]
      }

      if (full_path == wanted) {
        sub(/^"/, "", value)
        sub(/"$/, "", value)
        print value
        exit
      }
    }
  ' "${MATRIX_FILE}"
}

assert_matrix_value() {
  local path="$1"
  local expected="$2"
  local actual

  actual="$(yaml_scalar "${path}")"
  if [[ "${actual}" != "${expected}" ]]; then
    echo "version matrix mismatch for ${path}: expected ${expected}, got ${actual:-<missing>}" >&2
    return 1
  fi
}

assert_matrix_value "kubernetes.github_actions.version" "${KUBERNETES_VERSION#v}"
assert_matrix_value "kubernetes.github_actions.kind_version" "${KIND_VERSION#v}"
assert_matrix_value "kubernetes.github_actions.node_image" "${KIND_NODE_IMAGE}"
assert_matrix_value "kubernetes.github_actions.cluster_signing_duration" "${HUB_CLUSTER_SIGNING_DURATION}"
assert_matrix_value "components.open_cluster_management.release" "${OCM_VERSION}"
assert_matrix_value "components.clusteradm.release" "${CLUSTERADM_VERSION}"
assert_matrix_value "tools.kubectl.release" "${KUBECTL_VERSION}"
assert_matrix_value "tools.helm.release" "${HELM_VERSION}"

echo "certification matrix and OCM execution versions are consistent"
