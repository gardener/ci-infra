// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/prow/pkg/flagutil"
)

// TODO add option to change pr branch and maybe even if fork or not
type options struct {
	peribolosConfig string
	applyChanges    bool
	skipRepos       flagutil.Strings

	// PR/commit text overrides. Empty values fall back to the default* constants
	// in github.go. The *BodyFile variants read the body from a file and are
	// mutually exclusive with their inline counterpart (see buildPRConfig).
	prBranch       string
	prTitle        string
	commitTitle    string
	commitBody     string
	commitBodyFile string
	prBody         string
	prBodyFile     string

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
	flags.StringVar(&o.peribolosConfig, "peribolos-conf", "", "The path to the Peribolos config, will be used to populate aliases which have the same name as a GitHub team.")
	o.skipRepos = flagutil.NewStrings()
	flags.Var(&o.skipRepos, "skip-repos", "By default all repos defined in the Peribolos config will be managed, list all repos that should be skipped here. (in format: <org>/<repo>)")
	flags.BoolVar(&o.applyChanges, "confirm", false, "Set this flag in Order to apply the changes, without this flag only information on what would be changed is printed.")

	// PR/commit text overrides. Defaults live in github.go (default* constants).
	flags.StringVar(&o.prBranch, "pr-branch", defaultPRBranch, "Name of the branch pushed to the repo and used as the PR head.")
	flags.StringVar(&o.prTitle, "pr-title", defaultPRTitle, "Title of the opened pull request.")
	flags.StringVar(&o.commitTitle, "commit-title", defaultCommitTitle, "Title (subject line) of the commit.")
	flags.StringVar(&o.commitBody, "commit-body", "", "Body of the commit. Mutually exclusive with --commit-body-file. Defaults to a built-in message if neither is set.")
	flags.StringVar(&o.commitBodyFile, "commit-body-file", "", "Path to a file whose contents are used as the commit body. Mutually exclusive with --commit-body.")
	flags.StringVar(&o.prBody, "pr-body", "", "Body of the pull request. Mutually exclusive with --pr-body-file. Defaults to a built-in message if neither is set.")
	flags.StringVar(&o.prBodyFile, "pr-body-file", "", "Path to a file whose contents are used as the PR body. Mutually exclusive with --pr-body.")

	o.ghOpts.AddFlags(flags)

	// logrus
	flags.StringVar(&o.logLevel, "log-level", logrus.InfoLevel.String(), fmt.Sprintf("Logging level, one of %v", logrus.AllLevels))

	if err := flags.Parse(args); err != nil {
		return err
	}

	// logrus
	level, err := logrus.ParseLevel(o.logLevel)
	if err != nil {
		return fmt.Errorf("--log-level invalid: %w", err)
	}
	logrus.SetLevel(level)
	logrus.SetReportCaller(level >= logrus.DebugLevel)

	// check if args provide valid configuration
	if o.peribolosConfig == "" {
		return errors.New("--peribolos-conf needs to be set")
	}

	return nil
}

// buildPRConfig resolves the commit/PR text from the parsed options into a
// prConfig. For each body, an inline flag and its *-file counterpart are
// mutually exclusive; when neither is set the built-in default is used. Body
// files are read here, so this may touch the filesystem and return an error.
func (o *options) buildPRConfig() (prConfig, error) {
	commitBody, err := resolveBody("commit-body", o.commitBody, o.commitBodyFile, defaultCommitBody)
	if err != nil {
		return prConfig{}, err
	}
	prBody, err := resolveBody("pr-body", o.prBody, o.prBodyFile, defaultPRBody)
	if err != nil {
		return prConfig{}, err
	}

	return prConfig{
		branch:      o.prBranch,
		commitTitle: o.commitTitle,
		commitBody:  commitBody,
		prTitle:     o.prTitle,
		prBody:      prBody,
	}, nil
}

// resolveBody picks the body text from the inline value, a file, or a default,
// erroring if both inline and file are set. name is the flag base name (e.g.
// "pr-body") used only for error messages.
func resolveBody(name, inline, file, def string) (string, error) {
	if inline != "" && file != "" {
		return "", fmt.Errorf("--%s and --%s-file are mutually exclusive", name, name)
	}
	if file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("unable to read --%s-file %s: %w", name, file, err)
		}
		return string(raw), nil
	}
	if inline != "" {
		return inline, nil
	}
	return def, nil
}
