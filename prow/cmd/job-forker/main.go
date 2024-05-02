// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/prow/pkg/flagutil"
	"sigs.k8s.io/prow/pkg/logrusutil"
	"sigs.k8s.io/prow/pkg/pod-utils/downwardapi"

	ghi "github.com/gardener/ci-infra/prow/pkg/githubinteractor"
)

const (
	// ForkAnnotation is the annotation a job has to have in order for it to be forked
	ForkAnnotation = "fork-per-release"
	// ForkedAnnotation is the annotation that will be added when a config has been forked
	ForkedAnnotation = "created-by-job-forker"
	// TargetBranchPrefix is the prefix under which the job-forker will create a branch in which the changes will be commited
	TargetBranchPrefix = "job-forker"
)

type options struct {
	jobDirectory string
	// upstreamRepo includes the prow jobs which should be forked
	upstreamRepo string
	// upstreamBranch is the branch of upstreamRepo
	upstreamBranch string
	// outputDirectory
	outputDirectory      string
	releaseBranchPattern string
	labelsOverride       []string
	dryRun               bool
	recursive            bool
	github               flagutil.GitHubOptions
	gitEmail             string

	logLevel string
}

func (o *options) validate() error {
	if err := o.github.Validate(o.dryRun); err != nil {
		return err
	}
	if o.upstreamRepo == "" {
		return fmt.Errorf("please provide a non empty --upstream-repository")
	}
	if o.upstreamBranch == "" {
		return fmt.Errorf("please provide a non empty --upstream-branch")
	}
	if o.jobDirectory == "" {
		return fmt.Errorf("please provide a non empty --job-directory")
	}
	if o.releaseBranchPattern == "" {
		return fmt.Errorf("please provide a non empty --release-branch-pattern")
	}
	if o.outputDirectory == "" {
		return fmt.Errorf("please provide a non empty --output-directory")
	}
	return nil
}

func gatherOptions() options {
	var labelsOverride string
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&o.upstreamRepo, "upstream-repository", "", "upstream-repository includes the prow jobs which should be forked")
	fs.StringVar(&o.upstreamBranch, "upstream-branch", "master", "upstream-branch is the branch of upstream-repository")
	fs.StringVar(&o.jobDirectory, "job-directory", "", "Directory with the prow jobs which should be forked")
	fs.StringVar(&o.outputDirectory, "output-directory", "releases", "Output directory for forked prow jobs (relative path to the original prow job)")
	fs.BoolVar(&o.recursive, "recursive", false, "When set to true, all sub-folders of job-directory will be searched for prow-jobs")
	fs.StringVar(&o.releaseBranchPattern, "release-branch-pattern", "release-v\\d+\\.\\d+", "Pattern to identify release branches for which prow jobs should be forked")
	fs.StringVar(&labelsOverride, "labels-override", "", "Labels which should be added to the PR")
	fs.StringVar(&o.gitEmail, "git-email", "", "E-Mail the bot should use to commit changes")
	fs.BoolVar(&o.dryRun, "dry-run", true, "DryRun")
	fs.StringVar(&o.logLevel, "log-level", "info", fmt.Sprintf("Log level is one of %v.", logrus.AllLevels))
	o.github.AddFlags(fs)

	err := fs.Parse(os.Args[1:])
	if err != nil {
		logrus.WithError(err).Fatal("Unable to parse command line flags")
	}
	if labelsOverride != "" {
		o.labelsOverride = append(o.labelsOverride, strings.Split(labelsOverride, ",")...)
	} else {
		o.labelsOverride = nil
	}
	return o
}

func main() {
	logrusutil.Init(&logrusutil.DefaultFieldsFormatter{
		PrintLineNumber:  true,
		WrappedFormatter: logrus.StandardLogger().Formatter,
	})

	o := gatherOptions()
	if err := o.validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid input")
	}

	jobSpec, err := downwardapi.ResolveSpecFromEnv()
	if err != nil {
		logrus.WithError(err).Fatal("Unable to resolve prow job spec")
	}

	githubClient, err := o.github.GitHubClient(o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting GitHubClient")
	}
	gitClientFactory, err := o.github.GitClientFactory("", nil, o.dryRun, false)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting Git client")
	}
	botUser, err := githubClient.BotUser()
	if err != nil {
		logrus.WithError(err).Fatal("Error getting bot name")
	}

	githubServer := ghi.GithubServer{
		Ghc:     githubClient,
		Gcf:     gitClientFactory,
		Gc:      &ghi.CommitClient{},
		BotUser: botUser,
		Email:   o.gitEmail,
	}

	upstreamRepo, err := ghi.NewRepository(o.upstreamRepo, &githubServer)
	if err != nil {
		logrus.WithError(err).Fatal("Couldn't create repository object")
	}
	err = upstreamRepo.CloneRepo()
	if err != nil {
		logrus.WithError(err).Fatal("Couldn't clone repository")
	}

	err = upstreamRepo.RepoClient.Checkout(o.upstreamBranch)
	if err != nil {
		logrus.WithError(err).Fatalf("Couldn't checkout branch %s", o.upstreamBranch)
	}

	changes, err := generateForkedConfigurations(upstreamRepo, o)
	if err != nil {
		logrus.WithError(err).Fatalf("Error during forking of configurations")
	}

	if changes {
		err := upstreamRepo.PushChanges(
			o.upstreamRepo,
			o.upstreamBranch,
			fmt.Sprintf("%s-%s", TargetBranchPrefix, jobSpec.Job),
			"Forked prow jobs for release branches",
			fmt.Sprintf("Forked prow jobs for release branches created by prow job `%s`", jobSpec.Job),
			o.labelsOverride)
		if err != nil {
			logrus.WithError(err).Fatal("Error during pushing of changes")
		}
	} else {
		logrus.Info("No changes to commit")
	}
}
