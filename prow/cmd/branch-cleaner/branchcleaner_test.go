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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

type fakeGithubClient struct {
	*fakegithub.FakeClient
	Branches []github.Branch
}

func (f *fakeGithubClient) GetBranches(org, repo string, onlyProtected bool) ([]github.Branch, error) {
	var branches []github.Branch
	for _, branch := range f.Branches {
		if branch.Protected == onlyProtected {
			branches = append(branches, branch)
		}
	}
	return branches, nil
}

func (f *fakeGithubClient) GetBranchProtection(org, repo, branch string) (*github.BranchProtection, error) {
	for _, b := range f.Branches {
		if b.Name == branch && b.Protected {
			return &github.BranchProtection{}, nil
		}
	}

	return nil, nil
}

func (f *fakeGithubClient) GetPullRequests(org, repo string) ([]github.PullRequest, error) {
	var prs []github.PullRequest
	for _, pr := range f.PullRequests {
		prs = append(prs, *pr)
	}
	return prs, nil
}

func (f *fakeGithubClient) RemoveBranchProtection(org, repo, branch string) error {
	for _, b := range f.Branches {
		if b.Name == branch && b.Protected {
			return nil
		}
	}
	return fmt.Errorf("Branch %q does either not exist or is not protected", branch)
}

func (f *fakeGithubClient) GetRepo(owner, name string) (github.FullRepo, error) {
	repo, err := f.FakeClient.GetRepo(owner, name)
	if strings.HasSuffix(owner, "-fork") {
		repo.Repo.Fork = true
		repo.Parent.Owner.Login = strings.TrimSuffix(owner, "-fork")
		repo.Parent.Name = name
		repo.Parent.FullName = fmt.Sprintf("%s/%s", repo.Parent.Owner.Login, repo.Parent.Name)
	}
	return repo, err
}

