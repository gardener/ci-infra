# Image based on golang for gardener unit and integration tests
ARG image

FROM ${image} AS golang-test
# install gardener unit/integration test related dependencies
LABEL GOLANG_IMAGE=${image}
RUN set -eux; \
	apt-get update; \
	apt-get install -y --no-install-recommends \
		unzip \
		jq \
		parallel \
	; \
	rm -rf /var/lib/apt/lists/*
