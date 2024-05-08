#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0


# This script templates the oauth2-proxy helm chart for gardener-ci prow cluster
# and replaces the old content of /config/prow/cluster/oauth2-proxy/oauth2-proxy with the freshly templated version

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

if ! which helm &>/dev/null; then
  echo "helm not found, please install it"
  exit 1
fi

echo "Adding & updating oauth2-proxy helm repository"
helm repo add oauth2-proxy https://oauth2-proxy.github.io/manifests
helm repo update

echo "Templating oauth2-proxy"
helm template -n oauth2-proxy oauth2-proxy oauth2-proxy/oauth2-proxy --version "7.5.4" -f $SCRIPT_DIR/values.yaml > $SCRIPT_DIR/../oauth2-proxy-deployment.yaml

echo "Done"
