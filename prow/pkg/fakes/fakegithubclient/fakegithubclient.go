package fakegithubclient

import (
	"k8s.io/test-infra/prow/github"
)

// FakeGithubClient fakes the prow GitHubClient
type FakeGithubClient struct {
	FakedProtectedBranches   []github.Branch
	FakedUnprotectedBranches []github.Branch
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
		return g.FakedProtectedBranches, nil
	}
	return g.FakedUnprotectedBranches, nil
}

// AddLabels is a fake for AddLabels
func (g *FakeGithubClient) AddLabels(org, repo string, number int, label ...string) error {
	return nil
}
