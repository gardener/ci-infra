ARG ALPINE_VERSION=3.17.2

FROM alpine:$ALPINE_VERSION
LABEL ALPINE_VERSION=$ALPINE_VERSION
ARG CRANE_VERSION=v0.14.0
LABEL CRANE_VERSION=$CRANE_VERSION
RUN set -eux; \
    apk add --no-cache yq; \
    wget -O /tmp/go-containerregistry.tar.gz "https://github.com/google/go-containerregistry/releases/download/$CRANE_VERSION/go-containerregistry_Linux_x86_64.tar.gz"; \
    tar -xf /tmp/go-containerregistry.tar.gz -C /usr/local/bin; \
    rm /tmp/go-containerregistry.tar.gz; \
    crane version