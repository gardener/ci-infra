// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"path/filepath"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/prow/pkg/logrusutil"

	ghi "github.com/gardener/ci-infra/prow/pkg/githubinteractor"
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

	// gitFactory is the authenticated git client factory used for clone
	// and push.
	gitFactory, err := o.ghOpts.GitClientFactory("", nil, !o.applyChanges, false)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to initialize git client!")
	}

	// BotUser/Email require an authenticated client, we only fetch them in apply
	// mode. Dry runs never checkout/commit so leaving them unset is fine.
	gh := &ghi.GithubServer{
		Ghc: ghClient,
		Gcf: gitFactory,
		Gc:  &ghi.CommitClient{},
	}
	if o.applyChanges {
		botUser, err := ghClient.BotUser()
		if err != nil {
			logrus.WithError(err).Fatal("Failed to get bot user for git commit identity!")
		}
		email, err := ghClient.Email()
		if err != nil {
			logrus.WithError(err).Fatal("Failed to get bot email for git commit identity!")
		}
		gh.BotUser = botUser
		gh.Email = email
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
			rep, err := checkoutRepo(ghClient, gh, orgName, repoName, prCfg)
			if err != nil {
				logrus.WithError(err).Errorf("Failed to initialize Git Client for %s/%s", orgName, repoName)
				continue
			}

			// write changes to file
			repoDir := rep.RepoClient.Directory()
			aliasesPath := filepath.Join(repoDir, "OWNERS_ALIASES")

			if err := writeChanges(aliasesPath, changes); err != nil {
				logrus.WithError(err).Errorf("Failed to write changes to OWNERS_ALIASES of repo %s/%s", orgName, repoName)
				continue
			}

			// commit and push changes
			if err := commitAndPush(rep, gh, orgName, repoName, prCfg); err != nil {
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
			if err := rep.RepoClient.Clean(); err != nil {
				logrus.WithError(err).Errorf("Failed to delete/cleanup locally stored repo: %s/%s", orgName, repoName)
			}
		}
	}
}
