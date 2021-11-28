#!/usr/bin/env bash

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
