#!/usr/bin/env bash
# Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

# This script templates the ingress-nginx helm chart for gardener-ci prow cluster
# and replaces the old content of /config/prow/cluster/ingress-nginx with the freshly templated version

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

if ! which helm &>/dev/null; then
  echo "helm not found, please install it"
  exit 1
fi

echo "Adding & updating ingress-nginx helm repository"
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm repo update

echo "Creating ingress-nginx namespace yaml"
cat <<EOF > $SCRIPT_DIR/../prow/cluster/ingress-nginx/00_ingress-nginx-namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: ingress-nginx
EOF

echo "Templating ingress-nginx"
# Check values.yaml for help https://github.com/kubernetes/ingress-nginx/blob/main/charts/ingress-nginx/values.yaml
helm template -n ingress-nginx ingress-nginx ingress-nginx/ingress-nginx -f - > $SCRIPT_DIR/../prow/cluster/ingress-nginx/ingress-nginx-deployment.yaml <<EOF
controller:
  replicaCount: 2
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: worker.gardener.cloud/pool
            operator: In
            values:
            - controlplane
  updateStrategy:
    rollingUpdate:
      maxUnavailable: 1
    type: RollingUpdate
  watchIngressWithoutClass: true
  ingressClassResource:
    name: nginx
    enabled: true
    default: true
defaultBackend:
  enabled: true
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: worker.gardener.cloud/pool
            operator: In
            values:
            - controlplane
EOF

echo "Done"
