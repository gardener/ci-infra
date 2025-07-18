FROM alpine:3.22.1 AS copy-images
# renovate: datasource=github-releases depName=google/go-containerregistry
ARG CRANE_VERSION=v0.16.1
LABEL CRANE_VERSION=$CRANE_VERSION
RUN set -eux; \
    apk add --no-cache yq; \
    wget -O /tmp/go-containerregistry.tar.gz "https://github.com/google/go-containerregistry/releases/download/$CRANE_VERSION/go-containerregistry_Linux_x86_64.tar.gz"; \
    tar -xf /tmp/go-containerregistry.tar.gz -C /usr/local/bin; \
    rm /tmp/go-containerregistry.tar.gz; \
    crane version
