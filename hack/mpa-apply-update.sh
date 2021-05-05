#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT=$(dirname ${BASH_SOURCE})/..

"$SCRIPT_ROOT"/hack/vpa-process-yamls.sh apply "$@"
