// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"path/filepath"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/prow/pkg/config/secret"
	gitv2 "sigs.k8s.io/prow/pkg/git/v2"
	"sigs.k8s.io/prow/pkg/logrusutil"
)

func main() {
	logrusutil.ComponentInit()
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	o := parseOptions()

	prCfg, err := o.buildPRConfig()
	if err != nil {
		logrus.WithError(err).Fatal("Invalid PR/commit configuration!")
	}

	if o.applyChanges {
		logrus.Info("Running in APPLY mode (--confirm set): changes will be pushed and PRs opened")
	} else {
		logrus.Info("Running in DRY-RUN mode (no --confirm): no forks, commits or PRs will be created")
	}

	cfg := newFullOrgAliases()

	logrus.Infof("Reading Peribolos config from %s", o.peribolosConfig)
	orgConfig := parseOrgConfig(o.peribolosConfig)
	logrus.Infof("Parsed Peribolos config: %d org(s) defined", len(orgConfig.Orgs))

	ghClient, err := o.ghOpts.GitHubClient(!o.applyChanges)

	if err != nil {
		logrus.WithError(err).Fatal("Failed to initialize GitHub client!")
	}

	// pushFactory is fully authenticated (PAT or GitHub App) and is used only to
	// push the branch to the remote.
	pushFactory, err := o.ghOpts.GitClientFactory("", nil, !o.applyChanges, false)

	if err != nil {
		logrus.WithError(err).Fatal("Failed to initialize git client!")
	}

	// commitFactory carries the commit author identity (GitUser). GitClientFactory
	// never wires GitUser, so repoClient.Commit would nil-panic without this. Commit
	// is a purely local operation, so this factory needs no token and stays auth-agnostic.
	// The authenticated pushFactory handles the networked push separately.
	//
	// Only built in apply mode: BotUser()/Email() require an authenticated client, and
	// a dry run never commits, so building it unconditionally would break anonymous runs.
	var commitFactory gitv2.ClientFactory
	if o.applyChanges {
		botUser, err := ghClient.BotUser()
		if err != nil {
			logrus.WithError(err).Fatal("Failed to get bot user for git commit identity!")
		}
		email, err := ghClient.Email()
		if err != nil {
			logrus.WithError(err).Fatal("Failed to get bot email for git commit identity!")
		}

		commitFactory, err = gitv2.NewClientFactory(
			gitv2.WithGitUser(func() (name, gitEmail string, err error) {
				return botUser.Name, email, nil
			}),
			gitv2.WithCensor(secret.Censor),
		)
		if err != nil {
			logrus.WithError(err).Fatal("Failed to initialize commit git client!")
		}
	}

	// build our available aliases from the teams information we have
	for orgName, orgConfig := range orgConfig.Orgs {
		aliases := cfg.getConfig(orgName)
		addMembersFromTeams(aliases, orgConfig.Teams, "")
		logrus.Infof("Built local aliases for org %q from %d team(s): %d alias(es) available", orgName, len(orgConfig.Teams), len(aliases))
		for alias := range aliases {
			logrus.Debugf("  [%s] alias %q: %v", orgName, alias, sets.List(aliases.getMembers(alias)))
		}
	}

	// remove repos that should be skipped
	removeExcludedRepos(&orgConfig, o.skipRepos.Strings())

	// manage every repo defined in peribolos conf
	for orgName, orgConfig := range orgConfig.Orgs {
		logrus.Infof("Processing org %q with %d repo(s)", orgName, len(orgConfig.Repos))
		for repoName := range orgConfig.Repos {
			log := logrus.WithField("repo", orgName+"/"+repoName)
			log.Debug("Calculating alias changes")
			changes := calculateAliasChanges(ghClient, cfg, orgName, repoName)
			if len(changes) <= 0 {
				log.Info("No applicable changes, skipping")
				continue // skip change if not applicable
			}
			log.Infof("Found changes for %d alias(es)", len(changes))
			for alias, c := range changes {
				log.Infof("  alias %q: add=%v remove=%v", alias, sets.List(c.add), sets.List(c.remove))
			}

			if !o.applyChanges {
				log.Info("Dry-run: skipping fork/commit/PR")
				continue
			}

			// download repo
			repoClient, err := checkoutRepo(ghClient, commitFactory, orgName, repoName, prCfg)
			if err != nil {
				logrus.WithError(err).Errorf("Failed to initialize Git Client for %s/%s", orgName, repoName)
				continue
			}

			// write changes to file
			repoDir := repoClient.Directory()
			aliasesPath := filepath.Join(repoDir, "OWNERS_ALIASES")

			if err := writeChanges(aliasesPath, changes); err != nil {
				logrus.WithError(err).Errorf("Failed to write changes to OWNERS_ALIASES of repo %s/%s", orgName, repoName)
				continue
			}

			// commit and push changes
			if err := commitAndPush(repoClient, pushFactory, orgName, repoName, prCfg); err != nil {
				logrus.WithError(err).Errorf("Commit and push failed repo: %s/%s", orgName, repoName)
				continue
			}

			// open PR
			id, err := findOrCreatePR(ghClient, orgName, repoName, prCfg)
			if err != nil {
				logrus.WithError(err).Errorf("Opening PR failed on repo %s/%s", orgName, repoName)
				continue
			}

			logrus.Infof("Successfully applied changes to %s/%s and opened PR #%d", orgName, repoName, id)

			// cleanup
			if err := repoClient.Clean(); err != nil {
				logrus.WithError(err).Errorf("Failed to delete/cleanup locally stored repo: %s/%s", orgName, repoName)
			}
		}
	}
}
