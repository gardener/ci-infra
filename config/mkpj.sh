#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0


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
  gcr.io/k8s-prow/mkpj:v20240306-0b904de3b3 \
  --config-path=config/prow/config.yaml \
  --job-config-path=config/jobs \
  "$@"
