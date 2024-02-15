#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0


# Runs pj-on-kind.sh with config arguments specific to the prow.gardener.cloud instance.

set -o errexit
set -o nounset
set -o pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
export CONFIG_PATH="${root}/config/prow/config.yaml"
export JOB_CONFIG_PATH="${root}/config/jobs"

bash <(curl -s https://raw.githubusercontent.com/kubernetes/test-infra/master/prow/pj-on-kind.sh) "$@"
