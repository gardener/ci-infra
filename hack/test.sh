#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0


set -o errexit
set -o nounset
set -o pipefail

echo "> Test"

GO111MODULE=on go test -race -timeout=2m -mod=vendor $@ | grep -v 'no test files'
