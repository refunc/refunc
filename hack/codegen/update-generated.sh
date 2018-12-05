#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

vendor/k8s.io/code-generator/generate-groups.sh \
  "all" \
  "github.com/refunc/refunc/pkg/generated" \
  "github.com/refunc/refunc/pkg/apis" \
  "refunc:v1beta3" \
  --go-header-file "./hack/codegen/boilerplate.go.txt" \
  $@