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

# See https://misc.flogisoft.com/bash/tip_colors_and_formatting

color-green() { # Green
  echo -e "\x1B[1;32m${@}\x1B[0m"
}

color-step() { # Yellow
  echo -e "\x1B[1;33m${@}\x1B[0m"
}

color-context() { # Bold blue
  echo -e "\x1B[1;34m${@}\x1B[0m"
}

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

echo "$(color-step "Copy prow kubeconfig to ~/.gardener-prow/kubeconfig/kubeconfig--gardener--prow-combined.yaml")"
mkdir -p ~/.gardener-prow/kubeconfig/
cp -p $SCRIPT_DIR/../config/kubeconfig/garden.yaml ~/.gardener-prow/kubeconfig/
# Credential plugin does not get environment variable from shell. Thus, $HOME is empty and "~/" paths do not work in this context
cat $SCRIPT_DIR/../config/kubeconfig/kubeconfig--gardener--prow-combined.yaml |\
  sed "s:{{ garden-path }}:$(realpath ~/.gardener-prow/kubeconfig/garden.yaml):g" \
  > ~/.gardener-prow/kubeconfig/kubeconfig--gardener--prow-combined.yaml
chmod 600 ~/.gardener-prow/kubeconfig/*.yaml
echo "$(color-green done)"

echo "$(color-context "Activate prow kubeconfig with 'export KUBECONFIG=~/.gardener-prow/kubeconfig/kubeconfig--gardener--prow-combined.yaml'")"
echo "$(color-step "The kubeconfig contains absolute paths. Thus, it won't work anymore, if you move it to a different location.")"
echo "$(color-green success)"
