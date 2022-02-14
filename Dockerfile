# ----------------
# Build container
# ----------------

FROM golang:1.17.6 AS builder
LABEL stage=intermediate
ARG OS
ARG ARCH
# Copy entire repository to image
COPY . /code
WORKDIR /code
# Build go executables into binaries
RUN mkdir /build && GOBIN=/build \
    GO111MODULE=on CGO_ENABLED=0 GOOS=${OS} GOARCH=${ARCH} go install -mod vendor -a ./...

# --------------------------
# Executable container base
# --------------------------

FROM alpine:3.15.0 AS ssl_runner
# Install SSL ca certificates
RUN apk add --no-cache ca-certificates
# Create nonroot user and group to be used in executable containers
RUN addgroup -g 65532 -S nonroot && adduser -u 65532 -S nonroot -G nonroot
USER 65532

# ----------------------
# Executable containers
# ----------------------

FROM ssl_runner AS cla-assistant
ARG VERSION
LABEL app=cla-assistant
LABEL version=${VERSION}
WORKDIR /
COPY --from=builder /build/cla-assistant /cla-assistant
EXPOSE 8080
ENTRYPOINT [ "/cla-assistant" ]
