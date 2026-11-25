// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/prow/pkg/github"
	"sigs.k8s.io/prow/pkg/github/fakegithub"
	"sigs.k8s.io/prow/pkg/plugins"
)

// eventCountingClient counts how often ListIssueEvents is called.
type eventCountingClient struct {
	*fakegithub.FakeClient
	listIssueEventsCalls int
}

func (c *eventCountingClient) ListIssueEvents(org, repo string, number int) ([]github.ListedIssueEvent, error) {
	c.listIssueEventsCalls++
	return c.FakeClient.ListIssueEvents(org, repo, number)
}

// primeConfigFingerprint records the current config as the last-seen one, so a following reconcile
// does not treat it as a config change (which would reset the watermark).
func primeConfigFingerprint(s *Server) {
	config := s.configAgent.Config()
	orgs, repos := config.EnabledReposForExternalPlugin(pluginName)
	s.lastConfigFingerprint = configFingerprint(config, orgs, repos)
}

var _ = Describe("Server", func() {
	Describe("#reconcile", func() {
		var (
			fakeGithub *fakegithub.FakeClient
			server     *Server
		)

		BeforeEach(func() {
			fakeGithub = fakegithub.NewFakeClient()

			configAgent := &plugins.ConfigAgent{}
			configAgent.Set(&plugins.Configuration{
				ExternalPlugins: map[string][]plugins.ExternalPlugin{
					testOrg: {{Name: pluginName, Events: []string{"pull_request"}}},
				},
				Label: plugins.Label{
					RestrictedLabels: map[string][]plugins.RestrictedLabel{
						testOrg: {
							{
								Label:        "skip-review",
								AllowedUsers: []string{"renovate[bot]"},
							},
						},
					},
				},
			})

			botUserChecker, err := fakeGithub.BotUserChecker()
			Expect(err).NotTo(HaveOccurred())

			server = &Server{
				ghc:            fakeGithub,
				log:            logrus.NewEntry(logrus.StandardLogger()),
				configAgent:    configAgent,
				botUserChecker: botUserChecker,
			}
		})

		// seedPR registers an open pull request carrying the given label, added by the given actor.
		seedPR := func(label, actor string) {
			fakeGithub.Issues[testNumber] = &github.Issue{
				Number:  testNumber,
				State:   "open",
				HTMLURL: "https://github.com/" + testOrg + "/" + testRepo + "/pull/1",
				Labels:  []github.Label{{Name: label}},
			}
			fakeGithub.IssueEvents[testNumber] = []github.ListedIssueEvent{
				{Event: github.IssueActionLabeled, Label: github.Label{Name: label}, Actor: github.User{Login: actor}},
			}
		}

		labelRemoved := func(label string) bool {
			return sets.New(fakeGithub.IssueLabelsRemoved...).Has(testOrg + "/" + testRepo + "#1:" + label)
		}

		It("removes a protected label that a non-prow user added", func() {
			seedPR("approved", "mallory")

			Expect(server.reconcile()).To(Succeed())

			Expect(labelRemoved("approved")).To(BeTrue())
			Expect(countWarnings(fakeGithub.IssueComments[testNumber])).To(Equal(1))
		})

		It("keeps a protected label that the prow user added", func() {
			seedPR("lgtm", fakegithub.Bot+"[bot]")

			Expect(server.reconcile()).To(Succeed())

			Expect(fakeGithub.IssueLabelsRemoved).To(BeEmpty())
			Expect(countWarnings(fakeGithub.IssueComments[testNumber])).To(Equal(0))
		})

		It("keeps a restricted label that an allowed user added", func() {
			seedPR("skip-review", "renovate[bot]")

			Expect(server.reconcile()).To(Succeed())

			Expect(fakeGithub.IssueLabelsRemoved).To(BeEmpty())
			Expect(countWarnings(fakeGithub.IssueComments[testNumber])).To(Equal(0))
		})

		It("removes a restricted label that a disallowed user added", func() {
			seedPR("skip-review", "mallory")

			Expect(server.reconcile()).To(Succeed())

			Expect(labelRemoved("skip-review")).To(BeTrue())
			Expect(countWarnings(fakeGithub.IssueComments[testNumber])).To(Equal(1))
		})

		It("fetches a pull request's events only once when it carries multiple protected labels", func() {
			fakeGithub.Issues[testNumber] = &github.Issue{
				Number:  testNumber,
				State:   "open",
				HTMLURL: "https://github.com/" + testOrg + "/" + testRepo + "/pull/1",
				Labels:  []github.Label{{Name: "approved"}, {Name: "lgtm"}},
			}
			fakeGithub.IssueEvents[testNumber] = []github.ListedIssueEvent{
				{Event: github.IssueActionLabeled, Label: github.Label{Name: "approved"}, Actor: github.User{Login: "mallory"}},
				{Event: github.IssueActionLabeled, Label: github.Label{Name: "lgtm"}, Actor: github.User{Login: "mallory"}},
			}
			counter := &eventCountingClient{FakeClient: fakeGithub}
			server.ghc = counter

			Expect(server.reconcile()).To(Succeed())

			Expect(counter.listIssueEventsCalls).To(Equal(1))
			Expect(fakeGithub.IssueLabelsRemoved).To(ConsistOf(
				testOrg+"/"+testRepo+"#1:approved",
				testOrg+"/"+testRepo+"#1:lgtm",
			))
			// One warning per removed label.
			Expect(countWarnings(fakeGithub.IssueComments[testNumber])).To(Equal(2))
		})

		It("does not act when the label actor cannot be determined", func() {
			fakeGithub.Issues[testNumber] = &github.Issue{
				Number:  testNumber,
				State:   "open",
				HTMLURL: "https://github.com/" + testOrg + "/" + testRepo + "/pull/1",
				Labels:  []github.Label{{Name: "approved"}},
			}
			// No issue events seeded, so the actor is unknown.

			Expect(server.reconcile()).To(Succeed())

			Expect(fakeGithub.IssueLabelsRemoved).To(BeEmpty())
			Expect(countWarnings(fakeGithub.IssueComments[testNumber])).To(Equal(0))
		})

		It("does not act when the label was removed again after being added (stale search result)", func() {
			fakeGithub.Issues[testNumber] = &github.Issue{
				Number:  testNumber,
				State:   "open",
				HTMLURL: "https://github.com/" + testOrg + "/" + testRepo + "/pull/1",
				Labels:  []github.Label{{Name: "approved"}},
			}
			// The label was added by a non-prow user, then already removed again (e.g. by the webhook
			// path); the search index is stale and still lists it.
			fakeGithub.IssueEvents[testNumber] = []github.ListedIssueEvent{
				{Event: github.IssueActionLabeled, Label: github.Label{Name: "approved"}, Actor: github.User{Login: "mallory"}},
				{Event: github.IssueActionUnlabeled, Label: github.Label{Name: "approved"}, Actor: github.User{Login: fakegithub.Bot}},
			}

			Expect(server.reconcile()).To(Succeed())

			Expect(fakeGithub.IssueLabelsRemoved).To(BeEmpty())
			Expect(countWarnings(fakeGithub.IssueComments[testNumber])).To(Equal(0))
		})

		It("acts when the label was removed and then added again by a non-prow user", func() {
			fakeGithub.Issues[testNumber] = &github.Issue{
				Number:  testNumber,
				State:   "open",
				HTMLURL: "https://github.com/" + testOrg + "/" + testRepo + "/pull/1",
				Labels:  []github.Label{{Name: "approved"}},
			}
			fakeGithub.IssueEvents[testNumber] = []github.ListedIssueEvent{
				{Event: github.IssueActionLabeled, Label: github.Label{Name: "approved"}, Actor: github.User{Login: "mallory"}},
				{Event: github.IssueActionUnlabeled, Label: github.Label{Name: "approved"}, Actor: github.User{Login: fakegithub.Bot}},
				{Event: github.IssueActionLabeled, Label: github.Label{Name: "approved"}, Actor: github.User{Login: "mallory"}},
			}

			Expect(server.reconcile()).To(Succeed())

			Expect(fakeGithub.IssueLabelsRemoved).To(ConsistOf(testOrg + "/" + testRepo + "#1:approved"))
			Expect(countWarnings(fakeGithub.IssueComments[testNumber])).To(Equal(1))
		})

		It("does nothing when no repos are configured", func() {
			server.configAgent.Set(&plugins.Configuration{})
			seedPR("approved", "mallory")

			Expect(server.reconcile()).To(Succeed())

			Expect(fakeGithub.IssueLabelsRemoved).To(BeEmpty())
		})

		It("does not remove a repo-restricted label on a pull request in a different repo", func() {
			// foo-bar is restricted only in gardener/ci-infra, but the seeded pull request is in
			// gardener/gardener, where foo-bar is not protected.
			server.configAgent.Set(&plugins.Configuration{
				ExternalPlugins: map[string][]plugins.ExternalPlugin{
					testOrg: {{Name: pluginName, Events: []string{"pull_request"}}},
				},
				Label: plugins.Label{RestrictedLabels: map[string][]plugins.RestrictedLabel{
					"gardener/ci-infra": {{Label: "foo-bar"}},
				}},
			})
			seedPR("foo-bar", "mallory")

			Expect(server.reconcile()).To(Succeed())

			Expect(fakeGithub.IssueLabelsRemoved).To(BeEmpty())
			Expect(countWarnings(fakeGithub.IssueComments[testNumber])).To(Equal(0))
		})

		Context("with a label restricted both org-wide and per-repo with different allowed users", func() {
			BeforeEach(func() {
				// Mirrors the real config: skip-review is restricted org-wide and additionally in
				// gardener/ci-infra with different allowed users.
				server.configAgent.Set(&plugins.Configuration{
					ExternalPlugins: map[string][]plugins.ExternalPlugin{
						testOrg: {{Name: pluginName, Events: []string{"pull_request"}}},
					},
					Label: plugins.Label{RestrictedLabels: map[string][]plugins.RestrictedLabel{
						"gardener":          {{Label: "skip-review", AllowedUsers: []string{"renovate[bot]"}}},
						"gardener/ci-infra": {{Label: "skip-review", AllowedUsers: []string{"gardener-prow-autobumper[bot]"}}},
					}},
				})
			})

			// seedCIInfraPR registers a gardener/ci-infra pull request with skip-review added by user.
			seedCIInfraPR := func(user string) {
				fakeGithub.Issues[testNumber] = &github.Issue{
					Number:  testNumber,
					State:   "open",
					HTMLURL: "https://github.com/gardener/ci-infra/pull/1",
					Labels:  []github.Label{{Name: "skip-review"}},
				}
				fakeGithub.IssueEvents[testNumber] = []github.ListedIssueEvent{
					{Event: github.IssueActionLabeled, Label: github.Label{Name: "skip-review"}, Actor: github.User{Login: user}},
				}
			}

			It("keeps the label for a user allowed only by the repo config", func() {
				seedCIInfraPR("gardener-prow-autobumper[bot]")

				Expect(server.reconcile()).To(Succeed())

				Expect(fakeGithub.IssueLabelsRemoved).To(BeEmpty())
			})

			It("removes the label for a user allowed in neither config", func() {
				seedCIInfraPR("mallory")

				Expect(server.reconcile()).To(Succeed())

				Expect(fakeGithub.IssueLabelsRemoved).To(ConsistOf("gardener/ci-infra#1:skip-review"))
			})
		})

		It("skips pull requests not updated since the last successful pass", func() {
			updated := time.Now().Add(-time.Hour)
			fakeGithub.Issues[testNumber] = &github.Issue{
				Number:    testNumber,
				State:     "open",
				HTMLURL:   "https://github.com/" + testOrg + "/" + testRepo + "/pull/1",
				Labels:    []github.Label{{Name: "skip-review"}},
				UpdatedAt: updated,
			}
			fakeGithub.IssueEvents[testNumber] = []github.ListedIssueEvent{
				{Event: github.IssueActionLabeled, Label: github.Label{Name: "skip-review"}, Actor: github.User{Login: "renovate[bot]"}},
			}
			counter := &eventCountingClient{FakeClient: fakeGithub}
			server.ghc = counter
			primeConfigFingerprint(server)
			// Well beyond the search-index-lag buffer before the pull request's update.
			server.lastReconcile = updated.Add(10 * time.Minute)

			Expect(server.reconcile()).To(Succeed())

			Expect(counter.listIssueEventsCalls).To(Equal(0))
		})

		It("processes pull requests updated since the last successful pass", func() {
			updated := time.Now()
			fakeGithub.Issues[testNumber] = &github.Issue{
				Number:    testNumber,
				State:     "open",
				HTMLURL:   "https://github.com/" + testOrg + "/" + testRepo + "/pull/1",
				Labels:    []github.Label{{Name: "skip-review"}},
				UpdatedAt: updated,
			}
			fakeGithub.IssueEvents[testNumber] = []github.ListedIssueEvent{
				{Event: github.IssueActionLabeled, Label: github.Label{Name: "skip-review"}, Actor: github.User{Login: "renovate[bot]"}},
			}
			counter := &eventCountingClient{FakeClient: fakeGithub}
			server.ghc = counter
			primeConfigFingerprint(server)
			server.lastReconcile = updated.Add(-time.Minute)

			Expect(server.reconcile()).To(Succeed())

			Expect(counter.listIssueEventsCalls).To(Equal(1))
		})

		It("re-processes pull requests updated shortly before the last pass (search index lag)", func() {
			updated := time.Now()
			fakeGithub.Issues[testNumber] = &github.Issue{
				Number:    testNumber,
				State:     "open",
				HTMLURL:   "https://github.com/" + testOrg + "/" + testRepo + "/pull/1",
				Labels:    []github.Label{{Name: "skip-review"}},
				UpdatedAt: updated,
			}
			fakeGithub.IssueEvents[testNumber] = []github.ListedIssueEvent{
				{Event: github.IssueActionLabeled, Label: github.Label{Name: "skip-review"}, Actor: github.User{Login: "renovate[bot]"}},
			}
			counter := &eventCountingClient{FakeClient: fakeGithub}
			server.ghc = counter
			primeConfigFingerprint(server)
			// Watermark is past the update but within the search-index-lag buffer.
			server.lastReconcile = updated.Add(2 * time.Minute)

			Expect(server.reconcile()).To(Succeed())

			Expect(counter.listIssueEventsCalls).To(Equal(1))
		})

		It("re-processes all pull requests after a configuration change", func() {
			updated := time.Now().Add(-time.Hour)
			fakeGithub.Issues[testNumber] = &github.Issue{
				Number:    testNumber,
				State:     "open",
				HTMLURL:   "https://github.com/" + testOrg + "/" + testRepo + "/pull/1",
				Labels:    []github.Label{{Name: "skip-review"}},
				UpdatedAt: updated,
			}
			fakeGithub.IssueEvents[testNumber] = []github.ListedIssueEvent{
				{Event: github.IssueActionLabeled, Label: github.Label{Name: "skip-review"}, Actor: github.User{Login: "renovate[bot]"}},
			}
			counter := &eventCountingClient{FakeClient: fakeGithub}
			server.ghc = counter

			// First pass records the config fingerprint and the watermark.
			Expect(server.reconcile()).To(Succeed())
			Expect(counter.listIssueEventsCalls).To(Equal(1))

			// With unchanged config and the watermark past the update, the pull request is skipped.
			server.lastReconcile = updated.Add(10 * time.Minute)
			Expect(server.reconcile()).To(Succeed())
			Expect(counter.listIssueEventsCalls).To(Equal(1))

			// A configuration change forces a full pass, so the pull request is processed again.
			server.configAgent.Set(&plugins.Configuration{
				ExternalPlugins: map[string][]plugins.ExternalPlugin{
					testOrg: {{Name: pluginName, Events: []string{"pull_request"}}},
				},
				Label: plugins.Label{RestrictedLabels: map[string][]plugins.RestrictedLabel{
					testOrg: {{Label: "skip-review", AllowedUsers: []string{"renovate[bot]", "another[bot]"}}},
				}},
			})
			server.lastReconcile = updated.Add(10 * time.Minute)
			Expect(server.reconcile()).To(Succeed())
			Expect(counter.listIssueEventsCalls).To(Equal(2))
		})
	})

	Describe("#collectScopeSearches", func() {
		It("searches the always-protected labels across the whole enabled org", func() {
			searches := collectScopeSearches(&plugins.Configuration{}, []string{"gardener"}, nil)

			Expect(searches).To(ConsistOf(
				scopeSearch{org: "gardener", labels: sets.New("approved", "lgtm")},
			))
		})

		It("searches an org-wide restricted label in the org-wide search", func() {
			config := &plugins.Configuration{Label: plugins.Label{RestrictedLabels: map[string][]plugins.RestrictedLabel{
				"gardener": {{Label: "skip-review"}},
			}}}

			searches := collectScopeSearches(config, []string{"gardener"}, nil)

			Expect(searches).To(ConsistOf(
				scopeSearch{org: "gardener", labels: sets.New("approved", "lgtm", "skip-review")},
			))
		})

		It("searches a repo-scoped restricted label only in that repo", func() {
			config := &plugins.Configuration{Label: plugins.Label{RestrictedLabels: map[string][]plugins.RestrictedLabel{
				"gardener/ci-infra": {{Label: "skip-review"}},
			}}}

			searches := collectScopeSearches(config, []string{"gardener"}, nil)

			Expect(searches).To(ConsistOf(
				scopeSearch{org: "gardener", labels: sets.New("approved", "lgtm")},
				scopeSearch{org: "gardener", repo: "ci-infra", labels: sets.New("skip-review")},
			))
		})

		It("searches a global restricted label in the org-wide search", func() {
			config := &plugins.Configuration{Label: plugins.Label{RestrictedLabels: map[string][]plugins.RestrictedLabel{
				"*": {{Label: "skip-review"}},
			}}}

			searches := collectScopeSearches(config, []string{"gardener"}, nil)

			Expect(searches).To(ConsistOf(
				scopeSearch{org: "gardener", labels: sets.New("approved", "lgtm", "skip-review")},
			))
		})

		It("does not search a repo when the label is already searched org-wide", func() {
			config := &plugins.Configuration{Label: plugins.Label{RestrictedLabels: map[string][]plugins.RestrictedLabel{
				"gardener":          {{Label: "skip-review"}},
				"gardener/ci-infra": {{Label: "skip-review"}},
			}}}

			searches := collectScopeSearches(config, []string{"gardener"}, nil)

			Expect(searches).To(ConsistOf(
				scopeSearch{org: "gardener", labels: sets.New("approved", "lgtm", "skip-review")},
			))
		})

		It("scopes every protected label to the repo when only a repo is enabled", func() {
			config := &plugins.Configuration{Label: plugins.Label{RestrictedLabels: map[string][]plugins.RestrictedLabel{
				"gardener":          {{Label: "skip-review"}},
				"gardener/ci-infra": {{Label: "fast-track"}},
			}}}

			searches := collectScopeSearches(config, nil, []string{"gardener/ci-infra"})

			Expect(searches).To(ConsistOf(
				scopeSearch{org: "gardener", repo: "ci-infra", labels: sets.New("approved", "lgtm", "skip-review", "fast-track")},
			))
		})
	})
})
