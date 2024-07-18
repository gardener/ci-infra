#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0


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

echo "Templating ingress-nginx"
helm template -n ingress-nginx ingress-nginx ingress-nginx/ingress-nginx --version "4.11.1" -f $SCRIPT_DIR/values.yaml > $SCRIPT_DIR/../ingress-nginx-deployment.yaml

echo "Done"
