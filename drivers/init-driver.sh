#!/bin/bash

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage() {
  printf "Usage: %s <DRIVER NAME>\n\n%s\n" "${0}" "${1}"
  exit 1
}

[[ "$#" -ne "1" ]] && usage "Please provide the name of the new driver you want to initialize"

DRIVER_NAME="${1}"
DRIVER_FOLDER="${SCRIPT_DIR}/${DRIVER_NAME}"

# Create driver folder
[[ -d "${DRIVER_FOLDER}" ]] && { printf "The folder '%s' already exists\n" "${DRIVER_FOLDER}"; exit 1; }
mkdir "${DRIVER_FOLDER}"
cd "${DRIVER_FOLDER}"

# Create the new Go Project
go mod init "github.com/konflux-ci/may/drivers/${DRIVER_NAME}"

# Use KubeBuilder to init the project
kubebuilder init --domain may.konflux-ci.dev --license apache2 --project-name "driver-${DRIVER_NAME}"

# Add local replace to may's APIs
go mod edit -replace github.com/konflux-ci/may=../../may
go mod tidy

# Add controllers for StaticHost and DynamicHost
kubebuilder create api \
        --group may.konflux-ci.dev --version v1alpha1 --kind StaticHost \
        --controller=true --resource=false \
        --external-api-path=github.com/konflux-ci/may/api/v1alpha1 \
        --external-api-domain=konflux-ci.dev \
        --external-api-module=github.com/konflux-ci/may
kubebuilder create api \
        --group may.konflux-ci.dev --version v1alpha1 --kind DynamicHost \
        --controller=true --resource=false \
        --external-api-path=github.com/konflux-ci/may/api/v1alpha1 \
        --external-api-domain=konflux-ci.dev \
        --external-api-module=github.com/konflux-ci/may

# Generate Code and Manifests
make generate manifests
