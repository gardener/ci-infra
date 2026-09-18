// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	prowjobv1 "sigs.k8s.io/prow/pkg/apis/prowjobs/v1"
	"sigs.k8s.io/prow/pkg/flagutil"
	"sigs.k8s.io/prow/pkg/pod-utils/downwardapi"
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

	clientFactory, err := o.github.GitClientFactory("", nil, o.dryRun, false)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating git client factory")
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
