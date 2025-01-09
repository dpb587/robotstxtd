#!/bin/bash

set -euo pipefail

EXPORTS_BUILD_GITREF="${EXPORTS_BUILD_GITREF:-unknown}"
EXPORTS_BUILD_GITSHA="${EXPORTS_BUILD_GITSHA:-00000000}"
EXPORTS_VERSION_NUMBER="${EXPORTS_VERSION_NUMBER:-0.0.0}"
EXPORTS_VERSION_LABELS="${EXPORTS_VERSION_LABELS:-}"

cln_task_dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cln_task_key="$( basename "${cln_task_dir}" )"

cln_repo_dir="$( cd "${cln_task_dir}/../.." && pwd )"
cln_repo_key="$( basename "${cln_repo_dir}" )"
cln_repo_name="robotstxtd"

cd "${cln_repo_dir}"

cln_exports_dir="${1:-${cln_repo_dir}/tmp/exports}"
cln_exports_artifacts_dir="${cln_exports_dir}/artifacts/robotstxtd-image"

# task

image_name="robotstxtd"

if [[ "${EXPORTS_VERSION_NUMBER}" == "0.0.0" ]]
then
  image_repo="us-docker.pkg.dev/master-scanner-387914/private-docker-dev/${cln_repo_name}/${image_name}"
  image_tag_version="${EXPORTS_BUILD_GITSHA}"
else
  image_repo="us-docker.pkg.dev/master-scanner-387914/private-docker-release/${cln_repo_name}/${image_name}"
  image_tag_version="${EXPORTS_VERSION_NUMBER}${EXPORTS_VERSION_LABELS%+*}"
fi

rm -fr "${cln_exports_artifacts_dir}"
mkdir -p "${cln_exports_artifacts_dir}/docker-image"

docker build \
  --tag "${image_repo}" \
  --file "build/docker/robotstxtd/Dockerfile" \
  .

for tag in "${image_tag_version}"
do
  [ ! -z "${tag}" ] || continue

  docker tag "${image_repo}" "${image_repo}:${tag}"

  echo "${image_repo}:${tag}" \
    | tee -a "${cln_exports_artifacts_dir}/docker-image/refs"
done