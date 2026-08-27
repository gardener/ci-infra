#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: Contributors to the Gardener project
#
# SPDX-License-Identifier: Apache-2.0


set -o errexit
set -o nounset
set -o pipefail

echo "> Test"

GO111MODULE=on go test -race -timeout=2m $@ | grep -v 'no test files'
