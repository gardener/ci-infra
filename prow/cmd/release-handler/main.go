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

	"github.com/sirupsen/logrus"
	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/flagutil"
	gitv2 "k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
)

type options struct {
	releaseBranchPrefix string
	versionFilePath     string

	dryRun bool
	github flagutil.GitHubOptions

	org     string
	repo    string
	baseSHA string

	logLevel string
}

func (o *options) validate() error {
	if err := o.github.Validate(o.dryRun); err != nil {
		return err
	}
	if o.releaseBranchPrefix == "" {
		return fmt.Errorf("please provide a non empty --release-branch-prefix")
	}
	if o.versionFilePath == "" {
		return fmt.Errorf("please provide a non empty --version-file-path")
	}
	if o.github.TokenPath == "" {
		return fmt.Errorf("please provide a non empty --github-token-path")
	}
	return nil
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&o.releaseBranchPrefix, "release-branch-prefix", "release-", "prefix for release branches, release-handler maintains")
	fs.StringVar(&o.versionFilePath, "version-file-path", "VERSION", "path to the file which defines the semver version")
	fs.BoolVar(&o.dryRun, "dry-run", true, "DryRun")
	fs.StringVar(&o.logLevel, "log-level", "info", fmt.Sprintf("Log level is one of %v.", logrus.AllLevels))
	o.github.AddFlags(fs)

	err := fs.Parse(os.Args[1:])
	if err != nil {
		logrus.WithError(err).Fatal("Unable to parse command line flags")
	}

	jobSpec, err := downwardapi.ResolveSpecFromEnv()
	if err != nil {
		logrus.WithError(err).Fatal("Unable to resolve prow job spec")
	}

	switch jobSpec.Type {
	case prowjobv1.PeriodicJob:
		logrus.Fatal("Release-handler cannot be used in periodic prow jobs")
	case prowjobv1.BatchJob:
		logrus.Fatal("Release-handler cannot be used in batch prow jobs")
	case prowjobv1.PostsubmitJob:
		o.org = jobSpec.Refs.Org
		o.repo = jobSpec.Refs.Repo
		o.baseSHA = jobSpec.Refs.BaseSHA
	case prowjobv1.PresubmitJob:
		logrus.Fatal("Release-handler cannot be used in presubmit prow jobs")
	}

	return o
}

func constructClientFactoryOpts(githubClient github.Client, o options) (*gitv2.ClientFactoryOpts, error) {
	if err := secret.Add(o.github.TokenPath); err != nil {
		return nil, err
	}
	userGenerator := func() (string, error) {
		user, err := githubClient.BotUser()
		if err != nil {
			return "", err
		}
		return user.Login, nil
	}
	gitUser := func() (string, string, error) {
		user, err := githubClient.BotUser()
		if err != nil {
			return "", "", err
		}
		name := user.Name
		email := user.Email
		return name, email, nil
	}

	clientFactoryOpts := gitv2.ClientFactoryOpts{
		Censor:   secret.Censor,
		Username: userGenerator,
		Token:    secret.GetTokenGenerator(o.github.TokenPath),
		GitUser:  gitUser,
	}

	return &clientFactoryOpts, nil
}

func main() {
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

	clientFactoryOpts, err := constructClientFactoryOpts(githubClient, o)
	if err != nil {
		logrus.WithError(err).Fatal("Error constructing client factory opt")
	}

	clientFactory, err := gitv2.NewClientFactory(clientFactoryOpts.Apply)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating client factory")
	}

	releaseHandler := releaseHandler{
		options:          o,
		gitClientFactory: clientFactory,
	}

	err = releaseHandler.run()
	if err != nil {
		logrus.WithError(err).Fatal("Error when running release-handler")
	}
}
