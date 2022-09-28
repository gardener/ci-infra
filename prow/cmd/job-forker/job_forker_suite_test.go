package main

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
)

var (
	fakedVersion    = "1-53"
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
	branches = []github.Branch{
		{
			Name: "release-v1.00",
		},
		{
			Name: "release-v1.01",
		},
		{
			Name: "release-v1.02",
		},
	}
)

func TestJobForker(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "JobForker Suite")
}

var _ = Describe("Testing generatePeriodics()", func() {
	It("should generate 1 valid forked Periodic", func() {
		result := generatePeriodics(periodic, fakedVersion, fakedRepository)
		Expect(len(result)).To(Equal(1))
	})
})

var _ = Describe("Testing generatePostsubmits()", func() {
	It("should generate 1 valid forked Postsubmit", func() {
		result := generatePostsubmits(postsubmit, fakedVersion, fakedRepository)
		Expect(len(result)).To(Equal(1))
	})
})

var _ = Describe("Testing generatePresubmits()", func() {
	It("should generate 1 valid forked Presubmit", func() {
		result := generatePresubmits(presubmit, fakedVersion, fakedRepository)
		Expect(len(result)).To(Equal(1))
	})
})

var _ = Describe("Testing generateVersionsFromBranches()", func() {
	It("should correctly extract the versions from the releaseBranaches", func() {
		versions := generateVersionsFromBranches(branches, BranchPrefix)
		Expect(versions).To(BeEquivalentTo([]string{
			"v1.00",
			"v1.01",
			"v1.02",
		}))
	})
})
