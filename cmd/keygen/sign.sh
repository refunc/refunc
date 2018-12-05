#/bin/bash

set -e
SCRIPT_ROOT=$(cd $(dirname $0); pwd)

get_abs_filename() {
  # $1 : relative filename
  echo "$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
}

pushd ${SCRIPT_ROOT}/../.. >/dev/null
source ./hack/scripts/common
export ECDSA_PRIVATEKEY_FILE=$(get_abs_filename ${ECDSA_PRIVATEKEY_FILE})
popd >/dev/null

pushd ${SCRIPT_ROOT} >/dev/null
go run main.go -sign "$@"
popd >/dev/null
