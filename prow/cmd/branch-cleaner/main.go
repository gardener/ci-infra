// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/prow/pkg/flagutil"
	"sigs.k8s.io/prow/pkg/logrusutil"
)

type options struct {
	fullRepos         flagutil.Strings
	branchPattern     string
	keepBranches      int
	ignoreOpenPRs     bool
	releaseBranchMode bool

	dryRun bool
	github flagutil.GitHubOptions

	logLevel string
}

func (o *options) validate() error {
	if err := o.github.Validate(o.dryRun); err != nil {
		return err
	}
	if len(o.fullRepos.Strings()) == 0 {
		return fmt.Errorf("please provide at least one --repository")
	}
	if o.branchPattern == "" {
		return fmt.Errorf("please provide a non empty --branch-pattern")
	}
	if o.keepBranches < 0 {
		return fmt.Errorf("--keep-branches must not be negative")
	}
	_, err := regexp.Compile(o.branchPattern)
	if err != nil {
		return fmt.Errorf("error compiling branch pattern: %w", err)
	}

	return nil
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.Var(&o.fullRepos, "repository", "Repository which should be cleaned up.")
	fs.StringVar(&o.branchPattern, "branch-pattern", "^release-v\\d+\\.\\d+", "Regexp pattern to identify branches which should be cleaned up")
	fs.IntVar(&o.keepBranches, "keep-branches", 3, "Defines the number of branches which should be kept (sorted in an alphabetical order)")
	fs.BoolVar(&o.ignoreOpenPRs, "ignore-open-prs", false, "Defines if the branch should be deleted even when there are open PRs")
	fs.BoolVar(&o.releaseBranchMode, "release-branch-mode", false, "Checks for semver versions in the branches and considers only branches which have a corresponding release")

	fs.BoolVar(&o.dryRun, "dry-run", true, "DryRun")
	fs.StringVar(&o.logLevel, "log-level", "info", fmt.Sprintf("Log level is one of %v.", logrus.AllLevels))
	o.github.AddFlags(fs)

	err := fs.Parse(os.Args[1:])
	if err != nil {
		logrus.WithError(err).Fatal("Unable to parse command line flags")
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
		logrus.WithError(err).Fatal("Invalid options")
	}

	logLevel, err := logrus.ParseLevel(o.logLevel)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to parse loglevel")
	}
	logrus.SetLevel(logLevel)

	githubClient, err := o.github.GitHubClient(o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting Git client")
	}

	logrus.Infof("Running branch-cleaner for these repos: %s", o.fullRepos.String())
	for _, fullRepo := range o.fullRepos.Strings() {
		branchCleaner := branchCleaner{
			githubClient: githubClient,
			fullRepo:     fullRepo,
			options:      o,
		}

		if err := branchCleaner.run(); err != nil {
			logrus.WithError(err).Fatalf("Error when running branch-cleaner for repo %q", fullRepo)
		}
	}
}
