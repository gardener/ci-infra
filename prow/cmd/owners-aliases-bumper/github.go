// SPDX-FileCopyrightText: Copyright Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"slices"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/prow/pkg/git/v2"
	"sigs.k8s.io/prow/pkg/github"
)

const (
	defaultCommitTitle = "Update OWNERS_ALIASES from Peribolos config"
	defaultCommitBody  = `
Automated update by owners-aliases-bumper. Alias membership was synced with the GitHub team definitions in the Peribolos config.
`
	defaultPRBranch = "owners-aliases-bumper"
	defaultPRTitle  = defaultCommitTitle
	defaultPRBody   = `
<!-- Please ensure that you do not include company internal information. -->

**How to categorize this PR?**
<!--
Please select the kind of this pull request, e.g.:
/kind enhancement

Tide will not merge your PR, if it is missing a kind/* label.
"/kind" identifiers:    api-change|bug|cleanup|discussion|enhancement|epic|impediment|poc|post-mortem|question|regression|task|technical-debt|test
-->
/kind cleanup

**What this PR does / why we need it**:
Automated update by owners-aliases-bumper. The alias membership in this
repo's OWNERS_ALIASES file has been synced to match the GitHub team
definitions in the Peribolos config.

Aliases that share a name with a Peribolos team are populated from that
team's members and maintainers. Only aliases already present in
OWNERS_ALIASES are updated — no aliases are added or removed. This keeps
OWNERS_ALIASES in sync with the source-of-truth team definitions so
approvers/reviewers don't drift from actual team membership.

**Special notes for your reviewer**:
This PR was generated automatically.
`
)

// prConfig holds the resolved commit/PR text used when applying changes. It is
// built from options (see buildPRConfig) so the defaults above can be overridden
// via flags.
type prConfig struct {
	branch      string
	commitTitle string
	commitBody  string
	prTitle     string
	prBody      string
}

func checkoutRepo(ghClient github.Client, gitClient git.ClientFactory, orgName, repoName string, cfg prConfig) (git.RepoClient, error) {
	log := logrus.WithField("repo", orgName+"/"+repoName)

	log.Debug("Cloning repo (git client factory)")
	r, err := gitClient.ClientFor(orgName, repoName)
	if err != nil {
		return nil, err
	}

	log.Debug("Fetching repo info to determine default branch")
	repoInfo, err := ghClient.GetRepo(orgName, repoName)
	if err != nil {
		return nil, err
	}

	log.Debugf("Checking out default branch %q", repoInfo.DefaultBranch)
	if err := r.Checkout(repoInfo.DefaultBranch); err != nil {
		return nil, fmt.Errorf("unable to checkout branch %s of repo %s/%s: %w", repoInfo.DefaultBranch, orgName, repoName, err)
	}

	log.Debugf("Creating new branch %q", cfg.branch)
	if err := r.CheckoutNewBranch(cfg.branch); err != nil {
		return nil, fmt.Errorf("unable to checkout new branch %s of repo %s/%s: %w", cfg.branch, orgName, repoName, err)
	}

	log.Debugf("Repo checked out at %s on branch %q", r.Directory(), cfg.branch)
	return r, nil
}

// commitAndPush commits the working-tree changes using commitClient (which carries
// the commit-author identity) and then pushes via pushFactory, which is separately
// authenticated. Both operate on the same checkout: pushFactory wraps commitClient's
// directory with ClientFromDir rather than re-cloning, so the commit is present when
// we push. This split exists because GitClientFactory does not wire a GitUser, so a
// single client would nil-panic in Commit.
func commitAndPush(commitClient git.RepoClient, pushFactory git.ClientFactory, orgName, repoName string, cfg prConfig) error {
	log := logrus.WithField("repo", orgName+"/"+repoName)

	log.Infof("Committing changes with title %q", cfg.commitTitle)
	if err := commitClient.Commit(cfg.commitTitle, cfg.commitBody); err != nil {
		return fmt.Errorf("failed to commit to repo %s/%s: %w", orgName, repoName, err)
	}

	// Wrap the same working tree with the authenticated factory to push.
	pushClient, err := pushFactory.ClientFromDir(orgName, repoName, commitClient.Directory())
	if err != nil {
		return fmt.Errorf("failed to create push client for repo %s/%s: %w", orgName, repoName, err)
	}

	log.Infof("Pushing branch %q to central remote", cfg.branch)
	if err := pushClient.PushToCentral(cfg.branch, true); err != nil {
		return fmt.Errorf("failed to push to repo %s/%s: %w", orgName, repoName, err)
	}

	log.Debug("Commit and push complete")
	return nil
}

// prClient is the subset of github.Client used by findOrCreatePR.
type prClient interface {
	GetRepo(owner, name string) (github.FullRepo, error)
	GetPullRequests(org, repo string) ([]github.PullRequest, error)
	CreatePullRequest(org, repo, title, body, head, base string, canModify bool) (int, error)
	ClosePullRequest(org, repo string, number int) error
}

func findOrCreatePR(ghClient prClient, orgName, repoName string, cfg prConfig) (int, error) {
	repoInfo, err := ghClient.GetRepo(orgName, repoName)
	if err != nil {
		return 0, fmt.Errorf("failed to get Repo %s/%s: %w", orgName, repoName, err)
	}

	prs, err := ghClient.GetPullRequests(orgName, repoName)
	if err != nil {
		return 0, fmt.Errorf("failed to get PRs for repo %s/%s: %w", orgName, repoName, err)
	}

	// filter all prs that are not open and all prs that do not match our branch
	prs = slices.DeleteFunc(prs, func(pr github.PullRequest) bool {
		return pr.Head.Ref != cfg.branch || pr.State != "open"
	})
	prsDefaultBranch := slices.DeleteFunc(slices.Clone(prs), func(pr github.PullRequest) bool {
		return pr.Base.Ref != repoInfo.DefaultBranch
	})

	var prNum int
	if len(prsDefaultBranch) == 0 { // no open PR
		prNum, err = ghClient.CreatePullRequest(orgName, repoName, cfg.prTitle, cfg.prBody, cfg.branch, repoInfo.DefaultBranch, false)
		if err != nil {
			return 0, fmt.Errorf("failed to create PR for repo %s/%s: %w", orgName, repoName, err)
		}
	} else { // one or more open PRs
		prNum = prsDefaultBranch[0].Number
		// close all other PRs with same base branch
		for _, pr := range prsDefaultBranch[1:] {
			if err := ghClient.ClosePullRequest(orgName, repoName, pr.Number); err != nil {
				logrus.WithError(err).Warnf("failed closing PR %d from repo %s/%s", pr.Number, orgName, repoName)
			}
		}
	}

	// close prs with other base refs
	prs = slices.DeleteFunc(prs, func(pr github.PullRequest) bool {
		return pr.Base.Ref == repoInfo.DefaultBranch
	})

	for _, pr := range prs {
		if err := ghClient.ClosePullRequest(orgName, repoName, pr.Number); err != nil {
			logrus.WithError(err).Warnf("failed closing PR %d from repo %s/%s", pr.Number, orgName, repoName)
		}
	}

	return prNum, nil
}
