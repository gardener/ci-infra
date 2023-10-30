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

cd "$(git rev-parse --show-toplevel)"

docker run --rm -w /etc/ci-infra -v $PWD:/etc/ci-infra \
  gcr.io/k8s-prow/configurator:v20231027-9cda73bb73 \
  --yaml=config/testgrids/config.yaml \
  --default=config/testgrids/default.yaml \
  --prow-config=config/prow/config.yaml \
  --prow-job-config=config/jobs \
  --prowjob-url-prefix=https://github.com/gardener/ci-infra/tree/master/config/jobs \
  --update-description \
  --validate-config-file \
  --oneshot
