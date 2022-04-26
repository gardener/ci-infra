# Image based on golang for gardener unit and integration tests
ARG GOLANG_VERSION=1.17.9

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
