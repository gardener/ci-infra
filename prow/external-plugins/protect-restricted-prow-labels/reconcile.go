// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/prow/pkg/github"
	"sigs.k8s.io/prow/pkg/labels"
	"sigs.k8s.io/prow/pkg/plugins"
)

// searchIndexLag widens the reconciliation window to account for the GitHub search index lagging
// behind a pull request's actual updated_at.
const searchIndexLag = 5 * time.Minute

// reconcile re-scans open pull requests in the enabled repos and re-enforces the protected
// labels. It is a safety net for missed webhook events, e.g. a transient GitHub error or a
// restart while handling one: an acknowledged webhook is never redelivered.
func (s *Server) reconcile() error {
	config := s.configAgent.Config()
	orgs, repos := config.EnabledReposForExternalPlugin(pluginName)
	if len(orgs) == 0 && len(repos) == 0 {
		s.log.Warnf("No repos have been configured for the %s plugin", pluginName)
		return nil
	}

	// A configuration change can bring pull requests into scope that an earlier pass skipped, so
	// force a full pass by resetting the watermark when the plugin-relevant config changes.
	if fingerprint := configFingerprint(config, orgs, repos); fingerprint != s.lastConfigFingerprint {
		s.lastReconcile = time.Time{}
		s.lastConfigFingerprint = fingerprint
	}

	passStart := time.Now()

	// One search per scope returns every pull request carrying any protected label; the labels to
	// enforce are sorted out per pull request afterwards, so each pull request is processed once.
	candidates := map[string]*candidate{}
	var errs []error
	for _, scope := range collectScopeSearches(config, orgs, repos) {
		issues, err := s.search(scope)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, issue := range issues {
			matched := matchedLabels(issue, scope.labels)
			if len(matched) == 0 {
				continue
			}
			org, repo, ok := parseIssueRepo(issue.HTMLURL)
			if !ok {
				errs = append(errs, fmt.Errorf("could not determine repo from issue URL %q", issue.HTMLURL))
				continue
			}
			key := fmt.Sprintf("%s/%s#%d", org, repo, issue.Number)
			if candidates[key] == nil {
				candidates[key] = &candidate{org: org, repo: repo, number: issue.Number, labels: sets.New[string]()}
			}
			candidates[key].labels.Insert(matched...)
			if issue.UpdatedAt.After(candidates[key].updatedAt) {
				candidates[key].updatedAt = issue.UpdatedAt
			}
		}
	}

	keys := make([]string, 0, len(candidates))
	for key := range candidates {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		c := candidates[key]
		// After the first pass, only process pull requests that changed since then, minus a buffer
		// for the search index lagging behind. The rest were already handled in an earlier pass. On
		// the first pass (zero watermark) everything is handled.
		if !s.lastReconcile.IsZero() && !c.updatedAt.After(s.lastReconcile.Add(-searchIndexLag)) {
			continue
		}
		errs = append(errs, s.enforceCandidate(c))
	}

	if err := utilerrors.NewAggregate(errs); err != nil {
		// Keep the old watermark so a failed pass is fully retried next time.
		return err
	}
	s.lastReconcile = passStart
	return nil
}

// candidate is a pull request and the protected labels found on it during reconciliation.
type candidate struct {
	org       string
	repo      string
	number    int
	labels    sets.Set[string]
	updatedAt time.Time
}

// scopeSearch is a search scope and the protected labels to look for in it; an empty repo means
// the whole org.
type scopeSearch struct {
	org    string
	repo   string
	labels sets.Set[string]
}

// search returns the open pull requests in the scope that carry any of the scope's labels.
func (s *Server) search(scope scopeSearch) ([]github.Issue, error) {
	target := "org:" + scope.org
	if scope.repo != "" {
		target = "repo:" + scope.org + "/" + scope.repo
	}
	// Comma-separated labels are OR-ed by the GitHub search; each is quoted the same way Tide
	// builds its queries.
	quoted := make([]string, 0, scope.labels.Len())
	for _, label := range sets.List(scope.labels) {
		quoted = append(quoted, fmt.Sprintf("%q", label))
	}
	query := fmt.Sprintf("%s is:pr is:open archived:false label:%s", target, strings.Join(quoted, ","))
	issues, err := s.ghc.FindIssuesWithOrg(scope.org, query, "", false)
	if err != nil {
		return nil, fmt.Errorf("failed to search pull requests in %s: %w", scopeString(scope.org, scope.repo), err)
	}
	return issues, nil
}

// enforceCandidate fetches the pull request's events once and enforces every protected label on it.
func (s *Server) enforceCandidate(c *candidate) error {
	log := s.log.WithFields(logrus.Fields{
		github.OrgLogField:  c.org,
		github.RepoLogField: c.repo,
		github.PrLogField:   c.number,
	})

	events, err := s.ghc.ListIssueEvents(c.org, c.repo, c.number)
	if err != nil {
		return fmt.Errorf("failed to list issue events for %s/%s#%d: %w", c.org, c.repo, c.number, err)
	}

	var errs []error
	for _, label := range sets.List(c.labels) {
		actor := lastLabelActor(events, label)
		if actor == "" {
			// Unknown actor: leave the label alone to avoid false positives.
			log.WithField("label", label).Debug("Label actor unknown or label already removed, skipping.")
			continue
		}
		if err := s.enforceLabel(log.WithField("label", label), c.org, c.repo, c.number, label, actor); err != nil {
			errs = append(errs, err)
		}
	}
	return utilerrors.NewAggregate(errs)
}

