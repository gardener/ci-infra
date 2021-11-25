# ci-infra

This repository contains configuration files for the testing and automation needs of the Gardener project.

## ⚠️ Warning 🚧

This is currently under construction / in evaluation phase.

## CI Job Management

Gardener uses a [`prow`](https://github.com/kubernetes/test-infra/blob/master/prow) instance at [prow.gardener.cloud] to handle CI and
automation for parts of the project. Everyone can participate in a
self-service PR-based workflow, where changes are automatically deployed
after they have been reviewed. All job configs are located in [`config/jobs`].
