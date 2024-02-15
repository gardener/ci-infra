// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package fakegithub

import (
	"k8s.io/test-infra/prow/github"
)

// FakeGithubClient fakes the prow GitHubClient
type FakeGithubClient struct {
	ProtectedBranches   []github.Branch
	UnprotectedBranches []github.Branch
}

// CreatePullRequest is a fake for CreatePullRequest
func (g *FakeGithubClient) CreatePullRequest(_, _, _, _, _, _ string, _ bool) (int, error) {
	return 1, nil
}

// EnsureFork is a fake for EnsureFork
func (g *FakeGithubClient) EnsureFork(_, _, repo string) (string, error) {
	return repo, nil
}

// GetPullRequests is a fake for GetPullRequests
func (g *FakeGithubClient) GetPullRequests(_, _ string) ([]github.PullRequest, error) {
	return []github.PullRequest{}, nil
}

// GetBranches is a fake for GetBranches
func (g *FakeGithubClient) GetBranches(_, _ string, onlyProtected bool) ([]github.Branch, error) {
	if onlyProtected {
		return g.ProtectedBranches, nil
	}
	return g.UnprotectedBranches, nil
}

// AddLabels is a fake for AddLabels
func (g *FakeGithubClient) AddLabels(_, _ string, _ int, _ ...string) error {
	return nil
}
