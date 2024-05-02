// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/sirupsen/logrus"
	gitv2 "sigs.k8s.io/prow/pkg/git/v2"
)

const (
	compareVersionBranch = "release-handler-compare-versions"
)

type releaseHandler struct {
	options          options
	gitClientFactory gitv2.ClientFactory

	repoClient gitv2.RepoClient
}

type versionChange string

const (
	prepareNewMajorMinorVersion versionChange = "prepareNewMajorMinorVersion"
	prepareNewPatchVersion      versionChange = "prepareNewPatchVersion"
	newRelease                  versionChange = "newRelease"
	nextDevCycle                versionChange = "nextDevCycle"
	undefinedVersionChange      versionChange = "undefinedVersionChange"
)

type versionClassification struct {
	currentVersion  *semver.Version
	previousVersion *semver.Version

	versionChange versionChange
}

func (r *releaseHandler) run() error {
	var err error
	if r.repoClient == nil {
		logrus.Infof("Initialize git repo client for github.com/%s/%s", r.options.org, r.options.repo)
		r.repoClient, err = r.gitClientFactory.ClientFor(r.options.org, r.options.repo)
		if err != nil {
			return err
		}
	}

	logrus.Info("Ensure base ref is checked out")
	if err := r.checkOutBase(); err != nil {
		return nil
	}

	versionFileChanged, err := r.isVersionFileUpdated()
	if err != nil {
		return err
	}
	if !versionFileChanged {
		logrus.Infof("No changes in version file %q with last commit - exiting", r.options.versionFilePath)
		return nil
	}

	versionClassification, err := r.compareVersions()
	if err != nil {
		return err
	}

	return r.handleVersionChange(versionClassification)
}

func (r *releaseHandler) isVersionFileUpdated() (bool, error) {
	changes, err := r.repoClient.Diff("HEAD~1", "HEAD")
	if err != nil {
		return false, err
	}
	for _, change := range changes {
		if change == r.options.versionFilePath {
			logrus.Infof("Version file %q updated with last commit", r.options.versionFilePath)
			return true, nil
		}
	}
	return false, nil
}

func (r *releaseHandler) checkOutBase() error {
	logrus.Infof("Checking out base SHA %q", r.options.baseSHA)
	return r.repoClient.Checkout(r.options.baseSHA)
}

func (r *releaseHandler) compareVersions() (*versionClassification, error) {
	logrus.Infof("Reading version file %q from current commit", r.options.versionFilePath)
	currentVersion, err := getSemverFromFile(path.Join(r.repoClient.Directory(), r.options.versionFilePath))
	if err != nil {
		return nil, err
	}

	logrus.Infof("Creating temporary new branch %q to compare version files", compareVersionBranch)
	if err := r.repoClient.CheckoutNewBranch(compareVersionBranch); err != nil {
		return nil, err
	}

	logrus.Info("Getting version file from previous commit")
	if err := r.repoClient.ResetHard("HEAD~1"); err != nil {
		return nil, err
	}

	logrus.Infof("Reading version file %q from previous commit", r.options.versionFilePath)
	previousVersion, err := getSemverFromFile(path.Join(r.repoClient.Directory(), r.options.versionFilePath))
	if err != nil {
		return nil, err
	}

	if err := r.checkOutBase(); err != nil {
		return nil, err
	}

	logrus.Infof("Current version is %q - version of previous commit is %q", currentVersion, previousVersion)

	return classifyVersionChange(currentVersion, previousVersion)
}

func (r *releaseHandler) createReleaseBranch(version *semver.Version) error {
	releaseBranchName := fmt.Sprintf("%sv%d.%d", r.options.releaseBranchPrefix, version.Major(), version.Minor())
	if exists := r.repoClient.BranchExists(releaseBranchName); exists {
		logrus.Infof("Release branch %q is already existing - skip creation", releaseBranchName)
		return nil
	}
	logrus.Infof("Creating new release branch %q", releaseBranchName)
	if err := r.repoClient.CheckoutNewBranch(releaseBranchName); err != nil {
		return err
	}
	logrus.Infof("Starting release branch at HEAD~1 which is the latest commit for %q", version)
	if err := r.repoClient.ResetHard("HEAD~1"); err != nil {
		return err
	}
	if !r.options.dryRun {
		logrus.Info("Pushing release branch to upstream")
		if err := r.repoClient.PushToCentral(releaseBranchName, false); err != nil {
			return err
		}
		logrus.Infof("Release branch %q pushed", releaseBranchName)
	} else {
		logrus.Infof("Release branch %q created successfully", releaseBranchName)
		logrus.Info("Dry-run mode - not pushing any changes")
	}
	return nil
}

func (r *releaseHandler) handleVersionChange(versionClassification *versionClassification) error {

	switch c := versionClassification.versionChange; c {
	case prepareNewMajorMinorVersion:
		logrus.Infof("%q detected", c)
		if err := r.createReleaseBranch(versionClassification.previousVersion); err != nil {
			return err
		}
	default:
		logrus.Infof("%q detected - nothing to do", c)
	}

	return nil
}

func classifyVersionChange(currentVersion, previousVersion *semver.Version) (*versionClassification, error) {
	versionClassification := &versionClassification{currentVersion: currentVersion, previousVersion: previousVersion}

	switch {
	case currentVersion.Compare(previousVersion) != 1:
		return nil, fmt.Errorf("version does not increase between %q and %q", previousVersion, currentVersion)
	case currentVersion.Prerelease() == "":
		versionClassification.versionChange = newRelease
	case previousVersion.Prerelease() == "":
		versionClassification.versionChange = nextDevCycle
	case currentVersion.Major() > previousVersion.Major():
		versionClassification.versionChange = prepareNewMajorMinorVersion
	case currentVersion.Minor() > previousVersion.Minor():
		versionClassification.versionChange = prepareNewMajorMinorVersion
	case currentVersion.Patch() > previousVersion.Patch():
		versionClassification.versionChange = prepareNewPatchVersion
	default:
		return nil, fmt.Errorf("updating version %q to version %q is not supported", previousVersion, currentVersion)
	}

	return versionClassification, nil
}

func getSemverFromFile(path string) (*semver.Version, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, strings.Trim(scanner.Text(), " "))
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("version file %q is empty", path)
	}
	// semver version should be found in the first line
	version, err := semver.NewVersion(lines[0])
	if err != nil {
		return nil, fmt.Errorf("invalid semver version in first line of file %q: %w", path, err)
	}
	return version, nil
}
