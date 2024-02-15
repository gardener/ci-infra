# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0



REGISTRY            := $(shell cat .REGISTRY 2>/dev/null)
PUSH_LATEST_TAG     := true
VERSION             := v$(shell date '+%Y%m%d')-$(shell git rev-parse --short HEAD)

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
IMG_RELEASE_HANDLER := release-handler
REG_RELEASE_HANDLER  := $(REGISTRY)/$(IMG_RELEASE_HANDLER)
IMG_BRANCH_CLEANER := branch-cleaner
REG_BRANCH_CLEANER  := $(REGISTRY)/$(IMG_BRANCH_CLEANER)


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
	@echo "Building docker images with version and tag $(VERSION)"
	@docker build -t $(REG_CHERRYPICKER):$(VERSION) -t $(REG_CHERRYPICKER):latest -f Dockerfile --target $(IMG_CHERRYPICKER) .
	@docker build -t $(REG_CLA_ASSISTANT):$(VERSION) -t $(REG_CLA_ASSISTANT):latest -f Dockerfile --target $(IMG_CLA_ASSISTANT) .
	@docker build -t $(REG_IMAGE_BUILDER):$(VERSION) -t $(REG_IMAGE_BUILDER):latest -f Dockerfile --target $(IMG_IMAGE_BUILDER) .
	@docker build -t $(REG_JOB_FORKER):$(VERSION) -t $(REG_JOB_FORKER):latest -f Dockerfile --target $(IMG_JOB_FORKER) .
	@docker build -t $(REG_MILESTONE_ACTIVATOR):$(VERSION) -t $(REG_MILESTONE_ACTIVATOR):latest -f Dockerfile --target $(IMG_MILESTONE_ACTIVATOR) .
	@docker build -t $(REG_RELEASE_HANDLER):$(VERSION) -t $(REG_RELEASE_HANDLER):latest -f Dockerfile --target $(IMG_RELEASE_HANDLER) .
	@docker build -t $(REG_BRANCH_CLEANER):$(VERSION) -t $(REG_BRANCH_CLEANER):latest -f Dockerfile --target $(IMG_BRANCH_CLEANER) .

.PHONY: docker-push
docker-push:
ifeq ("$(REGISTRY)", "")
	@echo "Please set your docker registry in REGISTRY variable or .REGISTRY file first."; false;
endif
	@if ! docker images $(REG_CHERRYPICKER) | awk '{ print $$2 }' | grep -q -F $(VERSION); then echo "$(REG_CHERRYPICKER) version $(VERSION) is not yet built. Please run 'make docker-images'"; false; fi
	@if ! docker images $(REG_CLA_ASSISTANT) | awk '{ print $$2 }' | grep -q -F $(VERSION); then echo "$(REG_CLA_ASSISTANT) version $(VERSION) is not yet built. Please run 'make docker-images'"; false; fi
	@if ! docker images $(REG_IMAGE_BUILDER) | awk '{ print $$2 }' | grep -q -F $(VERSION); then echo "$(REG_IMAGE_BUILDER) version $(VERSION) is not yet built. Please run 'make docker-images'"; false; fi
	@if ! docker images $(REG_JOB_FORKER) | awk '{ print $$2 }' | grep -q -F $(VERSION); then echo "$(REG_JOB_FORKER) version $(VERSION) is not yet built. Please run 'make docker-images'"; false; fi
	@if ! docker images $(REG_MILESTONE_ACTIVATOR) | awk '{ print $$2 }' | grep -q -F $(VERSION); then echo "$(REG_MILESTONE_ACTIVATOR) version $(VERSION) is not yet built. Please run 'make docker-images'"; false; fi
	@if ! docker images $(REG_RELEASE_HANDLER) | awk '{ print $$2 }' | grep -q -F $(VERSION); then echo "$(REG_RELEASE_HANDLER) version $(VERSION) is not yet built. Please run 'make docker-images'"; false; fi
	@if ! docker images $(REG_BRANCH_CLEANER) | awk '{ print $$2 }' | grep -q -F $(VERSION); then echo "$(REG_BRANCH_CLEANER) version $(VERSION) is not yet built. Please run 'make docker-images'"; false; fi
	@docker push $(REG_CHERRYPICKER):$(VERSION)
	@docker push $(REG_CLA_ASSISTANT):$(VERSION)
	@docker push $(REG_IMAGE_BUILDER):$(VERSION)
	@docker push $(REG_JOB_FORKER):$(VERSION)
	@docker push $(REG_MILESTONE_ACTIVATOR):$(VERSION)
	@docker push $(REG_RELEASE_HANDLER):$(VERSION)
	@docker push $(REG_BRANCH_CLEANER):$(VERSION)
ifeq ("$(PUSH_LATEST_TAG)", "true")
	@docker push $(REG_CHERRYPICKER):latest
	@docker push $(REG_CLA_ASSISTANT):latest
	@docker push $(REG_IMAGE_BUILDER):latest
	@docker push $(REG_JOB_FORKER):latest
	@docker push $(REG_MILESTONE_ACTIVATOR):latest
	@docker push $(REG_RELEASE_HANDLER):latest
	@docker push $(REG_BRANCH_CLEANER):latest
endif


#####################################################################
# Rules for verification, formatting, linting, testing and cleaning #
#####################################################################

.PHONY: check
check: $(GOIMPORTS) $(GOLANGCI_LINT)
	@hack/check.sh --golangci-lint-config=./.golangci.yaml ./prow/...

.PHONY: tidy
tidy:
	@echo "> Tidy"
	@GO111MODULE=on go mod tidy

.PHONY: test
test:
	@./hack/test.sh ./prow/...

.PHONY: test-cov
test-cov:
	@./hack/test-cover.sh ./prow/...

.PHONY: verify
verify: check test verify-tidy

.PHONY: verify-tidy
verify-tidy: tidy
	@echo "> Verify tidy"
	@if !(git diff --quiet HEAD -- go.sum go.mod); then \
		echo "go module files are out of date, please run 'make tidy'"; exit 1; \
	fi
