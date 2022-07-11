#!/usr/bin/env bash
# Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

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

if ! [ -x "$(command -v "gcloud")" ]; then
  echo "ERROR: gcloud is not present. Exiting..."
  exit 1
fi

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
ensure-context garden-garden-ci
echo " $(color-green done)"

# create temporary files for service account keys and ensure that they are cleaned up finally
temp_gcr_serviceaccount=$(mktemp)
temp_storage_serviceaccount=$(mktemp)
temp_infrastructure_serviceaccount=$(mktemp)
cleanup-temp-serviceaccount-files() {
  rm -f "$temp_gcr_serviceaccount"
  rm -f "$temp_storage_serviceaccount"
  rm -f "$temp_infrastructure_serviceaccount"
}
trap cleanup-temp-serviceaccount-files EXIT

# Updating service account key for pushing images to eu.gcr.io
echo "$(color-step "Updating service account key for pushing images to eu.gcr.io...")"
gcloud iam service-accounts keys create $temp_gcr_serviceaccount \
    --iam-account=gardener-prow-gcr@gardener-project.iam.gserviceaccount.com \
    --project=gardener-project

kubectl config use-context gardener-prow-trusted
# the docker config needs to be in the format of a config.json file
kubectl create secret docker-registry -n test-pods gardener-prow-gcr-docker-config \
  --docker-server=eu.gcr.io \
  --docker-username=_json_key \
  --docker-password="$(cat $temp_gcr_serviceaccount)" \
  --docker-email=gardener-prow-gcr@gardener-project.iam.gserviceaccount.com \
  --dry-run=client -o yaml \
  | sed "s/kubernetes.io\/dockerconfigjson/Opaque/" \
  | sed "s/\.dockerconfigjson/config.json/" \
  | kubectl apply --server-side=true --force-conflicts -f -

echo "$(color-green done)"

# Updating service account key for accessing prow gcs storage bucket
echo "$(color-step "Updating service account key for accessing prow gcs storage bucket...")"

gcloud iam service-accounts keys create $temp_storage_serviceaccount \
    --iam-account=gardener-prow-storage@gardener-project.iam.gserviceaccount.com \
    --project=gardener-project

kubectl config use-context gardener-prow-trusted
kubectl create secret generic -n prow gardener-prow-storage \
    --from-file=service-account.json=$temp_storage_serviceaccount \
    --dry-run=client -o yaml \
    | kubectl apply --server-side=true --force-conflicts -f -
kubectl create secret generic -n test-pods gardener-prow-storage \
    --from-file=service-account.json=$temp_storage_serviceaccount \
    --dry-run=client -o yaml \
    | kubectl apply --server-side=true --force-conflicts -f -

kubectl config use-context gardener-prow-build
kubectl create secret generic -n test-pods gardener-prow-storage \
    --from-file=service-account.json=$temp_storage_serviceaccount \
    --dry-run=client -o yaml \
    | kubectl apply --server-side=true --force-conflicts -f -

echo "$(color-green done)"

# Restarting all prow components in trusted cluster, that they use the new secret
echo "$(color-step "Rolling prow deployments...")"
kubectl config use-context gardener-prow-trusted
kubectl -n prow rollout restart deployments
echo "$(color-green done)"

# Updating service account key of garden-ci infrastructure secret
echo "$(color-step "Updating service account key of garden-ci infrastructure secret...")"

gcloud iam service-accounts keys create $temp_infrastructure_serviceaccount \
    --iam-account=gardener-prow@gardener-project.iam.gserviceaccount.com \
    --project=gardener-project

kubectl config use-context garden-garden-ci
# Secrets modified via gardener dashboard write service account into serviceaccount.json
kubectl create secret generic -n garden-garden-ci gcp-gardener-prow \
    --from-file=serviceaccount.json=$temp_infrastructure_serviceaccount \
    --dry-run=client -o yaml \
    | kubectl apply --server-side=true --force-conflicts -f -

echo "$(color-green done)"

# Start reconciling trusted and build prow clusters to enable using the new infrastructure secret
echo "$(color-step "Start reconciling trusted and build prow clusters to enable using the new infrastructure secret...")"
kubectl config use-context garden-garden-ci
kubectl annotate shoot -n garden-garden-ci prow gardener.cloud/operation=reconcile
kubectl annotate shoot -n garden-garden-ci prow-work gardener.cloud/operation=reconcile
echo "$(color-green done)"

echo "$(color-green SUCCESS)"
echo "$(color-missing Old service account keys are not deleted automatically. Please check service accounts gardener-prow-gcr and gardener-prow-storage.)"
