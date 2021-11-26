# ci-infra

This repository contains configuration files for the testing and automation needs of the Gardener project.

## ⚠️ Warning 🚧

This is currently under construction / in evaluation phase.

## CI Job Management

Gardener uses a [`prow`](https://github.com/kubernetes/test-infra/blob/master/prow) instance at [prow.gardener.cloud] to handle CI and
automation for parts of the project. Everyone can participate in a
self-service PR-based workflow, where changes are automatically deployed
after they have been reviewed. All job configs are located in [`config/jobs`].

## How to setup

1. Create the prow cluster and prow workload cluster.
   ```bash
   $ kubectl apply -f config/clusters/shoot.yaml
   $ kubectl apply -f config/clusters/shoot-workload.yaml
   ```
1. Create the `prow` namespace in the prow cluster:
   ```bash
   $ kubectl apply -f config/prow/cluster/prow_namespace.yaml
   ```
1. Create the required secrets in the prow cluster:
  - `gardener-prow-storage` (Service account with `Storage Admin` permissions for GCS bucket, according to [test-infra guide](https://github.com/kubernetes/test-infra/blob/f8021394c8e493af2d3ec336a87888368d92e0c8/prow/getting_started_deploy.md#configure-a-gcs-bucket))
  - `github-app` (according to [test-infra guide](https://github.com/kubernetes/test-infra/blob/f8021394c8e493af2d3ec336a87888368d92e0c8/prow/getting_started_deploy.md#github-app))
  - `github-oauth-config` (according to [test-infra guide](https://github.com/kubernetes/test-infra/blob/f8021394c8e493af2d3ec336a87888368d92e0c8/prow/cmd/deck/github_oauth_setup.md))
  - `hmac-token`
    ```bash
    $ kubectl -n prow create secret generic hmac-token --from-literal=hmac=$(openssl rand -hex 20)
    ```
  - `oauth-cookie-secret`
    ```bash
    $ kubectl -n prow create secret generic oauth-cookie-secret --from-literal=hmac=$(openssl rand -base64 32)
    ```
  - `kubeconfig` (according to [test-infra guide](https://github.com/kubernetes/test-infra/blob/f8021394c8e493af2d3ec336a87888368d92e0c8/prow/getting_started_deploy.md#run-test-pods-in-different-clusters))
1. Deploy Prow components:
   ```bash
   $ ./config/prow/deploy.sh
   ```
