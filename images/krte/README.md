# `krte` image

**Note: This is a fork of [krte from kubernetes/test-infra (commit 8b8d9ff)](https://github.com/kubernetes/test-infra/tree/8b8d9ff4819af95d51b02160a2f99c74459f22d7/images/krte) repository.
It changes the Go version to Gardener requirements and is based on Gardener [golang-test](https://github.com/gardener/gardener/tree/master/hack/tools/image) image.**

krte - [KIND](https://sigs.k8s.io/kind) RunTime Environment

This image contains things we need to run kind in Kubernetes CI, and
is maintained for the sole purpose of testing Kubernetes with KIND.



## WARNING

This image is _not_ supported for other use cases. Use at your own risk.
