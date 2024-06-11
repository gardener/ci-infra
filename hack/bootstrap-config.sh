#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0


set -o errexit
set -o nounset
set -o pipefail

# See https://misc.flogisoft.com/bash/tip_colors_and_formatting

color-green() { # Green
  echo -e "\x1B[1;32m${@}\x1B[0m"
}

color-step() { # Yellow
  echo -e "\x1B[1;33m${@}\x1B[0m"
}

color-context() { # Bold blue
  echo -e "\x1B[1;34m${@}\x1B[0m"
}

color-missing() { # Yellow
  echo -e "\x1B[1;33m${@}\x1B[0m"
}

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

if ! [ -x "$(command -v "kubectl")" ]; then
  echo "ERROR: kubectl is not present. Exiting..."
  exit 1
fi

ensure-context() {
  local context=$1
  echo -n " $(color-context "$context")"
  kubectl config get-contexts "$context" &> /dev/null && return 0
  echo ": $(color-missing MISSING), stopping..."
  return 1
}

# create temporary kubeconfig copy that we can modify (switch contexts)
# in the deploy job pod, the kubeconfig is mounted from a secret and secret mounts are read-only filesystems
temp_kubeconfig=$(mktemp)
cleanup-kubeconfig() {
  rm -f "$temp_kubeconfig"
}
trap cleanup-kubeconfig EXIT

kubectl config view --raw > "$temp_kubeconfig"
export KUBECONFIG="$temp_kubeconfig"

echo -n "$(color-step "Ensuring contexts exist"):"
ensure-context gardener-prow-trusted
ensure-context gardener-prow-build
echo " $(color-green done)"

echo "$(color-step "Deploying bootstrap components to gardener-prow-trusted cluster...")"
kubectl config use-context gardener-prow-trusted
kubectl apply --server-side=true -k "$SCRIPT_DIR/../config/prow/cluster/bootstrap-trusted"
echo " $(color-green done)"

echo "$(color-step "Bootstrapping prow to gardener-prow-trusted cluster...")"
kubectl config use-context gardener-prow-trusted
cd "$(git rev-parse --show-toplevel)"

docker run --rm -w /etc/ci-infra -v $PWD:/etc/ci-infra \
  -v "$temp_kubeconfig":/etc/kubeconfig \
  gcr.io/k8s-prow/config-bootstrapper:v20240611-924d31f0c \
  --kubeconfig=/etc/kubeconfig \
  --source-path=.  \
  --config-path=config/prow/config.yaml \
  --plugin-config=config/prow/plugins.yaml \
  --job-config-path=config/jobs \
  --dry-run=false \
  $@
echo " $(color-green done)"
