#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0


set -o errexit
set -o nounset
set -o pipefail

cd "$(git rev-parse --show-toplevel)"

docker run --rm -w /etc/ci-infra -v $PWD:/etc/ci-infra \
  us-docker.pkg.dev/k8s-infra-prow/images/checkconfig:v20241113-0609cf597 \
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
