// Copyright (c) 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"os"
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/test-infra/prow/github/fakegithub"
)

var _ = Describe("activator", func() {

	const (
		noSectionFile               = "no-section"
		activeSectionFile           = "active-section"
		activeSectionNoOffsetFile   = "active-section-no-offset"
		activeSectionFile161        = "active-section-161"
		inactiveSectionFile         = "inactive-section"
		inactiveSectionNoOffsetFile = "inactive-section-no-offset"
		multipleStartsFile          = "multiple-starts"
		multipleEndsFile            = "multiple-ends"
		inconsistentOffsetsFile     = "inconsistent-offsets"

		noSectionContent = `tide:
  sync_period: 1m
  queries:
  - repos:
    - gardener/ci-infra
    labels:
    - lgtm
    - approved
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/needs-kind
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"
  - author: gardener-ci-robot
    repos:
    - gardener/ci-infra
    labels: # gardener-ci-robot should only create autobump PR with this label
    - skip-review
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"
  - repos:
    - gardener/gardener
    labels:
    - lgtm
    - approved
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/needs-kind
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"
  #   excludedBranches:
  #   - master
  # - repos:
  #   - gardener/gardener
  #   labels:
  #   - lgtm
  #   - approved
  #   - "cla: yes"
  #   missingLabels:
  #   - do-not-merge/blocked-paths
  #   - do-not-merge/contains-merge-commits
  #   - do-not-merge/hold
  #   - do-not-merge/invalid-commit-message
  #   - do-not-merge/invalid-owners-file
  #   - do-not-merge/needs-kind
  #   - do-not-merge/release-note-label-needed
  #   - do-not-merge/work-in-progress
  #   - needs-rebase
  #   - "cla: no"
  #   milestone: v1.60
  #   includedBranches:
  #   - master
  - repos:
    - gardener/gardener-extension-registry-cache
    labels:
    - lgtm
    - approved
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/needs-kind
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"

`

		activeSectionContent = `tide:
  sync_period: 1m
  queries:
  - repos:
    - gardener/ci-infra
    labels:
    - lgtm
    - approved
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/needs-kind
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"
  - author: gardener-ci-robot
    repos:
    - gardener/ci-infra
    labels: # gardener-ci-robot should only create autobump PR with this label
    - skip-review
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"
  - repos:
    - gardener/gardener
    labels:
    - lgtm
    - approved
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/needs-kind
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"
  #############################################################################
  ### Release Milestone section starts here
  #############################################################################
  # <-- /start: gardener-gardener-milestone -->
    excludedBranches:
    - master
  - repos:
    - gardener/gardener
    labels:
    - lgtm
    - approved
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/needs-kind
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"
    milestone: v1.60
    includedBranches:
    - master
  # <-- /end: gardener-gardener-milestone -->
  ##############################################################################
  ### End of Release Milestone section
  ##############################################################################
  - repos:
    - gardener/gardener-extension-registry-cache
    labels:
    - lgtm
    - approved
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/needs-kind
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"

`

		activeSectionNoOffsetContent = `tide:
  sync_period: 1m
  queries:
  - repos:
    - gardener/ci-infra
    labels:
    - lgtm
    - approved
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/needs-kind
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"
  - author: gardener-ci-robot
    repos:
    - gardener/ci-infra
    labels: # gardener-ci-robot should only create autobump PR with this label
    - skip-review
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"
  - repos:
    - gardener/gardener
    labels:
    - lgtm
    - approved
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/needs-kind
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"
  #############################################################################
  ### Release Milestone section starts here
  #############################################################################
# <-- /start: gardener-gardener-milestone -->
    excludedBranches:
    - master
  - repos:
    - gardener/gardener
    labels:
    - lgtm
    - approved
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/needs-kind
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"
    milestone: v1.60
    includedBranches:
    - master
# <-- /end: gardener-gardener-milestone -->
  ##############################################################################
  ### End of Release Milestone section
  ##############################################################################
  - repos:
    - gardener/gardener-extension-registry-cache
    labels:
    - lgtm
    - approved
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/needs-kind
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"

`

		activeSectionContent161 = `tide:
  sync_period: 1m
  queries:
  - repos:
    - gardener/ci-infra
    labels:
    - lgtm
    - approved
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/needs-kind
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"
  - author: gardener-ci-robot
    repos:
    - gardener/ci-infra
    labels: # gardener-ci-robot should only create autobump PR with this label
    - skip-review
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"
  - repos:
    - gardener/gardener
    labels:
    - lgtm
    - approved
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/needs-kind
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"
  #############################################################################
  ### Release Milestone section starts here
  #############################################################################
  # <-- /start: gardener-gardener-milestone -->
    excludedBranches:
    - master
  - repos:
    - gardener/gardener
    labels:
    - lgtm
    - approved
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/needs-kind
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"
    milestone: v1.61
    includedBranches:
    - master
  # <-- /end: gardener-gardener-milestone -->
  ##############################################################################
  ### End of Release Milestone section
  ##############################################################################
  - repos:
    - gardener/gardener-extension-registry-cache
    labels:
    - lgtm
    - approved
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/needs-kind
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"

`

		inactiveSectionContent = `tide:
  sync_period: 1m
  queries:
  - repos:
    - gardener/ci-infra
    labels:
    - lgtm
    - approved
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/needs-kind
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"
  - author: gardener-ci-robot
    repos:
    - gardener/ci-infra
    labels: # gardener-ci-robot should only create autobump PR with this label
    - skip-review
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"
  - repos:
    - gardener/gardener
    labels:
    - lgtm
    - approved
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/needs-kind
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"
  #############################################################################
  ### Release Milestone section starts here
  #############################################################################
  # <-- /start: gardener-gardener-milestone -->
  #   excludedBranches:
  #   - master
  # - repos:
  #   - gardener/gardener
  #   labels:
  #   - lgtm
  #   - approved
  #   - "cla: yes"
  #   missingLabels:
  #   - do-not-merge/blocked-paths
  #   - do-not-merge/contains-merge-commits
  #   - do-not-merge/hold
  #   - do-not-merge/invalid-commit-message
  #   - do-not-merge/invalid-owners-file
  #   - do-not-merge/needs-kind
  #   - do-not-merge/release-note-label-needed
  #   - do-not-merge/work-in-progress
  #   - needs-rebase
  #   - "cla: no"
  #   milestone: v1.60
  #   includedBranches:
  #   - master
  # <-- /end: gardener-gardener-milestone -->
  ##############################################################################
  ### End of Release Milestone section
  ##############################################################################
  - repos:
    - gardener/gardener-extension-registry-cache
    labels:
    - lgtm
    - approved
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/needs-kind
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"

`

		inactiveSectionNoOffsetContent = `tide:
  sync_period: 1m
  queries:
  - repos:
    - gardener/ci-infra
    labels:
    - lgtm
    - approved
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/needs-kind
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"
  - author: gardener-ci-robot
    repos:
    - gardener/ci-infra
    labels: # gardener-ci-robot should only create autobump PR with this label
    - skip-review
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"
  - repos:
    - gardener/gardener
    labels:
    - lgtm
    - approved
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/needs-kind
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"
  #############################################################################
  ### Release Milestone section starts here
  #############################################################################
# <-- /start: gardener-gardener-milestone -->
#     excludedBranches:
#     - master
#   - repos:
#     - gardener/gardener
#     labels:
#     - lgtm
#     - approved
#     - "cla: yes"
#     missingLabels:
#     - do-not-merge/blocked-paths
#     - do-not-merge/contains-merge-commits
#     - do-not-merge/hold
#     - do-not-merge/invalid-commit-message
#     - do-not-merge/invalid-owners-file
#     - do-not-merge/needs-kind
#     - do-not-merge/release-note-label-needed
#     - do-not-merge/work-in-progress
#     - needs-rebase
#     - "cla: no"
#     milestone: v1.60
#     includedBranches:
#     - master
# <-- /end: gardener-gardener-milestone -->
  ##############################################################################
  ### End of Release Milestone section
  ##############################################################################
  - repos:
    - gardener/gardener-extension-registry-cache
    labels:
    - lgtm
    - approved
    - "cla: yes"
    missingLabels:
    - do-not-merge/blocked-paths
    - do-not-merge/contains-merge-commits
    - do-not-merge/hold
    - do-not-merge/invalid-commit-message
    - do-not-merge/invalid-owners-file
    - do-not-merge/needs-kind
    - do-not-merge/release-note-label-needed
    - do-not-merge/work-in-progress
    - needs-rebase
    - "cla: no"

`

		multipleStartsContent = `foo
    # <-- /start: bar -->
    # foo
    # foo
    # bar
    # <-- /start: bar -->
    # bar
    # bar
    # <-- /end: bar -->
    bar
`

		multipleEndsContent = `foo
# <-- /start: bar -->
# foo
# foo
# bar
# <-- /end: bar -->
# bar
# bar
# <-- /end: bar -->
bar
`

		inconsistentOffsetsContent = `foo
# <-- /start: bar -->
# foo
# foo
# bar
  # <-- /end: bar -->
bar
`
	)

	var (
		testFileNames    map[string]string
		testFileContent  map[string]string
		fakeGithubClient *fakegithub.FakeClient
	)

	BeforeEach(func() {
		By("Create test files")
		testFileNames = map[string]string{}
		testFileContent = map[string]string{
			noSectionFile:               noSectionContent,
			activeSectionFile:           activeSectionContent,
			activeSectionFile161:        activeSectionContent161,
			activeSectionNoOffsetFile:   activeSectionNoOffsetContent,
			inactiveSectionFile:         inactiveSectionContent,
			inactiveSectionNoOffsetFile: inactiveSectionNoOffsetContent,
			multipleStartsFile:          multipleStartsContent,
			multipleEndsFile:            multipleEndsContent,
			inconsistentOffsetsFile:     inconsistentOffsetsContent,
		}
		for f, c := range testFileContent {
			file, err := os.CreateTemp("", f)
			Expect(err).NotTo(HaveOccurred())
			testFileNames[f] = file.Name()
			_, err = file.WriteString(c)
			Expect(err).NotTo(HaveOccurred())
		}

		DeferCleanup(func() {
			By("Remove test files")
			for _, f := range testFileNames {
				err := os.Remove(f)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		By("Create fake github client")
		fakeGithubClient = fakegithub.NewFakeClient()
	})

	testHandleMilestoneSection := func(filename, sectionIdentifier, milestone, expectedContent string, changes bool) {
		By("Run handleMilestoneSection")
		Expect(handleMilestoneSection(filename, sectionIdentifier, milestone)).To(Equal(changes))

		By("Validate file")
		content, err := os.ReadFile(filename)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(Equal(expectedContent))
	}

	testUpdateFiles := func(filename, sectionIdentifier, milestone, expectedContent string, changes bool) {
		directory := path.Dir(filename)
		file := path.Base(filename)
		By("Run updateFiles")
		Expect(updateFiles(directory, []string{file}, sectionIdentifier, milestone)).To(Equal(changes))

		By("Validate file")
		content, err := os.ReadFile(filename)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(Equal(expectedContent))
	}

	Context("handleMilestoneSection", func() {
		It("should change nothing if there is no section", func() {
			testHandleMilestoneSection(testFileNames[noSectionFile], "gardener-gardener-milestone", "v1.60", noSectionContent, false)
		})
		It("should not change an active section if the section does not match", func() {
			testHandleMilestoneSection(testFileNames[activeSectionFile], "bar", "", activeSectionContent, false)
		})
		It("should not change an inactive section if the section does not match", func() {
			testHandleMilestoneSection(testFileNames[inactiveSectionFile], "bar", "v1.60", inactiveSectionContent, false)
		})
		It("should not change an active section if it was already changed", func() {
			testHandleMilestoneSection(testFileNames[activeSectionFile], "gardener-gardener-milestone", "v1.60", activeSectionContent, false)
		})
		It("should not change an inactive section if it was already changed", func() {
			testHandleMilestoneSection(testFileNames[inactiveSectionFile], "gardener-gardener-milestone", "", inactiveSectionContent, false)
		})
		It("should inactivate active section if the section matches", func() {
			testHandleMilestoneSection(testFileNames[activeSectionFile], "gardener-gardener-milestone", "", inactiveSectionContent, true)
		})
		It("should activate inactive section if the section matches", func() {
			testHandleMilestoneSection(testFileNames[inactiveSectionFile], "gardener-gardener-milestone", "v1.60", activeSectionContent, true)
		})
		It("should activate inactive section and set the right milestone", func() {
			testHandleMilestoneSection(testFileNames[inactiveSectionFile], "gardener-gardener-milestone", "v1.61", activeSectionContent161, true)
		})
		It("should change the milestone when the section is already active", func() {
			testHandleMilestoneSection(testFileNames[activeSectionFile], "gardener-gardener-milestone", "v1.61", activeSectionContent161, true)
		})
		It("should activate inactive section with leading spaces if the section matches", func() {
			testHandleMilestoneSection(testFileNames[inactiveSectionNoOffsetFile], "gardener-gardener-milestone", "v1.60", activeSectionNoOffsetContent, true)
		})
		It("should fail if the offsets of section start and end are inconsistent", func() {
			_, err := handleMilestoneSection(testFileNames[inconsistentOffsetsFile], "bar", "v1.60")
			Expect(err).To(MatchError("inconsistent number of leading spaces at start and end of section identifiers"))
		})
	})

	Context("findSectionsInFile", func() {
		It("should return true if the section was found", func() {
			Expect(findSectionsInFile(testFileNames[activeSectionFile], "gardener-gardener-milestone")).To(BeTrue())
		})
		It("should return false if the section was not found", func() {
			Expect(findSectionsInFile(testFileNames[activeSectionFile], "bar")).To(BeFalse())
		})
		It("should return false if there is no section", func() {
			Expect(findSectionsInFile(testFileNames[noSectionFile], "bar")).To(BeFalse())
		})
		It("should fail if there are multiple consecutive starts of section", func() {
			_, err := findSectionsInFile(testFileNames[multipleStartsFile], "bar")
			Expect(err).To(MatchError(fmt.Sprintf("file %s: found second consecutive start of section identifier \"bar\" in line 6", testFileNames[multipleStartsFile])))
		})
		It("should fail if there are multiple consecutive ends of section", func() {
			_, err := findSectionsInFile(testFileNames[multipleEndsFile], "bar")
			Expect(err).To(MatchError(fmt.Sprintf("file %s: found an end of section identifier \"bar\" in line 9 without start of section", testFileNames[multipleEndsFile])))
		})
	})

	Context("getOpenMilestone", func() {
		It("should find an open milestone with matching pattern", func() {
			fakeGithubClient.MilestoneMap["bar"] = 1
			fakeGithubClient.MilestoneMap["v1.60"] = 2
			Expect(getOpenMilestone(fakeGithubClient, "foo", "bar", "v\\d+\\.\\d+")).To(Equal("v1.60"))
		})
		It("should return an empty string if no open milestone is matching the pattern", func() {
			fakeGithubClient.MilestoneMap["bar"] = 1
			Expect(getOpenMilestone(fakeGithubClient, "foo", "bar", "v\\d+\\.\\d+")).To(Equal(""))
		})
		It("should fail if there is more than an open milestone with matching pattern", func() {
			fakeGithubClient.MilestoneMap["v1.59"] = 1
			fakeGithubClient.MilestoneMap["v1.60"] = 2
			_, err := getOpenMilestone(fakeGithubClient, "foo", "bar", "v\\d+\\.\\d+")
			Expect(err).To(MatchError(fmt.Sprintf("found more than one open milestone of pattern %q", "v\\d+\\.\\d+")))
		})
	})

	Context("updateFiles", func() {
		It("should change nothing if there is no section", func() {
			testUpdateFiles(testFileNames[noSectionFile], "gardener-gardener-milestone", "v1.60", noSectionContent, false)
		})
		It("should not change an active section if the section does not match", func() {
			testUpdateFiles(testFileNames[activeSectionFile], "bar", "", activeSectionContent, false)
		})
		It("should not change an inactive section if the section does not match", func() {
			testUpdateFiles(testFileNames[inactiveSectionFile], "bar", "v1.60", inactiveSectionContent, false)
		})
		It("should not change an active section if it was already changed", func() {
			testUpdateFiles(testFileNames[activeSectionFile], "gardener-gardener-milestone", "v1.60", activeSectionContent, false)
		})
		It("should not change an inactive section if it was already changed", func() {
			testUpdateFiles(testFileNames[inactiveSectionFile], "gardener-gardener-milestone", "", inactiveSectionContent, false)
		})
		It("should inactivate active section if the section matches", func() {
			testUpdateFiles(testFileNames[activeSectionFile], "gardener-gardener-milestone", "", inactiveSectionContent, true)
		})
		It("should activate inactive section if the section matches", func() {
			testUpdateFiles(testFileNames[inactiveSectionFile], "gardener-gardener-milestone", "v1.60", activeSectionContent, true)
		})
		It("should activate inactive section and set the right milestone", func() {
			testUpdateFiles(testFileNames[inactiveSectionFile], "gardener-gardener-milestone", "v1.61", activeSectionContent161, true)
		})
		It("should change the milestone when the section is already active", func() {
			testUpdateFiles(testFileNames[activeSectionFile], "gardener-gardener-milestone", "v1.61", activeSectionContent161, true)
		})
		It("should activate inactive section with leading spaces if the section matches", func() {
			testUpdateFiles(testFileNames[inactiveSectionNoOffsetFile], "gardener-gardener-milestone", "v1.60", activeSectionNoOffsetContent, true)
		})
		It("should fail if the offsets of section start and end are inconsistent", func() {
			directory := path.Dir(testFileNames[inconsistentOffsetsFile])
			file := path.Base(testFileNames[inconsistentOffsetsFile])
			_, err := updateFiles(directory, []string{file}, "bar", "v1.60")
			Expect(err).To(MatchError("inconsistent number of leading spaces at start and end of section identifiers"))
		})
	})
})
