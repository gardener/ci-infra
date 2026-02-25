FROM alpine:3.23.3 AS copy-images
# renovate: datasource=github-releases depName=google/go-containerregistry
ARG CRANE_VERSION=v0.21.1
ARG TARGETARCH
LABEL CRANE_VERSION=$CRANE_VERSION
RUN set -eux; \
    echo "Building for target arch: ${TARGETARCH}"; \
    apk add --no-cache yq; \
    case "$TARGETARCH" in \
        amd64) ARCH=x86_64 ;; \
        arm64) ARCH=arm64 ;; \
        *) ARCH=$TARGETARCH ;; \
    esac; \
    wget -O /tmp/go-containerregistry.tar.gz "https://github.com/google/go-containerregistry/releases/download/$CRANE_VERSION/go-containerregistry_linux_${ARCH}.tar.gz"; \
    tar -xf /tmp/go-containerregistry.tar.gz -C /usr/local/bin; \
    rm /tmp/go-containerregistry.tar.gz; \
    crane version
