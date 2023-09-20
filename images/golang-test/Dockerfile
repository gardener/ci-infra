# gardenertools image
ARG gardenertoolsimage

FROM ${gardenertoolsimage} AS gardenertools

# Image based on golang for gardener unit and integration tests
ARG image

FROM ${image} AS golang-test
# install gardener unit/integration test related dependencies
LABEL GOLANG_IMAGE=${image}

COPY --from=gardenertools /gardenertools /gardenertools

RUN set -eux; \
	apt-get update; \
	apt-get install -y --no-install-recommends \
		unzip \
		jq \
		parallel \
	; \
	rm -rf /var/lib/apt/lists/*
