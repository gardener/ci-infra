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

set -o errexit
set -o nounset
set -o pipefail

cd "$(git rev-parse --show-toplevel)"

pat_path="${GITHUB_PAT_PATH:-}"
if [ -z "$pat_path" ] ; then
  echo "Error: no personal access token given. Please set GITHUB_PAT_PATH."
  exit 1
fi

docker run --rm -v $PWD/config/prow/labels.yaml:/etc/config/labels.yaml -v "$pat_path":/etc/github/oauth \
  gcr.io/k8s-prow/label_sync:v20210928-0afc0f8086 \
  --config /etc/config/labels.yaml \
  --token /etc/github/oauth \
  --only gardener/ci-infra \
  $@
