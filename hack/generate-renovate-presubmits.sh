#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: Contributors to the Gardener project
#
# SPDX-License-Identifier: Apache-2.0


set -o errexit
set -o nounset
set -o pipefail

# Regenerates config/jobs/common/renovate-presubmits.yaml locally (dry-run), creating a
# renovate-config-validator presubmit for every eligible repository of the gardener org.
# In production the ci-renovate-presubmit-generator periodic keeps the file in sync; this
# script is a convenience wrapper to preview changes locally.
#
# GITHUB_TOKEN_PATH must point to a file containing a GitHub token with read access to the
# gardener org (listing repositories). Additional flags are passed through to the generator,
# e.g. --skip-repos=gardener/<repo>.

echo "> Generate renovate presubmits"

if [ -z "${GITHUB_TOKEN_PATH:-}" ]; then
  echo "Please set GITHUB_TOKEN_PATH to a file containing a GitHub token with read access to the gardener org."
  exit 1
fi

cd "$(git rev-parse --show-toplevel)"

go run ./prow/cmd/renovate-presubmit-generator --github-token-path="$GITHUB_TOKEN_PATH" "$@"
