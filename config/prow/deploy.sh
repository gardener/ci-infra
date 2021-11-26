#!/bin/bash

# Prow deploy script
# Based on https://github.com/kubernetes/test-infra/blob/master/prow/deploy.sh

set -o errexit
set -o nounset
set -o pipefail

# See https://misc.flogisoft.com/bash/tip_colors_and_formatting

color-green() { # Green
  echo -e "\x1B[1;32m${@}\x1B[0m"
}

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

prow_components=(
"prow_namespace.yaml"
"test-pods_namespace.yaml"
"cpu-limit-range_limitrange.yaml"
"crier_deployment.yaml"
"crier_rbac.yaml"
"crier_service.yaml"
"deck_deployment.yaml"
"deck_rbac.yaml"
"deck_service.yaml"
"gce-ssd_storageclass.yaml"
"ghproxy_deployment.yaml"
"ghproxy_pvc.yaml"
"ghproxy_service.yaml"
"hook_deployment.yaml"
"hook_rbac.yaml"
"hook_service.yaml"
"horologium_deployment.yaml"
"horologium_rbac.yaml"
"horologium_service.yaml"
"prow_ingress.yaml"
"mem-limit-range_limitrange.yaml"
"needs-rebase_deployment.yaml"
"needs-rebase_service.yaml"
"prow_controller_manager_deployment.yaml"
"prow_controller_manager_rbac.yaml"
"prow_controller_manager_service.yaml"
"sinker_deployment.yaml"
"sinker_rbac.yaml"
"sinker_service.yaml"
"statusreconciler_deployment.yaml"
"statusreconciler_rbac.yaml"
"tide_deployment.yaml"
"tide_rbac.yaml"
"tide_service.yaml"
)

if ! [ -x "$(command -v "kubectl")" ]; then
  echo "ERROR: kubectl is not present. Exiting..."
  exit 1
fi

echo "$(color-green Deploying Prow...)"

# use server-side apply for CRD, otherwise annotation will be too long
kubectl apply --server-side=true -f "$SCRIPT_DIR/cluster/prowjob_customresourcedefinition.yaml"

for c in "${prow_components[@]}"; do
  kubectl apply -f "$SCRIPT_DIR/cluster/$c"
done
echo "$(color-green SUCCESS)"
