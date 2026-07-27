// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/prow/pkg/config/org"
	"sigs.k8s.io/yaml"
)

func parseOrgConfig(path string) org.FullConfig {
	raw, err := os.ReadFile(path)
	if err != nil {
		logrus.WithError(err).Fatal("Could not read --peribolos-conf file")
	}

	var cfg org.FullConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		logrus.WithError(err).Fatal("Failed to parse peribolos configuration")
	}
	return cfg
}

func removeExcludedRepos(orgs *org.FullConfig, excludedRepos []string) {
	for _, excludedRepo := range excludedRepos {
		splitRepoString := strings.Split(excludedRepo, "/")

		if len(splitRepoString) != 2 {
			logrus.Fatalf("Excluded repository %s is not formatted correctly, please define it as: <org>/<repo name>", excludedRepo)
		}

		// warn the user if the config does not look right
		if _, exists := orgs.Orgs[splitRepoString[0]]; !exists {
			logrus.Warnf("Attempted to exclude %s however, organization %s is not defined in the Peribolos config, excluding it anyways!", excludedRepo, splitRepoString[0])
		} else if _, exists := orgs.Orgs[splitRepoString[0]].Repos[splitRepoString[1]]; !exists {
			logrus.Warnf("Attempted to exclude %s however, %s is not defined in the Peribolos config, excluding it anyways!", excludedRepo, splitRepoString[1])
		}

		// remove repo from our config
		delete(orgs.Orgs[splitRepoString[0]].Repos, splitRepoString[1])
	}
}
