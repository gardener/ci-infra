// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/prow/pkg/github"
	"sigs.k8s.io/prow/pkg/github/fakegithub"
	"sigs.k8s.io/prow/pkg/plugins"
)

const (
	testOrg    = "gardener"
	testRepo   = "gardener"
	testNumber = 1
	testTeam   = "maintainers"

	// manualApprovalNotification mimics the notification the upstream approve plugin posts when
	// the "approved" label is added manually.
	manualApprovalNotification = "[APPROVALNOTIFIER] This PR is **APPROVED**\n\n" +
		"Approval requirements bypassed by manually added approval.\n\n" +
		"This pull-request has been approved by:"
)

func countWarnings(comments []github.IssueComment) int {
	count := 0
	for _, comment := range comments {
		if strings.Contains(comment.Body, warningMarkerPrefix) {
			count++
		}
	}
	return count
}

func warningBody(comments []github.IssueComment) string {
	for _, comment := range comments {
		if strings.Contains(comment.Body, warningMarkerPrefix) {
			return comment.Body
		}
	}
	return ""
}

func hasManualApprovalNotification(comments []github.IssueComment) bool {
	for _, comment := range comments {
		if strings.HasPrefix(comment.Body, "[APPROVALNOTIFIER]") && strings.Contains(comment.Body, "Approval requirements bypassed by manually added approval.") {
			return true
		}
	}
	return false
}

var _ = Describe("Server", func() {
	Describe("#handlePullRequest", func() {
		var (
			fakeGithub *fakegithub.FakeClient
			server     *Server
		)

		BeforeEach(func() {
			fakeGithub = fakegithub.NewFakeClient()

			configAgent := &plugins.ConfigAgent{}
			configAgent.Set(&plugins.Configuration{
				Label: plugins.Label{
					RestrictedLabels: map[string][]plugins.RestrictedLabel{
						testOrg: {
							{
								Label:        "skip-review",
								AllowedUsers: []string{"renovate[bot]"},
								AllowedTeams: []string{testTeam},
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

		type testCase struct {
			action       github.PullRequestEventAction
			label        string
			user         string
			seedComments []github.IssueComment
			teamMembers  []string

			expectLabelRemoved         bool
			expectWarnings             int
			expectNotificationDeletion bool
			expectCommand              string
		}

		DescribeTable("enforcing that protected labels are only set by allowed users",
			func(tc testCase) {
				if len(tc.seedComments) > 0 {
					fakeGithub.IssueComments[testNumber] = append([]github.IssueComment{}, tc.seedComments...)
					for _, comment := range tc.seedComments {
						if comment.ID > fakeGithub.IssueCommentID {
							fakeGithub.IssueCommentID = comment.ID
						}
					}
				}

				if len(tc.teamMembers) > 0 {
					fakeGithub.Teams = map[string]map[string]fakegithub.TeamWithMembers{
						testOrg: {testTeam: {Members: sets.New(tc.teamMembers...)}},
					}
				}

				action := tc.action
				if action == "" {
					action = github.PullRequestActionLabeled
				}
				event := github.PullRequestEvent{
					Action: action,
					Number: testNumber,
					Repo:   github.Repo{Owner: github.User{Login: testOrg}, Name: testRepo},
					Label:  github.Label{Name: tc.label},
					Sender: github.User{Login: tc.user},
				}

				Expect(server.handlePullRequest(server.log, event)).To(Succeed())

				if tc.expectLabelRemoved {
					Expect(fakeGithub.IssueLabelsRemoved).To(ConsistOf(testOrg + "/" + testRepo + "#1:" + tc.label))
				} else {
					Expect(fakeGithub.IssueLabelsRemoved).To(BeEmpty())
				}

				Expect(countWarnings(fakeGithub.IssueComments[testNumber])).To(Equal(tc.expectWarnings), "number of warning comments")

				if tc.expectCommand != "" {
					Expect(warningBody(fakeGithub.IssueComments[testNumber])).To(ContainSubstring("`"+tc.expectCommand+"`"), "warning should suggest the Prow command")
				}

				if tc.expectNotificationDeletion {
					Expect(hasManualApprovalNotification(fakeGithub.IssueComments[testNumber])).To(BeFalse(), "manual approval notification should be deleted")
				}
			},

			Entry("removes approved label added by a non-prow user and deletes the manual approval notification", testCase{
				label:                      "approved",
				user:                       "mallory",
				seedComments:               []github.IssueComment{{ID: 1, Body: manualApprovalNotification, User: github.User{Login: fakegithub.Bot}}},
				expectLabelRemoved:         true,
				expectWarnings:             1,
				expectNotificationDeletion: true,
				expectCommand:              "/approve",
			}),
			Entry("removes lgtm label added by a non-prow user", testCase{
				label:              "lgtm",
				user:               "mallory",
				expectLabelRemoved: true,
				expectWarnings:     1,
				expectCommand:      "/lgtm",
			}),
			Entry("keeps approved label added by the prow user", testCase{
				label:              "approved",
				user:               fakegithub.Bot + "[bot]",
				seedComments:       []github.IssueComment{{ID: 1, Body: manualApprovalNotification, User: github.User{Login: fakegithub.Bot}}},
				expectLabelRemoved: false,
				expectWarnings:     0,
			}),
			Entry("keeps lgtm label added by the prow user", testCase{
				label:              "lgtm",
				user:               fakegithub.Bot + "[bot]",
				expectLabelRemoved: false,
				expectWarnings:     0,
			}),
			Entry("keeps restricted label added by an allowed user", testCase{
				label:              "skip-review",
				user:               "renovate[bot]",
				expectLabelRemoved: false,
				expectWarnings:     0,
			}),
			Entry("keeps restricted label added by a member of an allowed team", testCase{
				label:              "skip-review",
				user:               "alice",
				teamMembers:        []string{"alice"},
				expectLabelRemoved: false,
				expectWarnings:     0,
			}),
			Entry("removes restricted label added by a disallowed user", testCase{
				label:              "skip-review",
				user:               "mallory",
				expectLabelRemoved: true,
				expectWarnings:     1,
				expectCommand:      "/label skip-review",
			}),
			Entry("ignores an unprotected label", testCase{
				label:              "kind/bug",
				user:               "mallory",
				expectLabelRemoved: false,
				expectWarnings:     0,
			}),
			Entry("replaces a previous warning for the same label", testCase{
				label:              "approved",
				user:               "mallory",
				seedComments:       []github.IssueComment{{ID: 1, Body: "@mallory an old warning\n" + warningMarker("approved"), User: github.User{Login: fakegithub.Bot}}},
				expectLabelRemoved: true,
				expectWarnings:     1,
			}),
			Entry("keeps a previous warning for a different label", testCase{
				label:              "approved",
				user:               "mallory",
				seedComments:       []github.IssueComment{{ID: 1, Body: "@mallory an old warning\n" + warningMarker("lgtm"), User: github.User{Login: fakegithub.Bot}}},
				expectLabelRemoved: true,
				expectWarnings:     2,
			}),
			Entry("ignores non-labeled actions", testCase{
				action:             github.PullRequestActionOpened,
				label:              "approved",
				user:               "mallory",
				expectLabelRemoved: false,
				expectWarnings:     0,
			}),
		)
	})
})
