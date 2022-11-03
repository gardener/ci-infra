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
func (g *FakeGithubClient) CreatePullRequest(org, repo, title, body, head, base string, canModify bool) (int, error) {
	return 1, nil
}

// EnsureFork is a fake for EnsureFork
func (g *FakeGithubClient) EnsureFork(forkingUser, org, repo string) (string, error) {
	return repo, nil
}

// GetPullRequests is a fake for GetPullRequests
func (g *FakeGithubClient) GetPullRequests(org, repo string) ([]github.PullRequest, error) {
	return []github.PullRequest{}, nil
}

// GetBranches is a fake for GetBranches
func (g *FakeGithubClient) GetBranches(org, repo string, onlyProtected bool) ([]github.Branch, error) {
	if onlyProtected {
		return g.ProtectedBranches, nil
	}
	return g.UnprotectedBranches, nil
}

// AddLabels is a fake for AddLabels
func (g *FakeGithubClient) AddLabels(org, repo string, number int, label ...string) error {
	return nil
}
