// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
)

var (
	fakedBranch     = github.Branch{Name: "release-v1.01"}
	fakedOrg        = "gardener"
	fakedRepo       = "gardener"
	fakedRepository = fakedOrg + "/" + fakedRepo
	fakedPath       = "config/jobs"
	periodic        = config.JobConfig{
		Periodics: []config.Periodic{
			{
				JobBase: config.JobBase{
					Name:       "testPeriodicThatShouldBeForked",
					SourcePath: fakedPath,
					Annotations: map[string]string{
						"fork-per-release": "true",
					},
					UtilityConfig: config.UtilityConfig{
						ExtraRefs: []prowapi.Refs{
							{
								Org:  fakedOrg,
								Repo: fakedRepo,
							},
						},
					},
				},
			},
			{
				JobBase: config.JobBase{
					Name:        "testPeriodicThatShouldNotBeForked",
					SourcePath:  fakedPath,
					Annotations: map[string]string{},
					UtilityConfig: config.UtilityConfig{
						ExtraRefs: []prowapi.Refs{
							{
								Org:  fakedOrg,
								Repo: fakedRepo,
							},
						},
					},
				},
			},
			{
				JobBase: config.JobBase{
					Name:       "testPeriodicThatShouldNotBeForked2",
					SourcePath: fakedPath,
					Annotations: map[string]string{
						"fork-per-release": "true",
					},
					UtilityConfig: config.UtilityConfig{
						ExtraRefs: []prowapi.Refs{
							{
								Org:  fakedOrg + "ButNotReally",
								Repo: fakedRepo,
							},
						},
					},
				},
			},
		},
	}

	postsubmit = config.JobConfig{
		PostsubmitsStatic: map[string][]config.Postsubmit{
			fakedRepository: {
				{
					JobBase: config.JobBase{
						Name:       "testPostsubmitThatShouldBeForked",
						SourcePath: fakedPath,
						Annotations: map[string]string{
							"fork-per-release": "true",
						},
					},
				},
				{
					JobBase: config.JobBase{
						Name:        "testPostsubmitThatShouldNotBeForked",
						SourcePath:  fakedPath,
						Annotations: map[string]string{},
					},
				},
			},
			fakedRepository + "ButNotReally": {
				{
					JobBase: config.JobBase{
						Name:       "testPostsubmitThatShouldNotBeForked2",
						SourcePath: fakedPath,
						Annotations: map[string]string{
							"fork-per-release": "true",
						},
					},
				},
			},
		},
	}

	presubmit = config.JobConfig{
		PresubmitsStatic: map[string][]config.Presubmit{
			fakedRepository: {
				{
					JobBase: config.JobBase{
						Name:       "testPostsubmitThatShouldBeForked",
						SourcePath: fakedPath,
						Annotations: map[string]string{
							"fork-per-release": "true",
						},
					},
				},
				{
					JobBase: config.JobBase{
						Name:        "testPostsubmitThatShouldNotBeForked",
						SourcePath:  fakedPath,
						Annotations: map[string]string{},
					},
				},
			},
			fakedRepository + "ButNotReally": {
				{
					JobBase: config.JobBase{
						Name:       "testPostsubmitThatShouldNotBeForked2",
						SourcePath: fakedPath,
						Annotations: map[string]string{
							"fork-per-release": "true",
						},
					},
				},
			},
		},
	}
)

var _ = Describe("Testing generatePeriodics()", func() {
	It("should generate 1 valid forked Periodic", func() {
		result := generatePeriodics(periodic, fakedRepository, fakedBranch)
		Expect(len(result)).To(Equal(1))
	})
})

var _ = Describe("Testing generatePostsubmits()", func() {
	It("should generate 1 valid forked Postsubmit", func() {
		result := generatePostsubmits(postsubmit, fakedRepository, fakedBranch)
		Expect(len(result)).To(Equal(1))
	})
})

var _ = Describe("Testing generatePresubmits()", func() {
	It("should generate 1 valid forked Presubmit", func() {
		result := generatePresubmits(presubmit, fakedRepository, fakedBranch)
		Expect(len(result)).To(Equal(1))
	})
})
