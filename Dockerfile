# ----------------
# Build container
# ----------------
ARG GOLANG_VERSION=1.17.9

FROM golang:${GOLANG_VERSION} AS builder
LABEL stage=intermediate
# Copy entire repository to image
COPY . /code
WORKDIR /code
# Build go executables into binaries
RUN mkdir /build && GOBIN=/build \
    GO111MODULE=on CGO_ENABLED=0 GOOS=$(go env GOOS) GOARCH=$(go env GOARCH) go install -mod vendor -a ./...

# --------------------------
# Executable container base
# --------------------------

FROM alpine:3.15.4 AS ssl_runner
# Install SSL ca certificates
RUN apk add --no-cache ca-certificates
# Create nonroot user and group to be used in executable containers
RUN addgroup -g 65532 -S nonroot && adduser -u 65532 -S nonroot -G nonroot
USER 65532

# ----------------------
# Executable containers
# ----------------------

FROM golang:${GOLANG_VERSION}-bullseye AS golang-test
# install gardener unit/integration test related dependencies
LABEL GOLANG_VERSION=${GOLANG_VERSION}
RUN set -eux; \
	apt-get update; \
	apt-get install -y --no-install-recommends \
		unzip \
		jq \
		parallel \
	; \
	rm -rf /var/lib/apt/lists/*

FROM ssl_runner AS cla-assistant
LABEL app=cla-assistant
WORKDIR /
COPY --from=builder /build/cla-assistant /cla-assistant
EXPOSE 8080
ENTRYPOINT [ "/cla-assistant" ]

FROM ssl_runner AS image-builder
LABEL app=image-builder
WORKDIR /
COPY --from=builder /build/image-builder /image-builder
ENTRYPOINT [ "/image-builder" ]
