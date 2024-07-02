# ----------------
# Build container
# ----------------
FROM golang:1.22.5 AS builder
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=$GOPROXY
LABEL stage=intermediate
# Copy entire repository to image
COPY . /code
WORKDIR /code
# Build go executables into binaries
RUN mkdir /build && GOBIN=/build \
    GO111MODULE=on CGO_ENABLED=0 GOOS=$(go env GOOS) GOARCH=$(go env GOARCH) go install ./...

# --------------------------
# Executable container base
# --------------------------

FROM gcr.io/distroless/static-debian12:nonroot AS base_nonroot

FROM alpine:3.20.1 AS ssl_git_runner
# Install SSL ca certificates
RUN apk add --no-cache ca-certificates git
# Create nonroot user and group to be used in executable containers
RUN addgroup -g 65532 -S nonroot && adduser -u 65532 -S nonroot -G nonroot
USER 65532

# ----------------------
# Executable containers
# ----------------------

FROM ssl_git_runner AS cherrypicker
LABEL app=cherrypicker
WORKDIR /
COPY --from=builder /build/cherrypicker /cherrypicker
ENTRYPOINT [ "/cherrypicker" ]

FROM ssl_git_runner AS job-forker
LABEL app=job-forker
WORKDIR /
COPY --from=builder /build/job-forker /job-forker
ENTRYPOINT [ "/job-forker" ]

FROM ssl_git_runner AS milestone-activator
LABEL app=milestone-activator
WORKDIR /
COPY --from=builder /build/milestone-activator /milestone-activator
ENTRYPOINT [ "/milestone-activator" ]

FROM base_nonroot AS cla-assistant
LABEL app=cla-assistant
WORKDIR /
COPY --from=builder /build/cla-assistant /cla-assistant
EXPOSE 8080
ENTRYPOINT [ "/cla-assistant" ]

FROM base_nonroot AS image-builder
LABEL app=image-builder
WORKDIR /
COPY --from=builder /build/image-builder /image-builder
ENTRYPOINT [ "/image-builder" ]

FROM ssl_git_runner AS release-handler
LABEL app=release-handler
WORKDIR /
COPY --from=builder /build/release-handler /release-handler
ENTRYPOINT [ "/release-handler" ]

FROM base_nonroot AS branch-cleaner
LABEL app=branch-cleaner
WORKDIR /
COPY --from=builder /build/branch-cleaner /branch-cleaner
ENTRYPOINT [ "/branch-cleaner" ]
