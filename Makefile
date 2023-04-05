# Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


REGISTRY          := $(shell cat .REGISTRY 2>/dev/null)
PUSH_LATEST_TAG   := true
GOLANG_VERSION    := 1.19.8
VERSION           := v$(shell date '+%Y%m%d')-$(shell git rev-parse --short HEAD)

IMG_GOLANG_TEST := golang-test
REG_GOLANG_TEST := $(REGISTRY)/$(IMG_GOLANG_TEST)
IMG_CHERRYPICKER := cherrypicker
REG_CHERRYPICKER := $(REGISTRY)/$(IMG_CHERRYPICKER)
IMG_CLA_ASSISTANT := cla-assistant
REG_CLA_ASSISTANT := $(REGISTRY)/$(IMG_CLA_ASSISTANT)
IMG_IMAGE_BUILDER := image-builder
REG_IMAGE_BUILDER := $(REGISTRY)/$(IMG_IMAGE_BUILDER)
IMG_JOB_FORKER := job-forker
REG_JOB_FORKER := $(REGISTRY)/$(IMG_JOB_FORKER)
IMG_MILESTONE_ACTIVATOR := milestone-activator
REG_MILESTONE_ACTIVATOR  := $(REGISTRY)/$(IMG_MILESTONE_ACTIVATOR)


#########################################
# Tools                                 #
#########################################

TOOLS_DIR := hack/tools
include hack/tools.mk


#################################################################
# Rules related to binary build, Docker image build and release #
#################################################################
.PHONY: docker-images
docker-images:
ifeq ("$(REGISTRY)", "")
	@echo "Please set your docker registry in REGISTRY variable or .REGISTRY file first."; false;
endif
	@echo "Building docker golang image for tests with version and tag $(GOLANG_VERSION)"
	@docker build --build-arg GOLANG_VERSION=$(GOLANG_VERSION) -t $(REG_GOLANG_TEST):$(GOLANG_VERSION) -t $(REG_GOLANG_TEST):latest -f images/golang-test/Dockerfile --target $(IMG_GOLANG_TEST) .
	@echo "Building docker images with version and tag $(VERSION)"
	@docker build --build-arg GOLANG_VERSION=$(GOLANG_VERSION) -t $(REG_CHERRYPICKER):$(VERSION) -t $(REG_CHERRYPICKER):latest -f Dockerfile --target $(IMG_CHERRYPICKER) .
	@docker build --build-arg GOLANG_VERSION=$(GOLANG_VERSION) -t $(REG_CLA_ASSISTANT):$(VERSION) -t $(REG_CLA_ASSISTANT):latest -f Dockerfile --target $(IMG_CLA_ASSISTANT) .
	@docker build --build-arg GOLANG_VERSION=$(GOLANG_VERSION) -t $(REG_IMAGE_BUILDER):$(VERSION) -t $(REG_IMAGE_BUILDER):latest -f Dockerfile --target $(IMG_IMAGE_BUILDER) .
	@docker build --build-arg GOLANG_VERSION=$(GOLANG_VERSION) -t $(REG_JOB_FORKER):$(VERSION) -t $(REG_JOB_FORKER):latest -f Dockerfile --target $(IMG_JOB_FORKER) .
	@docker build --build-arg GOLANG_VERSION=$(GOLANG_VERSION) -t $(REG_MILESTONE_ACTIVATOR):$(VERSION) -t $(REG_MILESTONE_ACTIVATOR):latest -f Dockerfile --target $(IMG_MILESTONE_ACTIVATOR) .

.PHONY: docker-push
docker-push:
ifeq ("$(REGISTRY)", "")
	@echo "Please set your docker registry in REGISTRY variable or .REGISTRY file first."; false;
endif
	@if ! docker images $(REG_GOLANG_TEST) | awk '{ print $$2 }' | grep -q -F $(GOLANG_VERSION); then echo "$(REG_GOLANG_TEST) version $(GOLANG_VERSION) is not yet built. Please run 'make docker-images'"; false; fi
	@docker push $(REG_GOLANG_TEST):$(GOLANG_VERSION)
	@if ! docker images $(REG_CHERRYPICKER) | awk '{ print $$2 }' | grep -q -F $(VERSION); then echo "$(REG_CHERRYPICKER) version $(VERSION) is not yet built. Please run 'make docker-images'"; false; fi
	@if ! docker images $(REG_CLA_ASSISTANT) | awk '{ print $$2 }' | grep -q -F $(VERSION); then echo "$(REG_CLA_ASSISTANT) version $(VERSION) is not yet built. Please run 'make docker-images'"; false; fi
	@if ! docker images $(REG_IMAGE_BUILDER) | awk '{ print $$2 }' | grep -q -F $(VERSION); then echo "$(REG_IMAGE_BUILDER) version $(VERSION) is not yet built. Please run 'make docker-images'"; false; fi
	@if ! docker images $(REG_JOB_FORKER) | awk '{ print $$2 }' | grep -q -F $(VERSION); then echo "$(REG_JOB_FORKER) version $(VERSION) is not yet built. Please run 'make docker-images'"; false; fi
	@if ! docker images $(REG_MILESTONE_ACTIVATOR) | awk '{ print $$2 }' | grep -q -F $(VERSION); then echo "$(REG_MILESTONE_ACTIVATOR) version $(VERSION) is not yet built. Please run 'make docker-images'"; false; fi
	@docker push $(REG_CHERRYPICKER):$(VERSION)
	@docker push $(REG_CLA_ASSISTANT):$(VERSION)
	@docker push $(REG_IMAGE_BUILDER):$(VERSION)
	@docker push $(REG_JOB_FORKER):$(VERSION)
	@docker push $(REG_MILESTONE_ACTIVATOR):$(VERSION)
ifeq ("$(PUSH_LATEST_TAG)", "true")
	@docker push $(REG_GOLANG_TEST):latest
	@docker push $(REG_CHERRYPICKER):latest
	@docker push $(REG_CLA_ASSISTANT):latest
	@docker push $(REG_IMAGE_BUILDER):latest
	@docker push $(REG_JOB_FORKER):latest
	@docker push $(REG_MILESTONE_ACTIVATOR):latest
endif


#####################################################################
# Rules for verification, formatting, linting, testing and cleaning #
#####################################################################

.PHONY: check
check: $(GOIMPORTS) $(GOLANGCI_LINT)
	@hack/check.sh --golangci-lint-config=./.golangci.yaml ./prow/...

.PHONY: revendor
revendor:
	@GO111MODULE=on go mod tidy
	@GO111MODULE=on go mod vendor

.PHONY: verify-vendor
verify-vendor: revendor
	@if !(git diff --quiet HEAD -- go.sum go.mod vendor); then \
		echo "go module files or vendor folder are out of date, please run 'make revendor'"; exit 1; \
	fi

.PHONY: test
test:
	@./hack/test.sh ./prow/...

.PHONY: test-cov
test-cov:
	@./hack/test-cover.sh ./prow/...
