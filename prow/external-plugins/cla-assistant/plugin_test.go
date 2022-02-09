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
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	githubql "github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

const (
	httpTestTimeout           time.Duration = time.Millisecond * 500
	testOwner                 string        = "TestOrg"
	testRepo                  string        = "test-repo"
	shaWithPR                 string        = "abc123"
	shaWithPRClaStatusPending string        = "pppp1234"
	shaWithPRAndYesLabel      string        = "xyz123yes"
	shaWithPRAndNoLabel       string        = "xyz123no"
	shaWithoutPR              string        = "nonono1234"
)

var (
	prLabelYes github.Label = github.Label{
		Name:        labelClaYes,
		Description: labelClaYes,
	}
	prLabelNo github.Label = github.Label{
		Name:        labelClaNo,
		Description: labelClaNo,
	}
)

func TestHandleIssueCommentEvent(t *testing.T) {
	type testCase struct {
		name                  string
		ice                   github.IssueCommentEvent
		reachedServerExpected bool
	}

	tests := []testCase{
		{
			name: "/cla in first line",
			ice: github.IssueCommentEvent{
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
			reachedServerExpected: true,
		},
		{
			name: "/cla in second line",
			ice: github.IssueCommentEvent{
				Action: github.IssueCommentActionCreated,
				Issue: github.Issue{
					Number:      12345,
					State:       github.PullRequestStateOpen,
					PullRequest: &struct{}{},
				},
				Comment: github.IssueComment{
					Body: "TestTestTest\n/cla",
				},
				Repo: github.Repo{
					Owner: github.User{
						Login: testOwner,
					},
					Name: testRepo,
				},
			},
			reachedServerExpected: true,
		},
		{
			name: "/cla with leading space",
			ice: github.IssueCommentEvent{
				Action: github.IssueCommentActionCreated,
				Issue: github.Issue{
					Number:      12345,
					State:       github.PullRequestStateOpen,
					PullRequest: &struct{}{},
				},
				Comment: github.IssueComment{
					Body: " /cla",
				},
				Repo: github.Repo{
					Owner: github.User{
						Login: testOwner,
					},
					Name: testRepo,
				},
			},
			reachedServerExpected: false,
		},
		{
			name: "/cla comment in non PR issue",
			ice: github.IssueCommentEvent{
				Action: github.IssueCommentActionCreated,
				Issue: github.Issue{
					Number:      12345,
					State:       github.PullRequestStateOpen,
					PullRequest: nil,
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
			reachedServerExpected: false,
		},
		{
			name: "/cla in first line with PR closed",
			ice: github.IssueCommentEvent{
				Action: github.IssueCommentActionCreated,
				Issue: github.Issue{
					Number:      12345,
					State:       github.PullRequestStateClosed,
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
			reachedServerExpected: false,
		},
		{
			name: "/cla in first line comment edited",
			ice: github.IssueCommentEvent{
				Action: github.IssueCommentActionEdited,
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
			reachedServerExpected: false,
		},
	}

	log := logrus.StandardLogger().WithField("TestHandleIssueCommentEvent", pluginName)

	p := newClaAssistantTestPlugin()
	defer p.http.server.Close()

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				p.http.serverReached = false
				err := p.plugin.handleIssueCommentEvent(log, &test.ice)
				assert.NoError(t, err)
				assert.Equal(t, test.reachedServerExpected, p.http.serverReached)
			})
	}
}

func TestHandleReviewCommentEvent(t *testing.T) {
	type testCase struct {
		name                  string
		rce                   github.ReviewCommentEvent
		reachedServerExpected bool
	}

	tests := []testCase{
		{
			name: "/cla in first line",
			rce: github.ReviewCommentEvent{
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
			reachedServerExpected: true,
		},
		{
			name: "/cla in second line",
			rce: github.ReviewCommentEvent{
				Action: github.ReviewCommentActionCreated,
				PullRequest: github.PullRequest{
					Number: 12345,
					State:  github.PullRequestStateOpen,
				},
				Comment: github.ReviewComment{
					Body: "TestTestTest\n/cla",
				},
				Repo: github.Repo{
					Owner: github.User{
						Login: testOwner,
					},
					Name: testRepo,
				},
			},
			reachedServerExpected: true,
		},
		{
			name: "/cla with leading space",
			rce: github.ReviewCommentEvent{
				Action: github.ReviewCommentActionCreated,
				PullRequest: github.PullRequest{
					Number: 12345,
					State:  github.PullRequestStateOpen,
				},
				Comment: github.ReviewComment{
					Body: " /cla",
				},
				Repo: github.Repo{
					Owner: github.User{
						Login: testOwner,
					},
					Name: testRepo,
				},
			},
			reachedServerExpected: false,
		},
		{
			name: "/cla in first line with PR closed",
			rce: github.ReviewCommentEvent{
				Action: github.ReviewCommentActionCreated,
				PullRequest: github.PullRequest{
					Number: 12345,
					State:  github.PullRequestStateClosed,
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
			reachedServerExpected: false,
		},
		{
			name: "/cla in first line comment edited",
			rce: github.ReviewCommentEvent{
				Action: github.ReviewCommentActionEdited,
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
			reachedServerExpected: false,
		},
	}

	log := logrus.StandardLogger().WithField("TestHandleReviewCommentEvent", pluginName)

	p := newClaAssistantTestPlugin()
	defer p.http.server.Close()

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				p.http.serverReached = false
				err := p.plugin.handleReviewCommentEvent(log, &test.rce)
				assert.NoError(t, err)
				assert.Equal(t, test.reachedServerExpected, p.http.serverReached)
			})
	}
}

func TestHandleReviewEvent(t *testing.T) {
	type testCase struct {
		name                  string
		pre                   github.ReviewEvent
		reachedServerExpected bool
	}

	tests := []testCase{
		{
			name: "/cla in first line",
			pre: github.ReviewEvent{
				Action: github.ReviewActionSubmitted,
				PullRequest: github.PullRequest{
					Number: 12345,
					State:  github.PullRequestStateOpen,
				},
				Review: github.Review{
					Body: "/cla",
				},
				Repo: github.Repo{
					Owner: github.User{
						Login: testOwner,
					},
					Name: testRepo,
				},
			},
			reachedServerExpected: true,
		},
		{
			name: "/cla in second line",
			pre: github.ReviewEvent{
				Action: github.ReviewActionSubmitted,
				PullRequest: github.PullRequest{
					Number: 12345,
					State:  github.PullRequestStateOpen,
				},
				Review: github.Review{
					Body: "TestTestTest\n/cla",
				},
				Repo: github.Repo{
					Owner: github.User{
						Login: testOwner,
					},
					Name: testRepo,
				},
			},
			reachedServerExpected: true,
		},
		{
			name: "/cla with leading space",
			pre: github.ReviewEvent{
				Action: github.ReviewActionSubmitted,
				PullRequest: github.PullRequest{
					Number: 12345,
					State:  github.PullRequestStateOpen,
				},
				Review: github.Review{
					Body: " /cla",
				},
				Repo: github.Repo{
					Owner: github.User{
						Login: testOwner,
					},
					Name: testRepo,
				},
			},
			reachedServerExpected: false,
		},
		{
			name: "/cla in first line with PR closed",
			pre: github.ReviewEvent{
				Action: github.ReviewActionSubmitted,
				PullRequest: github.PullRequest{
					Number: 12345,
					State:  github.PullRequestStateClosed,
				},
				Review: github.Review{
					Body: "/cla",
				},
				Repo: github.Repo{
					Owner: github.User{
						Login: testOwner,
					},
					Name: testRepo,
				},
			},
			reachedServerExpected: false,
		},
		{
			name: "/cla in first line comment edited",
			pre: github.ReviewEvent{
				Action: github.ReviewActionDismissed,
				PullRequest: github.PullRequest{
					Number: 12345,
					State:  github.PullRequestStateOpen,
				},
				Review: github.Review{
					Body: "/cla",
				},
				Repo: github.Repo{
					Owner: github.User{
						Login: testOwner,
					},
					Name: testRepo,
				},
			},
			reachedServerExpected: false,
		},
	}

	log := logrus.StandardLogger().WithField("TestHandleReviewCommentEvent", pluginName)

	p := newClaAssistantTestPlugin()
	defer p.http.server.Close()

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				p.http.serverReached = false
				err := p.plugin.handleReviewEvent(log, &test.pre)
				assert.NoError(t, err)
				assert.Equal(t, test.reachedServerExpected, p.http.serverReached)
			})
	}
}

func TestHandleStatusEvent(t *testing.T) {
	type testCase struct {
		name                 string
		se                   github.StatusEvent
		errorExpected        bool
		expectedLabelAdded   string
		expectedLabelRemoved string
	}

	tests := []testCase{
		{
			name:                 "Emtpy context",
			se:                   github.StatusEvent{},
			errorExpected:        true,
			expectedLabelAdded:   "",
			expectedLabelRemoved: "",
		},
		{
			name: "cla context - pending",
			se: github.StatusEvent{
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
			errorExpected:        false,
			expectedLabelAdded:   "",
			expectedLabelRemoved: "",
		},
		{
			name: "different event - cla pending",
			se: github.StatusEvent{
				Repo: github.Repo{
					Owner: github.User{
						Login: testOwner,
					},
					Name: testRepo,
				},
				Context: "tide",
				State:   github.StatusSuccess,
				SHA:     shaWithPRClaStatusPending,
			},
			errorExpected:        false,
			expectedLabelAdded:   "",
			expectedLabelRemoved: "",
		},
		{
			name: "different event - no cla status",
			se: github.StatusEvent{
				Repo: github.Repo{
					Owner: github.User{
						Login: testOwner,
					},
					Name: testRepo,
				},
				Context: "tide",
				State:   github.StatusSuccess,
				SHA:     "oooo0000",
			},
			errorExpected:        false,
			expectedLabelAdded:   "",
			expectedLabelRemoved: "",
		},
		{
			name: "different event - cla success - no PR",
			se: github.StatusEvent{
				Repo: github.Repo{
					Owner: github.User{
						Login: testOwner,
					},
					Name: testRepo,
				},
				Context: "tide",
				State:   github.StatusSuccess,
				SHA:     shaWithoutPR,
			},
			errorExpected:        false,
			expectedLabelAdded:   "",
			expectedLabelRemoved: "",
		},
		{
			name: "cla context - success - PR found",
			se: github.StatusEvent{
				Repo: github.Repo{
					Owner: github.User{
						Login: testOwner,
					},
					Name: testRepo,
				},
				Context: claGithubContext,
				State:   github.StatusSuccess,
				SHA:     shaWithPR,
			},
			errorExpected:        false,
			expectedLabelAdded:   labelClaYes,
			expectedLabelRemoved: "",
		},
		{
			name: "cla context - failed - PR found",
			se: github.StatusEvent{
				Repo: github.Repo{
					Owner: github.User{
						Login: testOwner,
					},
					Name: testRepo,
				},
				Context: claGithubContext,
				State:   github.StatusFailure,
				SHA:     shaWithPR,
			},
			errorExpected:        false,
			expectedLabelAdded:   labelClaNo,
			expectedLabelRemoved: "",
		},
		{
			name: "cla context - success - PR found - existing cla no label",
			se: github.StatusEvent{
				Repo: github.Repo{
					Owner: github.User{
						Login: testOwner,
					},
					Name: testRepo,
				},
				Context: claGithubContext,
				State:   github.StatusSuccess,
				SHA:     shaWithPRAndNoLabel,
			},
			errorExpected:        false,
			expectedLabelAdded:   labelClaYes,
			expectedLabelRemoved: labelClaNo,
		},
		{
			name: "cla context - failed - PR found - existing cla yes label",
			se: github.StatusEvent{
				Repo: github.Repo{
					Owner: github.User{
						Login: testOwner,
					},
					Name: testRepo,
				},
				Context: claGithubContext,
				State:   github.StatusFailure,
				SHA:     shaWithPRAndYesLabel,
			},
			errorExpected:        false,
			expectedLabelAdded:   labelClaNo,
			expectedLabelRemoved: labelClaYes,
		},
		{
			name: "cla context - success - PR found - existing cla yes label",
			se: github.StatusEvent{
				Repo: github.Repo{
					Owner: github.User{
						Login: testOwner,
					},
					Name: testRepo,
				},
				Context: claGithubContext,
				State:   github.StatusSuccess,
				SHA:     shaWithPRAndYesLabel,
			},
			errorExpected:        false,
			expectedLabelAdded:   "",
			expectedLabelRemoved: "",
		},
		{
			name: "cla context - failed - PR found - existing cla no label",
			se: github.StatusEvent{
				Repo: github.Repo{
					Owner: github.User{
						Login: testOwner,
					},
					Name: testRepo,
				},
				Context: claGithubContext,
				State:   github.StatusFailure,
				SHA:     shaWithPRAndNoLabel,
			},
			errorExpected:        false,
			expectedLabelAdded:   "",
			expectedLabelRemoved: "",
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
				err := p.plugin.handleStatusEvent(log, &test.se)
				if test.errorExpected {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
				if test.expectedLabelAdded != "" {
					for _, l := range p.fakeClient.IssueLabelsAdded {
						assert.Contains(t, l, test.expectedLabelAdded)
					}
				} else {
					assert.Nil(t, p.fakeClient.IssueLabelsAdded)
				}
				if test.expectedLabelRemoved != "" {
					for _, l := range p.fakeClient.IssueLabelsRemoved {
						assert.Contains(t, l, test.expectedLabelRemoved)
					}
				} else {
					assert.Nil(t, p.fakeClient.IssueLabelsRemoved)
				}
			})
	}
}

func TestForceClaRecheck(t *testing.T) {
	type testCase struct {
		name             string
		httpResponseCode int
		responseTimeout  bool
		recoverFromError bool
		errorExpected    bool
	}

	tests := []testCase{
		{
			name:             "all good",
			httpResponseCode: 200,
			responseTimeout:  false,
			recoverFromError: false,
			errorExpected:    false,
		},
		{
			name:             "HTTP status 403",
			httpResponseCode: 403,
			responseTimeout:  false,
			recoverFromError: false,
			errorExpected:    true,
		},
		{
			name:             "HTTP status 403 with recover",
			httpResponseCode: 403,
			responseTimeout:  false,
			recoverFromError: true,
			errorExpected:    false,
		},
		{
			name:             "Timeout",
			httpResponseCode: 200,
			responseTimeout:  true,
			recoverFromError: false,
			errorExpected:    true,
		},
		{
			name:             "Timeout with recover",
			httpResponseCode: 200,
			responseTimeout:  true,
			recoverFromError: true,
			errorExpected:    false,
		},
	}

	p := newClaAssistantTestPlugin()
	defer p.http.server.Close()
	ingestDataIntoFakeClient(p.fakeClient)

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {

				p.http.setServerParameters(&test.recoverFromError, &test.responseTimeout, &test.httpResponseCode)
				err := p.plugin.enforceClaRecheck(testOwner, testRepo, 1)
				if test.errorExpected {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
	}
}

func TestHelpProvider(t *testing.T) {
	p := newClaAssistantTestPlugin()
	defer p.http.server.Close()
	_, err := p.plugin.helpProvider([]config.OrgRepo{})
	assert.NoError(t, err)
}

// Helper functions
// Test HTTP server
func newClaTestServer() *claTestServer {
	s := claTestServer{httpResponseCode: 200, responseTimeout: false, recoverFromError: false}

	s.server = httptest.NewServer(http.HandlerFunc(s.serveHTTP))

	return &s
}

type claTestServer struct {
	server           *httptest.Server
	httpResponseCode int
	responseTimeout  bool
	recoverFromError bool
	mu               sync.Mutex

	serverReached bool
}

func (c *claTestServer) serveHTTP(w http.ResponseWriter, _ *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.serverReached = true
	if c.responseTimeout {
		time.Sleep(httpTestTimeout + time.Millisecond*1)
	}
	w.WriteHeader(c.httpResponseCode)
	_, err := w.Write([]byte("processed"))
	if err != nil {
		logrus.Errorf("Error writing response: %v", err)
	}
	if c.recoverFromError {
		c.responseTimeout = false
		c.httpResponseCode = 200
	}
}

func (c *claTestServer) setServerParameters(recoverFromError, responseTimeout *bool, httpResponseCode *int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if recoverFromError != nil {
		c.recoverFromError = *recoverFromError
	}
	if responseTimeout != nil {
		c.responseTimeout = *responseTimeout
	}
	if httpResponseCode != nil {
		c.httpResponseCode = *httpResponseCode
	}
}

// cla-assistant test plugin
func newClaAssistantTestPlugin() *claAssistantTestPlugin {
	ghc := newFakeClient()
	s := newClaTestServer()

	p := newClaAssistantPlugin(ghc, logrus.StandardLogger().WithField("plugin-test", pluginName))
	p.hc = s.server.Client()
	p.hc.Timeout = httpTestTimeout
	p.baseURL = s.server.URL
	p.maxRetryTime = time.Second * 2

	return &claAssistantTestPlugin{http: s, plugin: p, fakeClient: ghc}
}

type claAssistantTestPlugin struct {
	plugin     *claAssistantPlugin
	http       *claTestServer
	fakeClient *fakeClient
}

// Github fake client
func newFakeClient() *fakeClient {
	f := fakeClient{*fakegithub.NewFakeClient()}
	return &f
}

type fakeClient struct {
	fakegithub.FakeClient
}

func (f *fakeClient) QueryWithGitHubAppsSupport(ctx context.Context, q interface{}, vars map[string]interface{}, org string) error {

	sq, ok := q.(*searchQuery)
	if !ok {
		return fmt.Errorf("Query type not implemented")
	}

	query, ok := vars["query"].(githubql.String)
	if !ok {
		return fmt.Errorf("No query string")
	}

	queryList := strings.Split(string(query), " ")

	if len(queryList) < 3 {
		return fmt.Errorf("Invalid query")
	}

	sha := queryList[0]

	ownerRepo := strings.TrimPrefix(queryList[1], "repo:")
	owner := ownerRepo[:strings.LastIndex(ownerRepo, "/")]
	repo := ownerRepo[strings.LastIndex(ownerRepo, "/")+1:]

	var prNumbers []int
	for n, cm := range f.CommitMap {
		for _, c := range cm {
			if c.SHA == sha {
				p := n[strings.LastIndex(n, "#")+1:]
				prNumber, err := strconv.Atoi(p)
				if err != nil {
					continue
				}
				prNumbers = append(prNumbers, prNumber)
			}
		}
	}

	for _, prNumber := range prNumbers {
		pr, err := f.GetPullRequest(owner, repo, prNumber)
		if err != nil {
			continue
		}

		var labelNodes []struct{ Name githubql.String }
		for _, l := range pr.Labels {
			labelNodes = append(labelNodes, struct{ Name githubql.String }{Name: githubql.String(l.Name)})
		}

		sq.Search.Nodes = append(sq.Search.Nodes,
			struct {
				PullRequest pullRequest "graphql:\"... on PullRequest\""
			}{PullRequest: pullRequest{
				Number: githubql.Int(pr.Number),
				Author: struct{ Login githubql.String }{Login: "Test-Author"},
				Repository: struct {
					Name  githubql.String
					Owner struct{ Login githubql.String }
				}{Name: (githubql.String)(repo), Owner: struct{ Login githubql.String }{Login: (githubql.String)(owner)}},
				Labels: struct {
					Nodes []struct{ Name githubql.String }
				}{labelNodes},
				Mergeable: githubql.MergeableStateMergeable,
				State:     githubql.PullRequestStateOpen,
			},
			})
	}
	return nil
}

func createCommitMapKey(owner, repo string, pr int) string {
	return fmt.Sprintf("%s/%s#%d", owner, repo, pr)
}

func ingestDataIntoFakeClient(f *fakeClient) {
	// SHA must be convertable to integer
	err := f.CreateStatus(testOwner, testRepo, shaWithPRClaStatusPending, github.Status{Context: claGithubContext, State: github.StatusPending})
	if err != nil {
		logrus.Fatalf("Error creating status: %v", err)
	}
	err = f.CreateStatus(testOwner, testRepo, shaWithoutPR, github.Status{Context: claGithubContext, State: github.StatusSuccess})
	if err != nil {
		logrus.Fatalf("Error creating status: %v", err)
	}
	err = f.CreateStatus(testOwner, testRepo, shaWithPR, github.Status{Context: claGithubContext, State: github.StatusSuccess})
	if err != nil {
		logrus.Fatalf("Error creating status: %v", err)
	}
	err = f.CreateStatus(testOwner, testRepo, shaWithPR, github.Status{Context: claGithubContext, State: github.StatusFailure})
	if err != nil {
		logrus.Fatalf("Error creating status: %v", err)
	}
	err = f.AddRepoLabel(testOwner, testRepo, labelClaYes, labelClaYes, "")
	if err != nil {
		logrus.Fatalf("Error adding label: %v", err)
	}
	err = f.AddRepoLabel(testOwner, testRepo, labelClaNo, labelClaNo, "")
	if err != nil {
		logrus.Fatalf("Error adding label: %v", err)
	}

	shaPlusPRLabels := map[string]*github.Label{shaWithPR: nil, shaWithPRClaStatusPending: nil, shaWithPRAndYesLabel: &prLabelYes, shaWithPRAndNoLabel: &prLabelNo}
	for s, l := range shaPlusPRLabels {
		i, _ := f.CreatePullRequest(testOwner, testRepo, "Without label", "Body", "HEAD", "BASE", true)
		f.PullRequests[i].Head = github.PullRequestBranch{SHA: s}
		f.CommitMap[createCommitMapKey(testOwner, testRepo, i)] = append(f.CommitMap[createCommitMapKey(testOwner, testRepo, i)], github.RepositoryCommit{SHA: s})
		if l != nil {
			f.PullRequests[i].Labels = append(f.PullRequests[i].Labels, *l)
		}
	}
}
