#!/bin/bash

#
# testing using
# $ ./agent -k key --v 3 -l :7788
# $ cat pkg/runtime/legacy/load.sh| ssh setup@127.0.0.1 -Tp 7788
# $ echo "Hello\n" | ssh foo@127.0.0.1 -Tp 7788
#

# make fail on error
set -e
set -o errexit
set -o pipefail

WORK_DIR=${REFUNC_DIR:-$(pwd)}
FDIR=${REFUNC_ROOT_DIR:-${WORK_DIR}/root}
echo "working dir: ${WORK_DIR}, refunc dir: ${FDIR}"

rm -rf ${FDIR} 2>/dev/null || true
mkdir -p ${FDIR} 2>/dev/null || true

base64-d () {
  case $(uname) in
    Darwin) base64 -D "$@";;
    *)      base64 -d "$@";;
  esac
}

mybase64 () {
  case $(uname) in
    Darwin) base64 "$@";;
    *)      base64 -w 0 "$@";;
  esac
}

REFUNCJSON=$(cat <<EOF | mybase64
{
  "metadata":{
    "name":"example",
    "namespace":"ci-test"
  },
  "spec":{
    "hash":"e106d4bbe01711ac0eeda84840d853db",
    "entry":"fn.sh arg1",
    "maxReplicas":1,
    "runtime":{
      "name":"shell",
      "envs":{
        "REFUNC_MINIO_BUCKET":"formicary",
        "REFUNC_MINIO_ENDPOINT":"10.43.99.201"
      },
      "timeout":9,
      "credentials":{
        "accessKey":"G932NWFIDH5767K7VOBE",
        "secretKey":"pyerCt7E+/qooxjEqEnVYOCDz9Khl4svLxr8HTRO",
        "token":"eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJycGMtZXhhbXBsZS1kZzZkdyIsInBlcm1pc3Npb25zIjp7InB1Ymxpc2giOlsicmVmdW5jLiouKi5cdTAwM2UiLCJfSU5CT1guKi4qIl0sInN1YnNjcmliZSI6WyJyZWZ1bmMuKi4qLmV2ZW50cy5cdTAwM2UiLCJfSU5CT1guKi4qIl19fQ._d0z72tjNGcu7F9pCwEGvDKSARd3YXVhD3wk4qYuxXOP-hbJNFGyPIiCuaGvG0G7NokePeC77ADVgYbILxZoPA",
        "scope":"refunc/ci-test/example/data/"
      }
    }
  }
}
EOF
)

echo creating refunc.json
echo ${REFUNCJSON} | base64-d >${WORK_DIR}/refunc.json

REFUNCDATA=$(cat <<EOF | mybase64
#!/bin/bash
echo "args: \${@:1}"
export | grep REFUNC_

# read all lines in lines array
while IFS= read -r line; do
    echo ">> \${line}"
done

rm ${WORK_DIR}/refunc.json ${FDIR}/fn.sh
echo "done"
EOF
)

echo creating refunc
echo ${REFUNCDATA} | base64-d >${FDIR}/fn.sh
chmod u+x ${FDIR}/fn.sh
echo "done"
