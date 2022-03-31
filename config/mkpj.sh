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

# Usage: mkpj.sh --job=foo ...
#
# Arguments to this script will be passed to a dockerized mkpj
#
# Example Usage:
# config/mkpj.sh --job=post-test-infra-push-bootstrap | kubectl create -f -
#
# NOTE: kubectl should be pointed at the prow services cluster you intend
# to create the prowjob in!

cd "$(git rev-parse --show-toplevel)"

docker run -i --rm -w /etc/ci-infra -v $PWD:/etc/ci-infra \
  gcr.io/k8s-prow/mkpj:v20220330-40eb179576 \
  --config-path=config/prow/config.yaml \
  --job-config-path=config/jobs \
  "$@"
