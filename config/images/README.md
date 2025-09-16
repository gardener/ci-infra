# Copy Images

## Overview

In order to avoid DockerHub rate limit for image pulls and to support IPv6 Single Stack it is necessary
to copy all needed container images to gardener GCR.

In order to automate this effort, there is a CI job using crane to copy all specified images and tags for all architectures to AR.
The images needed are all listed inside the `images.yaml` with the following schema:

```yaml
images:
- source: kubernetesui/dashboard
  destination: europe-docker.pkg.dev/gardener-project/releases/3rd/kubernetesui/dashboard
  tags:
  - source: v2.2.0
  - source: v2.4.0
  - source: v2.5.1
- source: envoyproxy/envoy
  destination: europe-docker.pkg.dev/gardener-project/releases/3rd/envoyproxy/envoy
  tags:
  - source: distroless-v1.35.0
    destination: v1.35.0-distroless
```

## Usage

### Local

The script allows to copy images from any location to any location.

It is required to have yq and crane installed.
```bash
brew install yq
brew install crane
```

**Authentication**
There are two ways to authenticate to a private container registry.
1) Crane will use your existing docker config located at `~/.docker/config.json`
2) Using crane to create a `config.json` using `crane auth <registry> -u <username> -p <password>`

You can run the `copy-images.sh` script with
```bash
./copy-images.sh images.yaml
```

### Containerized

You can start the script inside the container from the current directory with
```bash
docker run -v $PWD:/app europe-docker.pkg.dev/gardener-project/releases/ci-infra/copy-images:latest /app/copy-images.sh /app/images.yaml
```

**Authentication**
In order to authenticate inside the container, mount your `~/.docker/config.json` to `/root/.docker/config.json`.
```bash
docker run -v $PWD:/app -v ~/.docker/config.json:/root/.docker/config.json europe-docker.pkg.dev/gardener-project/releases/ci-infra/copy-images:latest /app/copy-images.sh /app/images.yaml
```