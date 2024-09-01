#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0


# This script templates the athens helm chart for gardener-ci prow cluster
# and replaces the old content of /config/prow/cluster/athens/athens_deployment.yaml with the freshly templated version

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

if ! which helm &>/dev/null; then
  echo "helm not found, please install it"
  exit 1
fi

echo "Adding & updating athens helm repository"
helm repo add gomods https://gomods.github.io/athens-charts
helm repo update

echo "Templating athens"
helm template -n athens athens gomods/athens-proxy --version "0.12.2" -f $SCRIPT_DIR/values.yaml > $SCRIPT_DIR/../athens_deployment.yaml

echo "Done"
