// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"slices"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/prow/pkg/github"

	ghi "github.com/gardener/ci-infra/prow/pkg/githubinteractor"
)

const (
	defaultCommitTitle = "Update OWNERS_ALIASES from Peribolos config"
	defaultPRBranch    = "owners-aliases-bumper"
	defaultPRTitle     = defaultCommitTitle
	defaultPRBody      = `
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
	prTitle     string
	prBody      string
}

func checkoutRepo(ghClient github.Client, gh *ghi.GithubServer, orgName, repoName string, cfg prConfig) (*ghi.Repository, error) {
	log := logrus.WithField("repo", orgName+"/"+repoName)

	log.Debug("Cloning repo (git client factory)")
	rep, err := ghi.NewRepository(fmt.Sprintf("%s/%s", orgName, repoName), gh)
	if err != nil {
		return nil, err
	}
	if err := rep.CloneRepo(); err != nil {
		return nil, err
	}

	log.Debug("Fetching repo info to determine default branch")
	repoInfo, err := ghClient.GetRepo(orgName, repoName)
	if err != nil {
		return nil, err
	}

	log.Debugf("Checking out default branch %q", repoInfo.DefaultBranch)
	if err := rep.RepoClient.Checkout(repoInfo.DefaultBranch); err != nil {
		return nil, fmt.Errorf("unable to checkout branch %s of repo %s/%s: %w", repoInfo.DefaultBranch, orgName, repoName, err)
	}

	log.Debugf("Creating new branch %q", cfg.branch)
	if err := rep.RepoClient.CheckoutNewBranch(cfg.branch); err != nil {
		return nil, fmt.Errorf("unable to checkout new branch %s of repo %s/%s: %w", cfg.branch, orgName, repoName, err)
	}

	log.Debugf("Repo checked out at %s on branch %q", rep.RepoClient.Directory(), cfg.branch)
	return rep, nil
}

// commitAndPush stages the working-tree changes, creates a signed-off commit
// via gh.Gc (which shells out to the git binary, RepoClient.Commit has no
// --signoff), and pushes cfg.branch to the central remote. DCO 
// requires Signed-off
func commitAndPush(rep *ghi.Repository, gh *ghi.GithubServer, orgName, repoName string, cfg prConfig) error {
	log := logrus.WithField("repo", orgName+"/"+repoName)

	name, email := gh.BotUser.Name, gh.GetEmail()
	if err := rep.RepoClient.Config("user.name", name); err != nil {
		return fmt.Errorf("failed to configure git user for %s/%s: %w", orgName, repoName, err)
	}
	if err := rep.RepoClient.Config("user.email", email); err != nil {
		return fmt.Errorf("failed to configure git email for %s/%s: %w", orgName, repoName, err)
	}

	log.Infof("Committing changes with title %q (signed off)", cfg.commitTitle)
	if err := gh.Gc.Commit(rep.RepoClient.Directory(), name, email, cfg.commitTitle, true); err != nil {
		return fmt.Errorf("failed to commit to repo %s/%s: %w", orgName, repoName, err)
	}

	log.Infof("Pushing branch %q to central remote", cfg.branch)
	if err := rep.RepoClient.PushToCentral(cfg.branch, true); err != nil {
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
