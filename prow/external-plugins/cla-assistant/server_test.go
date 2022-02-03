// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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
	"encoding/json"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"k8s.io/test-infra/prow/github"
)

func TestHandleEvent(t *testing.T) {
	type testCase struct {
		name          string
		eventType     string
		event         interface{}
		errorExpected bool
	}

	tests := []testCase{
		{
			name:      "/cla in first line",
			eventType: "issue_comment",
			event: github.IssueCommentEvent{
				Action: github.IssueCommentActionCreated,
				Issue: github.Issue{
					Number:      12345,
					State:       github.PullRequestStateOpen,
					PullRequest: &struct{}{},
				},
				Comment: github.IssueComment{
					Body: "/cla",
				},
				Repo: github.Repo{
					Owner: github.User{
						Login: testOwner,
					},
					Name: testRepo,
				},
			},
			errorExpected: false,
		},
		{
			name:      "/cla in first line",
			eventType: "pull_request_review_comment",
			event: github.ReviewCommentEvent{
				Action: github.ReviewCommentActionCreated,
				PullRequest: github.PullRequest{
					Number: 12345,
					State:  github.PullRequestStateOpen,
				},
				Comment: github.ReviewComment{
					Body: "/cla",
				},
				Repo: github.Repo{
					Owner: github.User{
						Login: testOwner,
					},
					Name: testRepo,
				},
			},
			errorExpected: false,
		},
		{
			name:      "cla context - pending",
			eventType: "status",
			event: github.StatusEvent{
				Repo: github.Repo{
					Owner: github.User{
						Login: testOwner,
					},
					Name: testRepo,
				},
				Context: claGithubContext,
				State:   github.StatusPending,
				SHA:     shaWithPRClaStatusPending,
			},
			errorExpected: false,
		},
		{
			name:          "Emtpy context",
			eventType:     "status",
			event:         github.StatusEvent{},
			errorExpected: false,
		},
		{
			name:          "unasked event type",
			eventType:     "workflow",
			event:         github.WorkflowRunEvent{},
			errorExpected: false,
		},
	}

	log := logrus.StandardLogger().WithField("TestHandleIssueCommentEvent", pluginName)

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				p := newClaAssistantTestPlugin()
				defer p.http.server.Close()
				ingestDataIntoFakeClient(p.fakeClient)
				s := httpServer{log: log, cla: p.plugin}

				payload, err := json.Marshal(test.event)
				assert.NoError(t, err)

				err = s.handleEvent(test.eventType, "asdf1234", payload)
				if test.errorExpected {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
	}

}
