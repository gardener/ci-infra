#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0


set -o errexit
set -o nounset
set -o pipefail

cd "$(git rev-parse --show-toplevel)"

docker run --rm -w /etc/ci-infra -v $PWD:/etc/ci-infra \
  gcr.io/k8s-prow/checkconfig:v20240709-d43b36cf6 \
  --config-path=config/prow/config.yaml \
  --job-config-path=config/jobs \
  --plugin-config=config/prow/plugins.yaml \
  --strict \
  --warnings=mismatched-tide-lenient \
  --warnings=tide-strict-branch \
  --warnings=needs-ok-to-test \
  --warnings=validate-owners \
  --warnings=missing-trigger \
  --warnings=validate-urls \
  --warnings=unknown-fields \
  --warnings=duplicate-job-refs
  $@
