#!/bin/sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(realpath ${SCRIPT_DIR}/..)"
BIN_FOLDER="${SCRIPT_DIR}/.bin"
IN_CURRENT_SESSION=${IN_CURRENT_SESSION:-"false"}

[ "$#" -ne 1 ] && [ "$1" != "static" ] && [ "$1" != "dynamic" ] && { echo "Which demo do you want to start? You can chose between 'static' and 'dynamic'. Please execute the script passing the name of the demo."; exit 1; }
DEMO=${1}

# Ensure dependencies
if ! command -v go > /dev/null; then
  echo "Go is not installed. Please install it before running this script"
  exit 1
fi

# Install smug
mkdir -p "${BIN_FOLDER}"
GOBIN="${BIN_FOLDER}" go install github.com/ivaaaan/smug@latest

SMUG_ARGS=( )
if [[ "${IN_CURRENT_SESSION}" == "true" ]]; then
  SMUG_ARGS += "--inside-current-session"
fi

# Use smug to setup the demo environment
"${BIN_FOLDER}/smug" \
  -f "${SCRIPT_DIR}/${DEMO}/smug.yaml" \
  "${SMUG_ARGS}" \
  root_path="${ROOT_DIR}"
