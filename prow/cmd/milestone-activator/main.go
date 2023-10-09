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
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"

	ghi "github.com/gardener/ci-infra/prow/pkg/githubinteractor"
)

const (
	// TargetBranchPrefix is the prefix under which the milestone-activator will create a branch in which the changes will be commited
	TargetBranchPrefix = "milestone-activator"
)

type options struct {
	// filesWithSections are the files which should be scanned for milestone sections and updated
	filesWithSections flagutil.Strings
	// upstreamRepo includes the config file which should be changed
	upstreamRepo string
	// upstreamBranch is the branch of upstreamRepo
	upstreamBranch    string
	milestoneRepo     string
	milestonePattern  string
	sectionIdentifier string
	labelsOverride    []string
	dryRun            bool
	github            flagutil.GitHubOptions
	gitEmail          string
	prowJobName       string

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
	if len(o.filesWithSections.Strings()) == 0 {
		return fmt.Errorf("please provide at least one --file-with-sections")
	}
	if o.milestoneRepo == "" {
		return fmt.Errorf("please provide a non empty --milestone-repository")
	}
	if o.milestonePattern == "" {
		return fmt.Errorf("please provide a non empty --milestone-pattern")
	}
	if o.sectionIdentifier == "" {
		return fmt.Errorf("please provide a non empty --section-identifier")
	}
	return nil
}

func gatherOptions() options {
	var labelsOverride string
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&o.upstreamRepo, "upstream-repository", "", "upstream-repository includes the prow jobs which should be forked")
	fs.StringVar(&o.upstreamBranch, "upstream-branch", "master", "upstream-branch is the branch of upstream-repository")
	fs.Var(&o.filesWithSections, "file-with-sections", "file which should be scanned for milestone sections and updated")
	fs.StringVar(&o.milestoneRepo, "milestone-repository", "", "Repository which should be scanned for milestones")
	fs.StringVar(&o.milestonePattern, "milestone-pattern", "v\\d+\\.\\d+", "Pattern to identify milestones which should be scanned")
	fs.StringVar(&o.sectionIdentifier, "section-identifier", "", "Identifier for sections in config files which should be handled")
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
	jobSpec, err := downwardapi.ResolveSpecFromEnv()
	if err != nil {
		logrus.WithError(err).Fatal("Unable to resolve prow job spec")
	}
	o.prowJobName = jobSpec.Job

	return o
}

func main() {
	logrusutil.Init(&logrusutil.DefaultFieldsFormatter{
		PrintLineNumber:  true,
		WrappedFormatter: &logrus.TextFormatter{},
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

	err = runMilestoneActivator(githubClient, upstreamRepo, o)
	if err != nil {
		logrus.WithError(err).Fatal("Error when changing milestone configuration")
	}
}
