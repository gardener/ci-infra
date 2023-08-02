# Image based on golang for gardener unit and integration tests
ARG GOLANG_IMAGE

FROM ${GOLANG_IMAGE} AS golang-test
# install gardener unit/integration test related dependencies
LABEL GOLANG_IMAGE=${GOLANG_IMAGE}
RUN set -eux; \
	apt-get update; \
	apt-get install -y --no-install-recommends \
		unzip \
		jq \
		parallel \
	; \
	rm -rf /var/lib/apt/lists/*
