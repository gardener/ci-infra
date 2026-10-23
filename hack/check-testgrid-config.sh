#!/usr/bin/env bash
# SPDX-FileCopyrightText: Contributors to the Gardener project
#
# SPDX-License-Identifier: Apache-2.0


set -o errexit
set -o nounset
set -o pipefail

cd "$(git rev-parse --show-toplevel)"

docker run --rm -w /etc/ci-infra -v $PWD:/etc/ci-infra \
  gcr.io/k8s-staging-test-infra/configurator:v20261023-9ddb299e5f \
  --yaml=config/testgrids/config.yaml \
  --default=config/testgrids/default.yaml \
  --prow-config=config/prow/config.yaml \
  --prow-job-config=config/jobs \
  --prowjob-url-prefix=https://github.com/gardener/ci-infra/tree/master/config/jobs \
  --update-description \
  --validate-config-file \
  --oneshot
