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
	"regexp"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/logrusutil"
)

type options struct {
	fullRepo      string
	branchPattern string
	keepBranches  int
	ignoreOpenPRs bool

	dryRun bool
	github flagutil.GitHubOptions

	logLevel string
}

func (o *options) validate() error {
	if err := o.github.Validate(o.dryRun); err != nil {
		return err
	}
	if o.fullRepo == "" {
		return fmt.Errorf("please provide a non empty --repository")
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
	fs.StringVar(&o.fullRepo, "repository", "", "Repository which should be cleaned up.")
	fs.StringVar(&o.branchPattern, "branch-pattern", "^release-v\\d+\\.\\d+", "Regexp pattern to identify branches which should be cleaned up")
	fs.IntVar(&o.keepBranches, "keep-branches", 3, "Defines the number of branches which should be kept (sorted in an alphabetical order)")
	fs.BoolVar(&o.ignoreOpenPRs, "ignore-open-prs", false, "Defines if the branch should be deleted even when there are open PRs")

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
		logrus.WithError(err).Fatal("Error getting Git client")
	}

	branchCleaner := branchCleaner{
		githubClient: githubClient,
		options:      o,
	}

	if err := branchCleaner.run(); err != nil {
		logrus.WithError(err).Fatal("Error when running branch-cleaner")
	}
}
