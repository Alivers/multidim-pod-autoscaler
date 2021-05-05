#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT=$(dirname "${BASH_SOURCE}")/..

function print_help {
  echo "ERROR! Usage: mpa-process-yamls.sh <action> [<component>]"
  echo "<action> should be either 'create', 'diff', 'print' or 'delete'."
  echo "The 'print' action will print all resources that would be used by, e.g., 'kubectl diff'."
  echo "<component> might be one of 'admission', 'updater', 'recommender'."
  echo "If <component> is set, only the deployment of that component will be processed,"
  echo "otherwise all components and configs will be processed."
}

if [ $# -eq 0 ]; then
  print_help
  exit 1
fi

if [ $# -gt 2 ]; then
  print_help
  exit 1
fi

ACTION=$1
COMPONENTS="mpa-crd-gen mpa-rbac updater-deployment recommender-deployment admission-deployment"

if [ $# -gt 1 ]; then
  COMPONENTS="$2-deployment"
fi

for i in $COMPONENTS; do
  if [ "$i" == admission-deployment ] ; then
    if [ "${ACTION}" == create ] ; then
      (bash "${SCRIPT_ROOT}"/pkg/admission/gencerts.sh || true)
    elif [ "${ACTION}" == delete ] ; then
      (bash "${SCRIPT_ROOT}"/pkg/admission/rmcerts.sh || true)
      (bash "${SCRIPT_ROOT}"/pkg/admission/delete-webhook.sh || true)
    fi
  fi
  if [[ ${ACTION} == print ]]; then
    cat "${SCRIPT_ROOT}"/deploy/"$i".yaml
  else
    kubectl "${ACTION}" -f "${SCRIPT_ROOT}"/deploy/"$i".yaml || true
  fi
done
