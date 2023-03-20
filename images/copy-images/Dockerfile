ARG ALPINE_VERSION=3.17

FROM alpine:$ALPINE_VERSION
LABEL ALPINE_VERSION=$ALPINE_VERSION
ARG CRANE_VERSION=v0.14.0
LABEL CRANE_VERSION=$CRANE_VERSION
RUN set -eux; \
    apk update; \
    apk add --no-cache yq \
    curl; \
    curl -L "https://github.com/google/go-containerregistry/releases/download/$CRANE_VERSION/go-containerregistry_Linux_amd64.tar.gz" -O; \
    tar -xf go-containerregistry_Linux_arm64.tar.gz -C /usr/local/bin; \
    crane version