var _ = Describe("BranchCleaner", func() {
	var (
		bc         *branchCleaner
		fakeGithub *fakeGithubClient
	)

	BeforeEach(func() {
		fakeGithub = &fakeGithubClient{FakeClient: fakegithub.NewFakeClient()}
		bc = &branchCleaner{
			githubClient: fakeGithub,
		}
	})

	Describe("#getMatchingBranches", func() {
		JustBeforeEach(func() {
			fakeGithub.Branches = []github.Branch{
				{Name: "master", Protected: true},
				{Name: "release-v1.73"},
				{Name: "release-v1.74"},
				{Name: "release-v1.72"},
				{Name: "release-v1.75", Protected: true},
			}
		})

		It("should get matching branches in reverse order", func() {
			bc.options.branchPattern = "^release-v\\d+\\.\\d+"
			branches, err := bc.getMatchingBranches()
			Expect(err).ToNot(HaveOccurred())
			Expect(branches).To(Equal([]string{"release-v1.75", "release-v1.74", "release-v1.73", "release-v1.72"}))
		})

		It("should not find non matching branches", func() {
			bc.options.branchPattern = "^foobar-v\\d+\\.\\d+"
			branches, err := bc.getMatchingBranches()
			Expect(err).ToNot(HaveOccurred())
			Expect(branches).To(HaveLen(0))
		})

		It("should fail on an invalid regexp", func() {
			bc.options.branchPattern = "foobar-v\\d+++++++\\.\\d+"
			_, err := bc.getMatchingBranches()
			Expect(err).To(MatchError(ContainSubstring("error compiling branch pattern")))
		})
	})

	Describe("#identifyBranchesToDelete", func() {
		var branchesInput []string

		JustBeforeEach(func() {
			var err error
			branchesInput = []string{"release-v1.75", "release-v1.74", "release-v1.73", "release-v1.72"}
			bc.repo, err = fakeGithub.GetRepo("foo", "bar")
			Expect(err).ToNot(HaveOccurred())
		})

		It("should identify the correct number of branches", func() {
			bc.options.keepBranches = 2
			branches, err := bc.identifyBranchesToDelete(branchesInput)
			Expect(err).ToNot(HaveOccurred())
			Expect(branches).To(Equal([]string{"release-v1.73", "release-v1.72"}))
		})

		It("should skip a branch if there is an open PR for it", func() {
			bc.options.keepBranches = 2
			fakeGithub.PullRequests[1] = &github.PullRequest{
				Number: 1,
				State:  "open",
				Base:   github.PullRequestBranch{Ref: "release-v1.73"},
			}

			branches, err := bc.identifyBranchesToDelete(branchesInput)
			Expect(err).ToNot(HaveOccurred())
			Expect(branches).To(Equal([]string{"release-v1.72"}))
		})

		It("should not skip a branch with open PRs if it should ignore open PRs", func() {
			bc.options.keepBranches = 2
			bc.options.ignoreOpenPRs = true
			fakeGithub.PullRequests[1] = &github.PullRequest{
				Number: 1,
				State:  "open",
				Base:   github.PullRequestBranch{Ref: "release-v1.73"},
			}

			branches, err := bc.identifyBranchesToDelete(branchesInput)
			Expect(err).ToNot(HaveOccurred())
			Expect(branches).To(Equal([]string{"release-v1.73", "release-v1.72"}))
		})

		It("should skip a branch if the repo is a fork and there is an open PR to an upstream repository", func() {
			var err error
			bc.repo, err = fakeGithub.GetRepo("foo-fork", "bar")
			Expect(err).ToNot(HaveOccurred())
			bc.options.keepBranches = 2

			fakeGithub.PullRequests[1] = &github.PullRequest{
				Number: 1,
				State:  "open",
				Head: github.PullRequestBranch{
					Ref: "release-v1.73",
					Repo: github.Repo{
						Name:  "bar",
						Owner: github.User{Login: "foo-fork"},
					},
				},
			}

			branches, err := bc.identifyBranchesToDelete(branchesInput)
			Expect(err).ToNot(HaveOccurred())
			Expect(branches).To(Equal([]string{"release-v1.72"}))
		})
	})

	Describe("#run", func() {
		JustBeforeEach(func() {
			bc.fullRepo = "foo/bar"
			bc.options.branchPattern = "^release-v\\d+\\.\\d+"
			fakeGithub.Branches = []github.Branch{
				{Name: "master", Protected: true},
				{Name: "release-v1.73", Protected: true},
				{Name: "release-v1.74"},
				{Name: "release-v1.72"},
				{Name: "release-v1.75", Protected: true},
			}
		})

		It("should delete the correct refs", func() {
			bc.options.keepBranches = 2
			fakeGithub.PullRequests[1] = &github.PullRequest{
				Number: 1,
				State:  "open",
				Base:   github.PullRequestBranch{Ref: "release-v1.73"},
			}

			Expect(bc.run()).To(Succeed())
			Expect(fakeGithub.RefsDeleted).To(Equal([]struct{ Org, Repo, Ref string }{{"foo", "bar", "heads/release-v1.72"}}))
		})

		It("should delete the correct refs with release branch mode enabled", func() {
			bc.options.keepBranches = 2
			bc.options.releaseBranchMode = true
			fakeGithub.PullRequests[1] = &github.PullRequest{
				Number: 1,
				State:  "open",
				Base:   github.PullRequestBranch{Ref: "release-v1.73"},
			}

			Expect(bc.run()).To(Succeed())
			Expect(fakeGithub.RefsDeleted).To(Equal([]struct{ Org, Repo, Ref string }{{"foo", "bar", "heads/release-v1.72"}}))
		})

		It("should delete the correct refs with release branch mode enabled - one with branch protection, one without", func() {
			bc.options.keepBranches = 2
			bc.options.releaseBranchMode = true

			Expect(bc.run()).To(Succeed())
			Expect(fakeGithub.RefsDeleted).To(Equal([]struct{ Org, Repo, Ref string }{{"foo", "bar", "heads/release-v1.73"}, {"foo", "bar", "heads/release-v1.72"}}))
		})
	})
})
