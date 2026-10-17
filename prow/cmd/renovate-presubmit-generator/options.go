// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/prow/pkg/flagutil"
)

const (
	defaultOrg        = "gardener"
	defaultRepo       = "ci-infra"
	defaultOutput     = "config/jobs/common/renovate-presubmits.yaml"
	defaultCluster    = "gardener-prow-build"
	defaultBaseBranch = "master"
	defaultPRBranch   = "ci-renovate-presubmits"
	defaultPRTitle    = "Update renovate-config check presubmits"
)

type options struct {
	org           string
	repo          string
	output        string
	cluster       string
	renovateImage string
	skipRepos     flagutil.Strings

	// confirm switches from dry-run (write locally) to apply mode (open a pull request).
	confirm bool
	signoff bool

	baseBranch     string
	prBranch       string
	prTitle        string
	labelsOverride []string

	gitUser  string
	gitEmail string

	ghOpts flagutil.GitHubOptions

	logLevel string
}

func parseOptions() options {
	var o options
	if err := o.parseArgs(flag.CommandLine, os.Args[1:]); err != nil {
		logrus.Fatalf("Invalid flags: %v", err)
	}
	return o
}

func (o *options) parseArgs(flags *flag.FlagSet, args []string) error {
	var labelsOverride string

	flags.StringVar(&o.org, "org", defaultOrg, "GitHub org whose repositories should get a renovate-config presubmit.")
	flags.StringVar(&o.repo, "repo", defaultRepo, "Repository (within --org) that holds the generated config and against which the pull request is opened in apply mode.")
	flags.StringVar(&o.output, "output", defaultOutput, "Path (relative to the repository root) of the generated presubmit config.")
	flags.StringVar(&o.cluster, "cluster", defaultCluster, "Prow build cluster the generated jobs run on.")
	flags.StringVar(&o.renovateImage, "renovate-image", "", "Renovate image used to run renovate-config-validator. Only used when the output file does not already pin an image; required if the file does not exist yet.")
	o.skipRepos = flagutil.NewStrings()
	flags.Var(&o.skipRepos, "skip-repos", "Repositories (in format <org>/<repo>) that should not get a presubmit, even if they are eligible. May be passed multiple times.")

	flags.BoolVar(&o.confirm, "confirm", false, "Clone the repository and open a pull request with the regenerated file. Without this flag the file is only written locally (dry-run).")
	flags.BoolVar(&o.signoff, "signoff", false, "Add a DCO sign-off line to the commit. GitHub App commits usually should not be signed off.")

	flags.StringVar(&o.baseBranch, "base-branch", defaultBaseBranch, "Base branch of the pull request opened in apply mode.")
	flags.StringVar(&o.prBranch, "pr-branch", defaultPRBranch, "Name of the branch pushed to the repository and used as the pull request head.")
	flags.StringVar(&o.prTitle, "pr-title", defaultPRTitle, "Title of the opened pull request and the commit.")
	flags.StringVar(&labelsOverride, "labels-override", "", "Comma-separated list of labels to add to the pull request.")

	flags.StringVar(&o.gitUser, "git-user", "", "Name used as the git commit author. When empty, it is resolved from the authenticated GitHub user.")
	flags.StringVar(&o.gitEmail, "git-email", "", "Email used as the git commit author. When empty, it is resolved from the authenticated GitHub user.")

	o.ghOpts.AddFlags(flags)

	flags.StringVar(&o.logLevel, "log-level", logrus.InfoLevel.String(), fmt.Sprintf("Logging level, one of %v", logrus.AllLevels))

	if err := flags.Parse(args); err != nil {
		return err
	}

	level, err := logrus.ParseLevel(o.logLevel)
	if err != nil {
		return fmt.Errorf("--log-level invalid: %w", err)
	}
	logrus.SetLevel(level)

	if labelsOverride != "" {
		o.labelsOverride = strings.Split(labelsOverride, ",")
	}

	if err := o.ghOpts.Validate(!o.confirm); err != nil {
		return err
	}
	// Both listing repos and opening a PR need authentication.
	if o.ghOpts.TokenPath == "" && o.ghOpts.AppID == "" {
		return errors.New("please provide either --github-token-path or --github-app-id together with --github-app-private-key-path")
	}

	return nil
}
