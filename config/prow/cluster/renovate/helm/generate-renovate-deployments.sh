#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0


# This script templates the renovate helm chart for gardener-ci prow cluster
# and replaces the old content of /config/prow/cluster/renovate/renovate_deployment.yaml with the freshly templated version

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

if ! which helm &>/dev/null; then
  echo "helm not found, please install it"
  exit 1
fi

echo "Adding & updating renovate helm repository"
helm repo add renovatebot https://docs.renovatebot.com/helm-charts
helm repo update

echo "Templating renovate"
helm template -n renovate renovate renovatebot/renovate --version "37.422.2" -f $SCRIPT_DIR/values.yaml > $SCRIPT_DIR/../renovate_deployment.yaml

echo "Done"
