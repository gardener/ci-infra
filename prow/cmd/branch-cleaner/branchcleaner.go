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
	"regexp"
	"sort"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
)

type githubClient interface {
	GetRepo(owner, name string) (github.FullRepo, error)
	GetBranches(org, repo string, onlyProtected bool) ([]github.Branch, error)
	GetPullRequests(org, repo string) ([]github.PullRequest, error)
	DeleteRef(org, repo, ref string) error
}

type branchCleaner struct {
	githubClient githubClient
	options      options

	repo github.FullRepo
}

func (b *branchCleaner) run() error {
	logrus.Infof("Start cleaning up branches from %q", b.options.fullRepo)

	org, repo, err := config.SplitRepoName(b.options.fullRepo)
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

	branchesToDelete, err := b.identifyBranchesToDelete(matchingBranches)
	if err != nil {
		return fmt.Errorf("error identifying branches to delete: %w", err)
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

func (b *branchCleaner) identifyBranchesToDelete(branches []string) ([]string, error) {
	var branchesToDelete []string
	if len(branches) <= b.options.keepBranches {
		return branchesToDelete, nil
	}
	for _, branch := range branches[b.options.keepBranches:] {
		delete, err := b.checkBranchToDelete(branch)
		if err != nil {
			return nil, err
		}
		if delete {
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
