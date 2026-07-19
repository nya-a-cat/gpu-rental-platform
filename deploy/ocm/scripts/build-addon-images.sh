#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

require_command docker
require_command git
require_command jq
require_command mktemp
require_command tar

current_revision="${CURRENT_REVISION:-$(git -C "${REPO_ROOT}" rev-parse HEAD)}"
legacy_revision="${GPU_PLATFORM_ADDON_N_MINUS_ONE_REVISION}"
current_image="${GPU_PLATFORM_ADDON_IMAGE_REPOSITORY}:${current_revision}"
legacy_image="${GPU_PLATFORM_ADDON_IMAGE_REPOSITORY}:${legacy_revision}"
current_tree="$(git -C "${REPO_ROOT}" rev-parse "${current_revision}:apps/gpu-platform-addon")"
legacy_tree="$(git -C "${REPO_ROOT}" rev-parse "${legacy_revision}:apps/gpu-platform-addon")"

if [[ "${current_tree}" == "${legacy_tree}" ]]; then
  echo "current and N-1 add-on source trees are identical: ${current_tree}" >&2
  exit 1
fi

workspace="$(mktemp -d "${RUNNER_TEMP:-/tmp}/gpu-platform-addon-n-minus-one.XXXXXX")"
trap 'rm -rf -- "${workspace}"' EXIT
git -C "${REPO_ROOT}" archive "${legacy_revision}" | tar -x -C "${workspace}"

build_image() {
  local source_root="$1"
  local image="$2"
  local version="$3"
  local revision="$4"

  local build_args=(
    --file "${source_root}/infra/gpu-platform-addon.Dockerfile"
    --tag "${image}"
    --label "org.opencontainers.image.source=https://github.com/nya-a-cat/gpu-rental-platform"
    --label "org.opencontainers.image.version=${version}"
    --label "org.opencontainers.image.revision=${revision}"
  )
  docker build "${build_args[@]}" "${source_root}"
}

build_image "${workspace}" "${legacy_image}" "${GPU_PLATFORM_ADDON_N_MINUS_ONE_VERSION}" "${legacy_revision}"
build_image "${REPO_ROOT}" "${current_image}" "${GPU_PLATFORM_ADDON_VERSION}" "${current_revision}"

mkdir -p "${ARTIFACT_DIR}"
jq_args=(
  --arg current_revision "${current_revision}"
  --arg current_tree "${current_tree}"
  --arg current_image "${current_image}"
  --arg current_version "${GPU_PLATFORM_ADDON_VERSION}"
  --arg legacy_revision "${legacy_revision}"
  --arg legacy_tree "${legacy_tree}"
  --arg legacy_image "${legacy_image}"
  --arg legacy_version "${GPU_PLATFORM_ADDON_N_MINUS_ONE_VERSION}"
)
docker image inspect "${legacy_image}" "${current_image}" | jq "${jq_args[@]}" '
  {
    current: {
      version: $current_version,
      revision: $current_revision,
      sourceTree: $current_tree,
      image: $current_image
    },
    nMinusOne: {
      version: $legacy_version,
      revision: $legacy_revision,
      sourceTree: $legacy_tree,
      image: $legacy_image
    },
    images: [
      .[]
      | {
          id: .Id,
          repoTags: .RepoTags,
          created: .Created,
          size: .Size,
          os: .Os,
          architecture: .Architecture,
          labels: {
            source: .Config.Labels["org.opencontainers.image.source"],
            version: .Config.Labels["org.opencontainers.image.version"],
            revision: .Config.Labels["org.opencontainers.image.revision"]
          }
        }
    ]
  }
' >"${ARTIFACT_DIR}/image-provenance.json"

if [[ -n "${GITHUB_ENV:-}" ]]; then
  {
    echo "ADDON_CURRENT_IMAGE=${current_image}"
    echo "ADDON_N_MINUS_ONE_IMAGE=${legacy_image}"
  } >>"${GITHUB_ENV}"
fi

echo "built GPU Platform Add-on ${legacy_image} and ${current_image}"
