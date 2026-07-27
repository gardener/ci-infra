// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/prow/pkg/github"
)

// fakePRClient is a minimal prClient for findOrCreatePR.
type fakePRClient struct {
	repo         github.FullRepo
	getRepoErr   error
	prs          []github.PullRequest
	getPRsErr    error
	createdNum   int
	createErr    error
	createCalled bool
	closed       []int
}

func (f *fakePRClient) GetRepo(_, _ string) (github.FullRepo, error) {
	return f.repo, f.getRepoErr
}

func (f *fakePRClient) GetPullRequests(_, _ string) ([]github.PullRequest, error) {
	return f.prs, f.getPRsErr
}

func (f *fakePRClient) CreatePullRequest(_, _, _, _, _, _ string, _ bool) (int, error) {
	f.createCalled = true
	return f.createdNum, f.createErr
}

func (f *fakePRClient) ClosePullRequest(_, _ string, number int) error {
	f.closed = append(f.closed, number)
	return nil
}

// openPR builds an open PR on the bumper branch with the given number.
func openPR(number int) github.PullRequest {
	pr := github.PullRequest{Number: number, State: "open"}
	pr.Head.Ref = defaultPRBranch
	return pr
}

// testPRConfig is the prConfig used across findOrCreatePR tests.
var testPRConfig = prConfig{
	branch:      defaultPRBranch,
	commitTitle: defaultCommitTitle,
	commitBody:  defaultCommitBody,
	prTitle:     defaultPRTitle,
	prBody:      defaultPRBody,
}

var _ = Describe("Github", func() {
	Describe("#findOrCreatePR", func() {
		It("creates a PR when none exists", func() {
			f := &fakePRClient{createdNum: 42}
			num, err := findOrCreatePR(f, "gardener", "ci-infra", testPRConfig)
			Expect(err).ToNot(HaveOccurred())
			Expect(f.createCalled).To(BeTrue())
			Expect(num).To(Equal(42))
		})

		It("returns the existing PR number without creating one", func() {
			f := &fakePRClient{prs: []github.PullRequest{openPR(7)}}
			num, err := findOrCreatePR(f, "gardener", "ci-infra", testPRConfig)
			Expect(err).ToNot(HaveOccurred())
			Expect(f.createCalled).To(BeFalse())
			Expect(num).To(Equal(7))
		})

		It("ignores PRs on other branches or that are closed", func() {
			closedOnBranch := openPR(1)
			closedOnBranch.State = "closed"
			otherBranch := github.PullRequest{Number: 2, State: "open"}
			otherBranch.Head.Ref = "some-other-branch"

			f := &fakePRClient{
				prs:        []github.PullRequest{closedOnBranch, otherBranch},
				createdNum: 99,
			}
			num, err := findOrCreatePR(f, "gardener", "ci-infra", testPRConfig)
			Expect(err).ToNot(HaveOccurred())
			Expect(f.createCalled).To(BeTrue(), "no matching open PR, so a new one is created")
			Expect(num).To(Equal(99))
		})

		It("keeps the first PR and closes the rest when several match", func() {
			f := &fakePRClient{prs: []github.PullRequest{openPR(5), openPR(6), openPR(7)}}
			num, err := findOrCreatePR(f, "gardener", "ci-infra", testPRConfig)
			Expect(err).ToNot(HaveOccurred())
			Expect(num).To(Equal(5))
			Expect(f.closed).To(ConsistOf(6, 7))
		})

		It("propagates a GetRepo error", func() {
			f := &fakePRClient{getRepoErr: errors.New("boom")}
			_, err := findOrCreatePR(f, "gardener", "ci-infra", testPRConfig)
			Expect(err).To(MatchError(ContainSubstring("failed to get Repo")))
		})

		It("propagates a GetPullRequests error", func() {
			f := &fakePRClient{getPRsErr: errors.New("boom")}
			_, err := findOrCreatePR(f, "gardener", "ci-infra", testPRConfig)
			Expect(err).To(MatchError(ContainSubstring("failed to get PRs")))
		})

		It("propagates a CreatePullRequest error", func() {
			f := &fakePRClient{createErr: errors.New("boom")}
			_, err := findOrCreatePR(f, "gardener", "ci-infra", testPRConfig)
			Expect(err).To(MatchError(ContainSubstring("failed to create PR")))
		})
	})
})
