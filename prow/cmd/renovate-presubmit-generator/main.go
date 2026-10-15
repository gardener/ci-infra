// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/prow/pkg/github"
	"sigs.k8s.io/prow/pkg/logrusutil"

	ghi "github.com/gardener/ci-infra/prow/pkg/githubinteractor"
)

func main() {
	logrusutil.ComponentInit()
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	o := parseOptions()

	ghClient, err := o.ghOpts.GitHubClient(!o.confirm)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to initialize GitHub client!")
	}

	logrus.Infof("Listing repositories of org %q", o.org)
	repos, err := ghClient.GetRepos(o.org, false)
	if err != nil {
		logrus.WithError(err).Fatalf("Failed to list repositories of org %q", o.org)
	}

	eligible := eligibleRepos(repos, o.org, sets.New[string](o.skipRepos.Strings()...))
	logrus.Infof("Found %d eligible (public, non-archived, non-fork) repositories out of %d", len(eligible), len(repos))

	if !o.confirm {
		if err := writeLocal(o, eligible); err != nil {
			logrus.WithError(err).Fatal("Failed to generate presubmit config")
		}
		return
	}

	if err := openPullRequest(o, ghClient, eligible); err != nil {
		logrus.WithError(err).Fatal("Failed to open pull request with regenerated presubmit config")
	}
}

// writeLocal renders the presubmit config and writes it to the output path in the working tree.
func writeLocal(o options, repos []string) error {
	image, err := resolveImage(o.output, o.renovateImage)
	if err != nil {
		return err
	}
	content, err := renderPresubmits(o.org, o.cluster, image, repos)
	if err != nil {
		return err
	}
	if err := os.WriteFile(o.output, content, 0644); err != nil {
		return err
	}
	logrus.Infof("Wrote %s (renovate image %s)", o.output, image)
	return nil
}

// openPullRequest clones the target repo, regenerates the presubmit config and, if it changed,
// opens or updates a pull request.
func openPullRequest(o options, ghClient github.Client, repos []string) error {
	gitFactory, err := o.ghOpts.GitClientFactory("", nil, !o.confirm, false)
	if err != nil {
		return err
	}

	gh := &ghi.GithubServer{
		Ghc:     ghClient,
		Gcf:     gitFactory,
		Gc:      &ghi.CommitClient{},
		AppAuth: o.ghOpts.AppID != "",
	}
	if o.gitUser != "" {
		gh.BotUser = &github.UserData{Name: o.gitUser}
	} else {
		botUser, err := ghClient.BotUser()
		if err != nil {
			return err
		}
		gh.BotUser = botUser
	}
	if o.gitEmail != "" {
		gh.Email = o.gitEmail
	} else {
		email, err := ghClient.Email()
		if err != nil {
			return err
		}
		gh.Email = email
	}

	repoFullName := o.org + "/" + o.repo
	repository, err := ghi.NewRepository(repoFullName, gh)
	if err != nil {
		return err
	}
	if err := repository.CloneRepo(); err != nil {
		return err
	}
	if err := repository.RepoClient.Checkout(o.baseBranch); err != nil {
		return err
	}

	outputPath := filepath.Join(repository.RepoClient.Directory(), o.output)
	image, err := resolveImage(outputPath, o.renovateImage)
	if err != nil {
		return err
	}
	content, err := renderPresubmits(o.org, o.cluster, image, repos)
	if err != nil {
		return err
	}

	if existing, err := os.ReadFile(outputPath); err == nil && bytes.Equal(existing, content) {
		logrus.Infof("%s is already up to date, nothing to do", o.output)
		return nil
	}

	if err := os.WriteFile(outputPath, content, 0644); err != nil {
		return err
	}
	logrus.Infof("Regenerated %s (renovate image %s), opening pull request", o.output, image)

	return repository.PushChanges(repoFullName, o.baseBranch, o.prBranch, o.prTitle, o.prTitle, o.labelsOverride, o.signoff)
}
