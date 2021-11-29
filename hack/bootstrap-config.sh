#!/usr/bin/env bash

cd "$(git rev-parse --show-toplevel)"

kubeconfig="${KUBECONFIG:-}"
if [ -z "$kubeconfig" ] ; then
  echo "Error: no kubeconfig given. Please set KUBECONFIG."
  exit 1
fi

docker run --rm -w /etc/ci-infra -v $PWD/config:/etc/ci-infra/config \
  -v "$kubeconfig":/etc/kubeconfig \
  gcr.io/k8s-prow/config-bootstrapper:v20211126-48cb2fc883 \
  --kubeconfig=/etc/kubeconfig \
  --source-path=.  \
  --config-path=config/prow/config.yaml \
  --plugin-config=config/prow/plugins.yaml \
  --job-config-path=config/jobs \
  --dry-run=false \
  $@