// collectScopeSearches groups the protected labels by the scope they must be searched in, so each
// scope needs only one search. `approved` and `lgtm` are protected wherever the plugin is enabled;
// a restricted label only in the scope it is restricted in. Labels already searched org-wide are
// not searched again per repo.
func collectScopeSearches(config *plugins.Configuration, orgs, repos []string) []scopeSearch {
	orgWide := map[string]sets.Set[string]{}
	repoScoped := map[string]sets.Set[string]{}
	addTo := func(scopes map[string]sets.Set[string], scope, label string) {
		if scopes[scope] == nil {
			scopes[scope] = sets.New[string]()
		}
		scopes[scope].Insert(strings.ToLower(label))
	}

	alwaysProtected := []string{labels.Approved, labels.LGTM}

	for _, org := range orgs {
		for _, label := range alwaysProtected {
			addTo(orgWide, org, label)
		}
		for key, restrictedLabels := range config.Label.RestrictedLabels {
			for _, restricted := range restrictedLabels {
				switch {
				case key == "*" || key == org:
					addTo(orgWide, org, restricted.Label)
				case strings.HasPrefix(key, org+"/"):
					addTo(repoScoped, key, restricted.Label)
				}
			}
		}
	}

	for _, orgRepo := range repos {
		org, _, ok := splitOrgRepo(orgRepo)
		if !ok {
			continue
		}
		for _, label := range alwaysProtected {
			addTo(repoScoped, orgRepo, label)
		}
		for key, restrictedLabels := range config.Label.RestrictedLabels {
			for _, restricted := range restrictedLabels {
				if key == "*" || key == org || key == orgRepo {
					addTo(repoScoped, orgRepo, restricted.Label)
				}
			}
		}
	}

	var searches []scopeSearch
	for org, labelSet := range orgWide {
		searches = append(searches, scopeSearch{org: org, labels: labelSet})
	}
	for orgRepo, labelSet := range repoScoped {
		org, repo, _ := splitOrgRepo(orgRepo)
		// Drop labels already covered by the org-wide search for this org.
		remaining := labelSet.Difference(orgWide[org])
		if remaining.Len() == 0 {
			continue
		}
		searches = append(searches, scopeSearch{org: org, repo: repo, labels: remaining})
	}
	return searches
}

// configFingerprint returns a deterministic fingerprint of the plugin-relevant configuration: the
// enabled scopes and the restricted-label rules (labels, allowed users and teams). The
// reconciliation forces a full pass whenever it changes.
func configFingerprint(config *plugins.Configuration, orgs, repos []string) string {
	scopes := append(append([]string{}, orgs...), repos...)
	sort.Strings(scopes)

	restricted := make(map[string][]plugins.RestrictedLabel, len(config.Label.RestrictedLabels))
	for key, entries := range config.Label.RestrictedLabels {
		normalized := make([]plugins.RestrictedLabel, len(entries))
		copy(normalized, entries)
		for i := range normalized {
			normalized[i].AllowedUsers = sortedCopy(normalized[i].AllowedUsers)
			normalized[i].AllowedTeams = sortedCopy(normalized[i].AllowedTeams)
		}
		sort.Slice(normalized, func(i, j int) bool { return normalized[i].Label < normalized[j].Label })
		restricted[key] = normalized
	}

	data, _ := json.Marshal(struct {
		Scopes           []string
		RestrictedLabels map[string][]plugins.RestrictedLabel
	}{Scopes: scopes, RestrictedLabels: restricted})
	return string(data)
}

func sortedCopy(in []string) []string {
	out := append([]string{}, in...)
	sort.Strings(out)
	return out
}

// lastLabelActor returns who currently has the label added, or an empty string if that cannot be
// determined or the label was removed again after being added. The latter covers a stale search
// result still listing a label that the webhook path has already removed.
func lastLabelActor(events []github.ListedIssueEvent, label string) string {
	var actor string
	for _, event := range events {
		if !strings.EqualFold(event.Label.Name, label) {
			continue
		}
		switch event.Event {
		case github.IssueActionLabeled:
			actor = event.Actor.Login
		case github.IssueActionUnlabeled:
			actor = ""
		}
	}
	return actor
}

// matchedLabels returns the pull request's labels that are in want, matched case-insensitively and
// returned in their original casing.
func matchedLabels(issue github.Issue, want sets.Set[string]) []string {
	var matched []string
	for _, label := range issue.Labels {
		if want.Has(strings.ToLower(label.Name)) {
			matched = append(matched, label.Name)
		}
	}
	return matched
}

// parseIssueRepo extracts the org and repo from a GitHub issue/PR HTML URL such as
// https://github.com/<org>/<repo>/pull/<number>.
func parseIssueRepo(htmlURL string) (org, repo string, ok bool) {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(htmlURL, "https://"), "http://")
	parts := strings.Split(trimmed, "/")
	// parts: [host, org, repo, "pull", number]
	if len(parts) < 3 || parts[1] == "" || parts[2] == "" {
		return "", "", false
	}
	return parts[1], parts[2], true
}

func splitOrgRepo(orgRepo string) (org, repo string, ok bool) {
	parts := strings.SplitN(orgRepo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func scopeString(org, repo string) string {
	if repo == "" {
		return org
	}
	return org + "/" + repo
}
