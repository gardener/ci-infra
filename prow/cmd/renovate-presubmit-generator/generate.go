// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/prow/pkg/github"
)

// runIfChanged pattern matches all possible renovate config file paths,
// see https://docs.renovatebot.com/configuration-options/#configuration-options.
const runIfChanged = `^((\.github/|\.gitlab/)?renovate\.json5?|\.renovaterc(\.json5?)?|package.json)$`

var (
	// renovateImageRegex extracts the renovate image and tag from an existing output file.
	renovateImageRegex = regexp.MustCompile(`image:\s*(ghcr\.io/renovatebot/renovate:\S+)`)
	//go:embed templates/renovate-presubmits.yaml.tmpl
	presubmitsTemplateText string
	// presubmitsTemplate renders one presubmit per repo: the first as a YAML anchor, the rest
	// merging it (<<: *renovate-presubmit) and only overriding the name.
	presubmitsTemplate = template.Must(template.New("presubmits").Parse(presubmitsTemplateText))
)

// eligibleRepos returns the sorted names of org repos that should get a presubmit: public, not
// archived, not a fork, not a dot-prefixed meta-repo (e.g. ".github"), and not in skip ("org/repo").
func eligibleRepos(repos []github.Repo, org string, skip sets.Set[string]) []string {
	var names []string
	for _, repo := range repos {
		if repo.Private || repo.Archived || repo.Fork {
			continue
		}
		if strings.HasPrefix(repo.Name, ".") {
			continue
		}
		if skip.Has(fmt.Sprintf("%s/%s", org, repo.Name)) {
			continue
		}
		names = append(names, repo.Name)
	}
	sort.Strings(names)
	return names
}

// resolveImage returns the renovate image for the jobs: the one pinned in the output file at path
// (preserving Renovate's bumps) if present, otherwise flagImage. It errors if neither is set.
func resolveImage(path, flagImage string) (string, error) {
	switch content, err := os.ReadFile(path); {
	case err == nil:
		if match := renovateImageRegex.FindSubmatch(content); match != nil {
			return string(match[1]), nil
		}
	case !errors.Is(err, os.ErrNotExist):
		return "", fmt.Errorf("reading %s: %w", path, err)
	}

	if flagImage != "" {
		return flagImage, nil
	}
	return "", fmt.Errorf("no renovate image available: pass --renovate-image or make sure %s already pins one", path)
}

// renderPresubmits renders the presubmit config for the given repositories.
func renderPresubmits(org, cluster, image string, repos []string) ([]byte, error) {
	var buf bytes.Buffer
	data := struct {
		Org          string
		Cluster      string
		Image        string
		RunIfChanged string
		Repos        []string
	}{
		Org:          org,
		Cluster:      cluster,
		Image:        image,
		RunIfChanged: runIfChanged,
		Repos:        repos,
	}
	if err := presubmitsTemplate.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("rendering presubmits: %w", err)
	}
	return buf.Bytes(), nil
}
