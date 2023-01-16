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
	"bufio"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"

	ghi "github.com/gardener/ci-infra/prow/pkg/githubinteractor"
)

const (
	startOfSectionPattern = "# <-- /start: %s -->"
	endOfSectionPattern   = "# <-- /end: %s -->"
	commentedLines        = "# "
	milestoneProperty     = "milestone:"
)

func runMilestoneActivator(milestoneClient github.MilestoneClient, upstreamRepo *ghi.Repository, o options) error {

	milestoneOrg, milestoneRepo, err := config.SplitRepoName(o.milestoneRepo)
	if err != nil {
		return err
	}

	milestone, err := getOpenMilestone(milestoneClient, milestoneOrg, milestoneRepo, o.milestonePattern)
	if err != nil {
		return err
	}

	changes, err := updateFiles(upstreamRepo.RepoClient.Directory(), o.filesWithSections.Strings(), o.sectionIdentifier, milestone)
	if err != nil {
		return err
	}

	if !changes {
		logrus.Info("No changes to commit")
		return nil
	}

	logrus.Info("Committing changes")
	return upstreamRepo.PushChanges(
		o.upstreamRepo,
		o.upstreamBranch,
		fmt.Sprintf("%s-%s", TargetBranchPrefix, o.prowJobName),
		"Update milestone configuration",
		fmt.Sprintf("Milestone section update created by prow job `%s`", o.prowJobName),
		o.labelsOverride)
}

func updateFiles(repoDirectory string, filePaths []string, sectionIdentifier, milestone string) (bool, error) {
	var changes bool
	for _, filePath := range filePaths {
		path := path.Join(repoDirectory, filePath)

		logrus.Infof("Working on file %q", path)

		sectionsFound, err := findSectionsInFile(path, sectionIdentifier)
		if err != nil {
			return false, err
		}

		if !sectionsFound {
			logrus.Infof("No section with identifier %q found in file %q", sectionIdentifier, path)
			continue
		}
		logrus.Infof("Found sections with identifier %q in file %q", sectionIdentifier, path)

		fileChanged, err := handleMilestoneSection(path, sectionIdentifier, milestone)
		if err != nil {
			return false, err
		}
		changes = changes || fileChanged
		if !fileChanged {
			logrus.Infof("No changes in file %q", path)
		}
	}

	return changes, nil
}

func getOpenMilestone(milestoneClient github.MilestoneClient, org, repo, milestonePattern string) (string, error) {
	milestones, err := milestoneClient.ListMilestones(org, repo)
	if err != nil {
		return "", err
	}

	milestoneRegexp, err := regexp.Compile(milestonePattern)
	if err != nil {
		return "", err
	}

	var milestoneTitle string
	for _, milestone := range milestones {
		if milestone.State != "open" {
			logrus.Debugf("Skip milestone %s with state %s", milestone.Title, milestone.State)
			continue
		}
		if milestoneRegexp.MatchString(milestone.Title) {
			if milestoneTitle != "" {
				return "", fmt.Errorf("found more than one open milestone of pattern %q", milestonePattern)
			}
			milestoneTitle = milestone.Title
		}

	}
	if milestoneTitle == "" {
		logrus.Info("No open milestone found")
	} else {
		logrus.Infof("Found open milestone %q", milestoneTitle)
	}

	return milestoneTitle, nil
}

func findSectionsInFile(path, sectionIdentifier string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	var (
		startOfSection int
		endOfSection   int
		lineNumber     int
	)

	startOfSectionIdentifier := fmt.Sprintf(startOfSectionPattern, sectionIdentifier)
	endOfSectionIdentifier := fmt.Sprintf(endOfSectionPattern, sectionIdentifier)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		switch strings.Trim(line, " ") {
		case startOfSectionIdentifier:
			logrus.Debugf("Found start of section in line %v of file %q", line, path)
			startOfSection++
			if startOfSection > endOfSection+1 {
				return false, fmt.Errorf("file %s: found second consecutive start of section identifier %q in line %v", path, sectionIdentifier, lineNumber)
			}

		case endOfSectionIdentifier:
			logrus.Debugf("Found end of section in line %v of file %q", line, path)
			endOfSection++
			if endOfSection > startOfSection {
				return false, fmt.Errorf("file %s: found an end of section identifier %q in line %v without start of section", path, sectionIdentifier, lineNumber)
			}
		}

	}

	if startOfSection != endOfSection {
		return false, fmt.Errorf("file %s: found a started section of %q which is not ended", path, sectionIdentifier)
	}

	return startOfSection > 0, nil
}

func handleMilestoneSection(path, sectionIdentifier, milestone string) (bool, error) {
	var (
		changes  bool
		newLines []string
	)

	logrus.Infof("Handling milestone sections in file %q", path)

	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	var (
		changeLines bool
		lineNumber  int
		offset      int
	)

	startOfSectionIdentifier := fmt.Sprintf(startOfSectionPattern, sectionIdentifier)
	endOfSectionIdentifier := fmt.Sprintf(endOfSectionPattern, sectionIdentifier)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		switch strings.Trim(line, " ") {
		case startOfSectionIdentifier:
			offset = strings.Index(line, startOfSectionIdentifier)
			changeLines = true
			newLines = append(newLines, line)
			logrus.Debugf("Start handling section beginning in line %v with an offset of %v spaces", line, offset)
		case endOfSectionIdentifier:
			if strings.Index(line, endOfSectionIdentifier) != offset {
				return false, fmt.Errorf("inconsistent number of leading spaces at start and end of section identifiers")
			}
			changeLines = false
			newLines = append(newLines, line)
			logrus.Debugf("Finished handling section at line %v", line)
		default:
			if !changeLines {
				newLines = append(newLines, line)
				continue
			}

			var (
				newLine     string
				lineChanged bool
			)
			if milestone == "" {
				newLine, lineChanged = handleDeactivateLine(line, offset)
			} else {
				newLine, lineChanged = handleActivateLine(line, milestone, offset)
			}
			changes = changes || lineChanged
			newLines = append(newLines, newLine)
		}
	}

	if changes {
		logrus.Infof("Changes detected - writing to file %q", path)
		output := strings.Join(newLines, "\n") + "\n"

		err := os.WriteFile(path, []byte(output), 0644)
		if err != nil {
			return false, err
		}
	}

	return changes, nil
}

func handleActivateLine(line, milestone string, offset int) (string, bool) {
	var (
		changes bool
		newLine string
	)
	if !strings.HasPrefix(strings.TrimLeft(line, " "), commentedLines) {
		newLine = line
	} else {
		newLine = line[0:offset] + strings.TrimPrefix(line[offset:], commentedLines)
		changes = true
	}
	if i := strings.LastIndex(newLine, milestoneProperty); i != -1 {
		tmpLine := newLine
		newLine = fmt.Sprintf("%s %s", newLine[0:i+len(milestoneProperty)], milestone)
		if newLine != tmpLine {
			changes = true
		}
	}
	return newLine, changes
}

func handleDeactivateLine(line string, offset int) (string, bool) {
	if strings.HasPrefix(strings.TrimLeft(line, " "), commentedLines) {
		return line, false
	}
	newLine := line[0:offset] + commentedLines + line[offset:]
	return newLine, true
}
