// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/prow/pkg/github"
	"sigs.k8s.io/prow/pkg/repoowners"

	"github.com/gardener/gardener-landscape-kit/pkg/apis/config/v1alpha1"
	"github.com/gardener/gardener-landscape-kit/pkg/utils/meta"

	"go.yaml.in/yaml/v4"
)

type change struct {
	add    sets.Set[string]
	remove sets.Set[string]
}

// fileGetter fetches a file from a repository. It is the subset of
// github.Client used by calculateAliasChanges.
type fileGetter interface {
	GetFile(org, repo, filepath, commit string) ([]byte, error)
}

func calculateAliasChanges(ghClient fileGetter, localAliases *fullOrgAliases, orgName, repoName string) map[string]change {
	log := logrus.WithField("repo", orgName+"/"+repoName)

	// get the aliases currently in the repo:
	log.Debug("Fetching OWNERS_ALIASES from repo default branch")
	raw, err := ghClient.GetFile(orgName, repoName, "OWNERS_ALIASES", "") // empty commit ID for latest commit on default branch

	if err != nil {
		if _, ok := errors.AsType[*github.FileNotFound](err); ok {
			logrus.Infof("Repo %s/%s has no OWNERS_ALIASES file skipping...", orgName, repoName)
			return nil // repo does not have a OWNERS_ALIASES file nothing to do
		}
		logrus.WithError(err).Errorf("Failed to get OWNERS_ALIASES from %s/%s", orgName, repoName)
		return nil // skip gracefully if failed
	}
	log.Debugf("Fetched OWNERS_ALIASES (%d bytes)", len(raw))

	aliases, err := repoowners.ParseAliasesConfig(raw)
	if err != nil {
		logrus.WithError(err).Errorf("Failed to parse OWNERS_ALIASES from %s/%s", orgName, repoName)
		return nil // skip
	}
	log.Debugf("Parsed %d alias(es) from repo OWNERS_ALIASES", len(aliases))

	changes := make(map[string]change)

	// compare all existing aliases with ours
	for alias, members := range aliases {
		localMembers := localAliases.getConfig(orgName).getMembers(alias)

		if localMembers == nil {
			log.Debugf("Alias %q not present in local config, skipping", alias)
			continue // alias does not exist in our local config, skip
		}

		toBeAdded := localMembers.Difference(members)
		toBeDeleted := members.Difference(localMembers)
		if toBeAdded.Len() != 0 || toBeDeleted.Len() != 0 {
			changes[alias] = change{
				add:    toBeAdded,
				remove: toBeDeleted,
			}
			log.Debugf("Alias %q differs: repo=%v local=%v -> add=%v remove=%v",
				alias, sets.List(members), sets.List(localMembers), sets.List(toBeAdded), sets.List(toBeDeleted))
		} else {
			log.Debugf("Alias %q already in sync (%d member(s))", alias, members.Len())
		}
	}

	log.Debugf("Comparison done: %d alias(es) tracked", len(changes))
	return changes
}

// ownersAliasesFile models the OWNERS_ALIASES file. Only the string array
// syntax is accepted, not the {alias: {user1, user2}} notation (which is
// technically allowed by repoowners).
type ownersAliasesFile struct {
	Aliases map[string][]string `yaml:"aliases"`
}

func deepCopyYaml(original []byte) ([]byte, error) {
	var originalParsed ownersAliasesFile
	if err := yaml.Unmarshal(original, &originalParsed); err != nil {
		return nil, fmt.Errorf("failed unmarshaling for deepCopy: %w", err)
	}
	return yaml.Marshal(originalParsed)
}

func writeChanges(aliasesPath string, aliasChanges map[string]change) (err error) {
	originalRaw, err := os.ReadFile(aliasesPath)
	if err != nil {
		return fmt.Errorf("unable to read file %s: %w", aliasesPath, err)
	}

	var aliasModified ownersAliasesFile
	if err := yaml.Unmarshal(originalRaw, &aliasModified); err != nil {
		return fmt.Errorf("failed parsing file at %s: %w", aliasesPath, err)
	}

	originalMarshaled, err := deepCopyYaml(originalRaw)
	if err != nil {
		return fmt.Errorf("failed to parse original parsed yaml back to yaml... file: %s: %w", aliasesPath, err)
	}

	// make modifications
	for alias, change := range aliasChanges {
		_, exists := aliasModified.Aliases[alias]
		if !exists {
			logrus.Warnf("expected %s to contain alias %s, however it was not found", aliasesPath, alias)
			continue
		}

		for user := range change.remove {
			aliasModified.Aliases[alias] = deleteValue(aliasModified.Aliases[alias], user)
		}

		for user := range change.add {
			aliasModified.Aliases[alias] = append(aliasModified.Aliases[alias], user)
		}
		slices.Sort(aliasModified.Aliases[alias])
	}

	modifiedMarshaled, err := yaml.Marshal(aliasModified)
	if err != nil {
		return fmt.Errorf("failed to parse modified aliases to yaml file: %s: %w", aliasesPath, err)
	}

	output, err := meta.ThreeWayMergeManifest(originalMarshaled, modifiedMarshaled, originalRaw, v1alpha1.MergeModeHint)
	if err != nil {
		return fmt.Errorf("failed to merge new aliases with yaml file: %s: %w", aliasesPath, err)
	}

	fo, err := os.Create(aliasesPath)
	if err != nil {
		return fmt.Errorf("failed to open file %s for writing: %w", aliasesPath, err)
	}
	defer func() {
		if errC := fo.Close(); errC != nil && err == nil {
			err = fmt.Errorf("failed closing file %s: %w", aliasesPath, errC)
		}
	}()

	if _, err = fo.Write(output); err != nil {
		return fmt.Errorf("failed to write to file %s: %w", aliasesPath, err)
	}

	return nil
}

func deleteValue(arr []string, value string) []string {
	return slices.DeleteFunc(arr, func(e string) bool {
		return e == value
	})
}
