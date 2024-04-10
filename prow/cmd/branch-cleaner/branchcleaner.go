// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/Masterminds/semver"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/prow/prow/config"
	"sigs.k8s.io/prow/prow/github"
)

type githubClient interface {
	GetRepo(owner, name string) (github.FullRepo, error)
	GetBranches(org, repo string, onlyProtected bool) ([]github.Branch, error)
	GetBranchProtection(org, repo, branch string) (*github.BranchProtection, error)
	RemoveBranchProtection(org, repo, branch string) error
	GetPullRequests(org, repo string) ([]github.PullRequest, error)
	GetRef(org, repo, ref string) (string, error)
	DeleteRef(org, repo, ref string) error
}

type branchCleaner struct {
	githubClient githubClient
	fullRepo     string
	options      options

	repo github.FullRepo
}

var (
	semverMajorMinor *regexp.Regexp
)

func init() {
	semverMajorMinor = regexp.MustCompile(`v(0|[1-9]\d*)\.(0|[1-9]\d*)`)
}

func (b *branchCleaner) run() error {
	logrus.Infof("Start cleaning up branches from %q", b.fullRepo)

	org, repo, err := config.SplitRepoName(b.fullRepo)
	if err != nil {
		return err
	}

	logrus.Info("Get Repository from Github")
	b.repo, err = b.githubClient.GetRepo(org, repo)
	if err != nil {
		return err
	}

	matchingBranches, err := b.getMatchingBranches()
	if err != nil {
		return fmt.Errorf("error identifying matching branches: %w", err)
	}

	if b.options.releaseBranchMode {
		logrus.Info("Release branch mode active. Searching release tags for the branches")
		matchingBranches, err = b.identifyBranchesWithReleaseTags(matchingBranches)
		if err != nil {
			return fmt.Errorf("error identifying branches with release tags: %w", err)
		}
	}

	branchesToDelete, err := b.identifyBranchesToDelete(matchingBranches)
	if err != nil {
		return fmt.Errorf("error identifying branches to delete: %w", err)
	}

	if b.options.releaseBranchMode {
		logrus.Infof("Release branch mode active. Deleting branch protection rules for %v if existing", branchesToDelete)
		if err := b.removeBranchProtection(branchesToDelete); err != nil {
			return fmt.Errorf("error removing branch protection rules: %w", err)
		}
	}

	return b.deleteBranches(branchesToDelete)
}

func (b *branchCleaner) getMatchingBranches() ([]string, error) {
	branchPattern, err := regexp.Compile(b.options.branchPattern)
	if err != nil {
		return nil, fmt.Errorf("error compiling branch pattern: %w", err)
	}

	logrus.Info("Get unprotected branches")
	unprotectedBranches, err := b.githubClient.GetBranches(b.repo.Owner.Login, b.repo.Name, false)
	if err != nil {
		return nil, err
	}
	logrus.Info("Get protected branches")
	protectedBranches, err := b.githubClient.GetBranches(b.repo.Owner.Login, b.repo.Name, true)
	if err != nil {
		return nil, err
	}
	branches := append(unprotectedBranches, protectedBranches...)

	var matchingBranches []string
	for _, branch := range branches {
		if branchPattern.MatchString(branch.Name) {
			matchingBranches = append(matchingBranches, branch.Name)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(matchingBranches)))
	logrus.Infof("Found these matching branches sorted in reverse order: %v", matchingBranches)

	return matchingBranches, nil
}

func (b *branchCleaner) identifyBranchesWithReleaseTags(branches []string) ([]string, error) {
	var branchesWithReleaseTags []string
	for _, branch := range branches {
		branchSemverStr := semverMajorMinor.FindString(branch)
		branchSemver, err := semver.NewVersion(branchSemverStr)
		if err != nil {
			return nil, fmt.Errorf("error parsing semver of branch %q: %w", branch, err)
		}
		ref, err := b.githubClient.GetRef(b.repo.Owner.Login, b.repo.Name, fmt.Sprintf("tags/v%s", branchSemver.String()))
		if github.IsNotFound(err) {
			logrus.Infof("There is no release tag for branch %q - skipping this branch", branch)
			continue
		} else if err != nil {
			return nil, err
		}
		logrus.Infof("Found release tag %q for branch %q", ref, branch)
		branchesWithReleaseTags = append(branchesWithReleaseTags, branch)
	}
	logrus.Infof("These release branches have at least one release tag: %v", branchesWithReleaseTags)

	return branchesWithReleaseTags, nil
}

func (b *branchCleaner) identifyBranchesToDelete(branches []string) ([]string, error) {
	var branchesToDelete []string
	if len(branches) <= b.options.keepBranches {
		return branchesToDelete, nil
	}
	for _, branch := range branches[b.options.keepBranches:] {
		deleteBranch, err := b.checkBranchToDelete(branch)
		if err != nil {
			return nil, err
		}
		if deleteBranch {
			branchesToDelete = append(branchesToDelete, branch)
		}
	}
	logrus.Infof("Identified these branches for deletion: %v", branchesToDelete)

	return branchesToDelete, nil
}

func (b *branchCleaner) checkBranchToDelete(branch string) (bool, error) {
	if b.options.ignoreOpenPRs {
		return true, nil
	}

	var openPRs bool

	logrus.Infof("Checking if there are open PRs for branch %q", branch)
	prs, err := b.githubClient.GetPullRequests(b.repo.Owner.Login, b.repo.Name)
	if err != nil {
		return false, err
	}
	for _, pr := range prs {
		if pr.Base.Ref == branch && pr.State == github.PullRequestStateOpen {
			logrus.Infof("Found an open PR for branch %q - skip deletion", branch)
			openPRs = true
			break
		}
	}

	if b.repo.Fork && !openPRs {
		logrus.Infof("Repository %q is a fork of %q. Checking upstream repository for open PRs", b.repo.FullName, b.repo.Parent.FullName)
		prs, err := b.githubClient.GetPullRequests(b.repo.Parent.Owner.Login, b.repo.Parent.Name)
		if err != nil {
			return false, err
		}
		for _, pr := range prs {
			if pr.Head.Repo.Owner.Login == b.repo.Owner.Login &&
				pr.Head.Repo.Name == b.repo.Name &&
				pr.Head.Ref == branch {
				logrus.Infof("Found an open PR in upstream repository from branch %q - skip deletion", branch)
				openPRs = true
				break
			}
		}
	}

	return !openPRs, nil
}

func (b *branchCleaner) deleteBranches(branches []string) error {
	for _, branch := range branches {
		logrus.Infof("Deleting branch %q", branch)
		if err := b.githubClient.DeleteRef(b.repo.Owner.Login, b.repo.Name, fmt.Sprintf("heads/%s", branch)); err != nil {
			return err
		}
	}
	logrus.Infof("Successfully deleted these branches: %v", branches)

	return nil
}

func (b *branchCleaner) removeBranchProtection(branches []string) error {
	for _, branch := range branches {
		branchProtection, err := b.githubClient.GetBranchProtection(b.repo.Owner.Login, b.repo.Name, branch)
		if err != nil {
			return err
		}
		if branchProtection == nil {
			logrus.Infof("There is no branch protection rule for %q branch", branch)
			continue
		}
		if err := b.githubClient.RemoveBranchProtection(b.repo.Owner.Login, b.repo.Name, branch); err != nil {
			return err
		}
		logrus.Infof("Successfully deleted branch protection rule of %q branch", branch)
	}

	return nil
}
