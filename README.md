# ci-infra
[![REUSE status](https://api.reuse.software/badge/github.com/gardener/ci-infra)](https://api.reuse.software/info/github.com/gardener/ci-infra)


This repository contains configuration files for the testing and automation needs of the Gardener project.

## CI Job Management

Gardener uses a [`prow`](https://github.com/kubernetes/test-infra/blob/master/prow) instance at [prow.gardener.cloud](https://prow.gardener.cloud) to handle CI and automation for parts of the project.
Everyone can participate in a self-service PR-based workflow, where changes are automatically deployed after they have been reviewed and merged.
All job configs are located in [`config/jobs`](config/jobs).

### TestGrid
The results of prow jobs can be visualized in TestGrid in dashboards at [testgrid.k8s.io/gardener](https://testgrid.k8s.io/gardener). We don't run our own TestGrid installation, but include our dashboards into the TestGrid installation of Kubernetes.

We configured dashboards for each of our repositories where we run tests with prow. You find them at [config/testgrids/config.yaml](./config/testgrids/config.yaml).

When the desired dashboard is defined, you can add your prow job to a dashboard annotating them like in the example below.

```yaml
annotations:
  testgrid-dashboards: dashboard-name      # [Required] A dashboard already defined in gardener-testgrid.yaml.
  testgrid-tab-name: some-short-name       # [Optional] A shorter name for the tab. If omitted, just uses the job name.
  testgrid-alert-email: me@me.com          # [Optional] An alert email that will be applied to the tab created in the first dashboard specified in testgrid-dashboards.
  description: Words about your job.       # [Optional] A description of your job. If omitted, only the job name is used.
  testgrid-num-columns-recent: "10"        # [Optional] The number of runs in a row that can be omitted before the run is considered stale. The default value is 10.
  testgrid-num-failures-to-alert: "3"      # [Optional] The number of continuous failures before sending an email. The default value is 3.
  testgrid-days-of-results: "15"           # [Optional] The number of days for which the results are visible. The default value is 15.
  testgrid-alert-stale-results-hours: "12" # [Optional] The number of hours that pass with no results after which the email is sent. The default value is 12.
```

For `postsubmit` and `periodic` prow jobs there will be a test-group created automatically. If you don't want to add them to TestGrid please use this annotation to disable creation of a test-group. For `presubmit` prow jobs no test-group will be created unless you annotate them as in the previous example.
```yaml
annotations:
  testgrid-create-test-group: "false"
```

You can test your TestGrid configuration locally with the `./hack/check-testgrid-config.sh`. Please open a PR for `ci-infra` repository for your new configuration. When it is merged the new configuration will be pushed to `gs://gardener-prow/testgrid/config` automatically and your jobs will become visible at [testgrid.k8s.io/gardener](https://testgrid.k8s.io/gardener) soon.


## Combined kubeconfig for prow clusters and Gardener project

The scripts from this repository rely on a combined `kubeconfig`. It contains two contexts for the prow clusters `gardener-prow-trusted`, `gardener-prow-build` and one for the Gardener project the clusters are created in.
Please setup your local kubeconfig file by using the `hack/setup-prow-kubeconfig.sh` script. Afterwards, you find it here:
```bash
export KUBECONFIG=~/.gardener-prow/kubeconfig/kubeconfig--gardener--prow-combined.yaml
```
The kubeconfig contains absolute paths. Thus, it won't work anymore, if you move it to a different location.

## How to setup

The following commands assume you are using the combined `kubeconfig` generated in the previous section. When you create new clusters the configuration of `gardener-prow-trusted`, `gardener-prow-build`  contexts will be incomplete in the beginning. They are completed in step 2 when the clusters have been created.


1. Create the prow cluster and prow workload cluster.

   Please copy cluster spec from prow config GCS bucket to your /tmp folder and run these commands. 
   ```bash
   kubectl config use-context garden-cluster
   kubectl apply -f /tmp/clusters/prow-trusted.yaml
   kubectl apply -f /tmp/clusters/prow-build.yaml
   ```
2. Complete your combined kubeconfig with the data of the clusters created in the previous step
3. Create the `prow` namespace in the prow cluster:
   ```bash
   kubectl config use-context gardener-prow-trusted
   kubectl apply --server-side=true -f config/prow/cluster/prow_namespace.yaml
   ```
4. Create the `test-pods` namespace in the workload/build cluster:
   ```bash
   kubectl config use-context gardener-prow-build
   kubectl apply --server-side=true -f config/prow/cluster/base/test-pods_namespace.yaml
   ```   
5. Create the required secrets (mainly in the prow cluster):
    - the secrets for GCP service accounts can be created by our credentials rotation script `./hack/rotate-secrets.sh`. Please see Rotate [credentials section](#rotate-credentials) for more details.
    - `github-app` (according to [test-infra guide](https://github.com/kubernetes/test-infra/blob/f8021394c8e493af2d3ec336a87888368d92e0c8/prow/getting_started_deploy.md#github-app))
    - `github-token` (Personal Access Token for [@gardener-ci-robot](https://github.com/gardener-ci-robot) with scopes `public_repo, read:org, repo:status`, needs to be present in the `prow` and `test-pods` namespace of the prow cluster)
    - `github` (Personal Access Token for [@gardener-ci-robot](https://github.com/gardener-ci-robot) with `repo` scope, needs to be present in `renovate` namespace of the prow cluster)
    - `github-oauth-config` (according to [test-infra guide](https://github.com/kubernetes/test-infra/blob/f8021394c8e493af2d3ec336a87888368d92e0c8/prow/cmd/deck/github_oauth_setup.md))
    - `hmac-token`
      ```bash
      kubectl config use-context gardener-prow-trusted
      kubectl -n prow create secret generic hmac-token --from-literal=hmac=$(openssl rand -hex 20)
      ```
    - `oauth-cookie-secret`
      ```bash
      kubectl config use-context gardener-prow-trusted
      kubectl -n prow create secret generic oauth-cookie-secret --from-literal=secret=$(openssl rand -base64 32)
      ```
    - `oauth2-proxy`
      ```bash
      kubectl config use-context gardener-prow-trusted
      kubectl -n oauth2-proxy create secret generic oauth2-proxy \
      --from-literal=cookie-secret=$(openssl rand -base64 32) \
      --from-literal=client-id=<Github OAuth2 Proxy App client-id> \
      --from-literal=client-secret=<Github OAuth2 Proxy App client-secret>
      ```
    - `kubeconfig` (ref [test-infra guide](https://github.com/kubernetes/test-infra/blob/f8021394c8e493af2d3ec336a87888368d92e0c8/prow/getting_started_deploy.md#run-test-pods-in-different-clusters), needs to be present in the `prow` and `test-pods` namespace of the prow-trusted cluster)
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
    - `slack-token` for the `Gardener Prow` Slack App in the [Gardener Project workspace](https://gardener-cloud.slack.com/)
      - follow the [test-infra guide](https://github.com/kubernetes/test-infra/blob/master/prow/cmd/crier/README.md#slack-reporter) for setting up the Slack App
      - this is used by crier to report job status changes (e.g., test failures) to dedicated Slack channels
      - generally, failures/errors of `periodic`, `postsubmit` and `batch` jobs are reported to [`#test-failures`](https://gardener-cloud.slack.com/archives/C046PHX7ACR)
      - job status changes concerning the prow infrastructure itself (e.g., deploy and autobump jobs) are reported to [`#prow-alerts`](https://gardener-cloud.slack.com/archives/C04680KRF8D)
      - this token is also used by hook (`slack` plugin) to post merge warnings to [`#prow-alerts`](https://gardener-cloud.slack.com/archives/C04680KRF8D)
    - `alertmanager-slack`: URLs for incoming webhooks for the [#gardener-prow-alerts](https://sap-ti.slack.com/archives/C02P7MSTJHJ) channel in the SAP Slack workspace
      - alertmanager instances in both clusters use an incoming webhook to post monitoring alerts to Slack
      - different webhooks are used for the two instances
      - for both clusters (prow-trusted and prow-build) do the following:
        - follow https://api.slack.com/incoming-webhooks and set up a webhook for posting to `#gardener-prow-alerts`
        - create a secret in the `monitoring` namespace of the respective cluster with the Webhook URL under key `api_url`
    - `grafana-admin` (admin user password)
      ```bash
      kubectl config use-context gardener-prow-trusted
      kubectl -n monitoring create secret generic grafana-admin --from-literal=admin_password=$(openssl rand -base64 32)
      kubectl config use-context gardener-prow-build
      kubectl -n monitoring create secret generic grafana-admin --from-literal=admin_password=$(openssl rand -base64 32)
      ```
6. Deploy Prow components. The initial deployment has to be done manually, later on changes to the components will be automatically deployed once merged into master.
   ```bash
   ./config/prow/deploy.sh
   ```
7. Bootstrap Prow configuration/jobs. This initial configuration has to be done manually, later on changes to configuration and jobs will be automatically applied by the [`updateconfig`](https://github.com/kubernetes/test-infra/tree/master/prow/plugins/updateconfig) plugin once merged into master. The bootstrap tool does not work with the kubectl OIDC auth plugin. Thus, for the initial bootstrapping run, you will need a kubeconfig with a token for the trusted cluster.
   ```bash
   ./hack/boostrap-config.sh
   ```

The [getting started guide](https://github.com/kubernetes/test-infra/blob/master/prow/getting_started_deploy.md) in `kubernetes/test-infra` is a good starting point for further investigations.

## Monitoring

A monitoring stack based on [kube-prometheus](https://github.com/prometheus-operator/kube-prometheus) plus [test-infra monitoring](https://github.com/kubernetes/test-infra/tree/master/config/prow/cluster/monitoring) capabilities is installed in the prow clusters:
- [prometheus-operator](https://github.com/prometheus-operator/prometheus-operator)
- alertmanager (cluster with 3 replicas for HA)
- prometheus (2 replicas for HA)
- blackbox-exporter
- kube-state-metrics
- grafana

Alertmanager will send Slack alerts in [`#gardener-prow-alerts`](https://sap-ti.slack.com/archives/C02P7MSTJHJ) in the SAP Slack workspace.

Grafana is available publicly at https://monitoring.prow.gardener.cloud (trusted cluster) and https://monitoring-build.prow.gardener.cloud (build cluster).

## Renovate

[`Renovate` 🤖](https://github.com/renovatebot/renovate) is working in repositories which are enabled in `prow`.

You can enable it by creating a `renovate.json5` file in the respective repository. Please check the [renovate docs](https://docs.renovatebot.com/configuration-options/) for the configuration options.

This repository also hosts [shared config presets](config/renovate) for renovate to reuse common configuration across gardener repositories.

### Changing Renovate Configuration

Renovate can be difficult and tiresome to configure correctly.
Follow these two tips to validate your configuration changes before opening/merging PRs!

Both require a local installation of renovate, e.g.:

```bash
brew install renovate
```

#### Config Validation

Use the [config validator](https://docs.renovatebot.com/config-validation/) to ensure you have no syntax or schema issues in your configuration file:

```
$ renovate-config-validator
 INFO: Validating .github/renovate.json5
 INFO: Config validated successfully
```

[`config/jobs/common/renovate-presubmits.yaml`](./config/jobs/common/renovate-presubmits.yaml) has a reusable prow job definition that runs `renovate-config-validator` as a presubmit job.

#### Local Dry-Run

You can perform a [dry-run](https://docs.renovatebot.com/self-hosted-configuration/#dryrun) of renovate by using the [`local` platform](https://docs.renovatebot.com/modules/platform/local/).
It will output a lot of logs which indicate what dependencies were detected, what updates are available, etc. without actually opening/updating PRs:

```
$ LOG_LEVEL=debug GITHUB_API_GITHUB_COM_TOKEN=$(cat ~/path/to/my-personal-access-token) renovate --detect-host-rules-from-env --platform=local
```

Renovate requires a token for repository lookups on GitHub.
You can use any [personal access token](https://github.com/settings/tokens) on your account.
