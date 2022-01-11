# ci-infra

This repository contains configuration files for the testing and automation needs of the Gardener project.

## ⚠️ Warning 🚧

This is currently under construction / in evaluation phase.

## CI Job Management

Gardener uses a [`prow`](https://github.com/kubernetes/test-infra/blob/master/prow) instance at [prow.gardener.cloud](https://prow.gardener.cloud) to handle CI and automation for parts of the project.
Everyone can participate in a self-service PR-based workflow, where changes are automatically deployed after they have been reviewed and merged.
All job configs are located in [`config/jobs`](config/jobs).

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
1. Create the `test-pods` namespace in the workload/build cluster:
   ```bash
   $ kubectl apply -f config/prow/cluster/build
   ```   
1. Create the required secrets (mainly in the prow cluster):
    - `gardener-prow-storage` (Service account with `Storage Admin` permissions for GCS bucket, according to [test-infra guide](https://github.com/kubernetes/test-infra/blob/f8021394c8e493af2d3ec336a87888368d92e0c8/prow/getting_started_deploy.md#configure-a-gcs-bucket), needs to be present in the `prow` namespace and in the `test-pods` namespace in both clusters)
    - `github-app` (according to [test-infra guide](https://github.com/kubernetes/test-infra/blob/f8021394c8e493af2d3ec336a87888368d92e0c8/prow/getting_started_deploy.md#github-app))
    - `github-token` (Personal Access Token for [@gardener-ci-robot](https://github.com/gardener-ci-robot) with scopes `public_repo, read:org, repo:status`, needs to be present in the `prow` and `test-pods` namespace of the prow cluster)
    - `github-oauth-config` (according to [test-infra guide](https://github.com/kubernetes/test-infra/blob/f8021394c8e493af2d3ec336a87888368d92e0c8/prow/cmd/deck/github_oauth_setup.md))
    - `hmac-token`
      ```bash
      $ kubectl -n prow create secret generic hmac-token --from-literal=hmac=$(openssl rand -hex 20)
      ```
    - `oauth-cookie-secret`
      ```bash
      $ kubectl -n prow create secret generic oauth-cookie-secret --from-literal=secret=$(openssl rand -base64 32)
      ```
    - `kubeconfig` (ref [test-infra guide](https://github.com/kubernetes/test-infra/blob/f8021394c8e493af2d3ec336a87888368d92e0c8/prow/getting_started_deploy.md#run-test-pods-in-different-clusters), needs to be present in the `prow` and `test-pods` namespace of the prow cluster)
      - add two contexts: the prow cluster as `gardener-prow-trusted` and the build/workload cluster as `gardener-prow-build`
      - `gardener-prow-trusted` context should use the in-cluster `ServiceAccount` token and CA file, so that all Prow components are bound to their respective RBAC roles
      - `gardener-prow-build` needs to be bound to the `cluster-admin` role. The [gencred](https://github.com/kubernetes/test-infra/tree/master/gencred) utility can be used to easily create a `ServiceAccount` and `ClusterRoleBinding` and retrieve the `ServiceAccount` token.
      - Template:
        ```yaml
        apiVersion: v1
        kind: Config
        current-context: gardener-prow-build # default cluster
        contexts:
        - name: gardener-prow-trusted
          context:
            cluster: gardener-prow-trusted
            user: gardener-prow-trusted-token
        - name: gardener-prow-build
          context:
            cluster: gardener-prow-build
            user: gardener-prow-build-token
        clusters:
        - name: gardener-prow-trusted
          cluster: # in-cluster config
            server: 'https://kubernetes.default.svc'
            certificate-authority: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
        - name: gardener-prow-build
          cluster:
            server: <<workload-cluster-api-server-address>>
            certificate-authority-data: <<base64-encoded-CA-bundle>>
        users:
        - name: gardener-prow-trusted-token
          user:
            tokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token # use in-cluster config
        - name: gardener-prow-build-token
          user:
            token: <<service-account-token-with-cluster-admin-permissions>> # generated via gencred
        ```
    - `slack-token` (according to [test-infra guide](https://github.com/kubernetes/test-infra/blob/master/prow/cmd/crier/README.md#slack-reporter))
    - `alertmanager-prow-slack` (needs to be present in the prow-monitoring namespace of the prow cluster)
      - Follow https://api.slack.com/incoming-webhooks and setup a webhook.
      - Create the secret including the Webhook URL under key `api_url`.
    - `grafana` (admin user password)
      ```bash
      $ kubectl -n prow-monitoring create secret generic grafana --from-literal=admin_password=$(openssl rand -base64 32)
      ```
1. Deploy Prow components. The initial deployment has to be done manually, later on changes to the components will be automatically deployed once merged into master.
   ```bash
   $ ./config/prow/deploy.sh
   ```
1. Bootstrap Prow configuration/jobs. This initial configuration has to be done manually, later on changes to configuration and jobs will be automatically applied by the [`updateconfig`](https://github.com/kubernetes/test-infra/tree/master/prow/plugins/updateconfig) plugin once merged into master.
   ```bash
   $ ./hack/boostrap-config.sh
   ```

## Monitoring

A basic monitoring stack is installed in the prow cluster consisting of:
- [prometheus-operator](https://github.com/prometheus-operator/prometheus-operator)
- alertmanager (cluster with 3 replicas for HA)
- prometheus (2 replicas for HA)
- blackbox-exporter
- kube-state-metrics
- grafana

Alertmanager will send Slack alerts in `#gardener-prow-alerts` (SAP-internal workspace).

Grafana is available publicly at https://monitoring.prow.gardener.cloud.
