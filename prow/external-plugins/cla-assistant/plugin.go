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
	"regexp"
	"time"

	backoff "github.com/cenkalti/backoff/v4"
	githubql "github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
)

const (
	pluginName       string = "cla-assistant"
	claGithubContext string = "license/cla"

	labelClaYes string = "cla: yes"
	labelClaNo  string = "cla: no"

	claAssistantBaseURL string = "https://cla-assistant.io"
	claAssistantURLPath string = "/check/%v/%v?pullRequest=%v"
)

var (
	checkCLARe = regexp.MustCompile(`(?mi)^/cla\s*$`)
)

type githubClient interface {
	AddLabel(org, repo string, number int, label string) error
	RemoveLabel(org, repo string, number int, label string) error
	CreateComment(org, repo string, number int, comment string) error
	ListStatuses(org, repo, ref string) ([]github.Status, error)
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	QueryWithGitHubAppsSupport(ctx context.Context, q interface{}, vars map[string]interface{}, org string) error
}

// claAssistantPlugin
type claAssistantPlugin struct {
	ghc          githubClient
	hc           *http.Client
	log          *logrus.Entry
	baseURL      string
	maxRetryTime time.Duration
}

func newClaAssistantPlugin(ghc githubClient, log *logrus.Entry) claAssistantPlugin {
	hc := &http.Client{Timeout: time.Second * 15}
	return claAssistantPlugin{ghc: ghc, hc: hc, log: log, baseURL: claAssistantBaseURL, maxRetryTime: time.Minute * 1}
}

func (c *claAssistantPlugin) handleIssueCommentEvent(l *logrus.Entry, ice *github.IssueCommentEvent) error {

	l.Debugf(
		"Comment for issue %v of org/repo %s/%s in state %v with action %v received - /cla in message %v",
		ice.Issue.Number,
		ice.Repo.Owner.Login,
		ice.Repo.Name,
		ice.Issue.State,
		ice.Action,
		checkCLARe.MatchString(ice.Comment.Body),
	)

	if !ice.Issue.IsPullRequest() {
		return nil
	}

	// Only consider open PRs and new comments.
	if ice.Issue.State != github.PullRequestStateOpen || ice.Action != github.IssueCommentActionCreated {
		return nil
	}

	// Only consider "/cla" comments.
	if !checkCLARe.MatchString(ice.Comment.Body) {
		return nil
	}

	l.Infof("Calling cla-assistant.io to initialize recheck of PR %v.", ice.Issue.Number)

	return c.enforceClaRecheck(ice.Repo.Owner.Login, ice.Repo.Name, ice.Issue.Number)
}

func (c *claAssistantPlugin) handleReviewCommentEvent(l *logrus.Entry, rce *github.ReviewCommentEvent) error {

	l.Debugf(
		"Comment for PR %v of org/repo %s/%s in state %v with action %v received - /cla in message %v",
		rce.PullRequest.Number,
		rce.Repo.Owner.Login,
		rce.Repo.Name,
		rce.PullRequest.State,
		rce.Action,
		checkCLARe.MatchString(rce.Comment.Body),
	)

	// Only consider open PRs and new comments.
	if rce.PullRequest.State != github.PullRequestStateOpen || rce.Action != github.ReviewCommentActionCreated {
		return nil
	}

	// Only consider "/cla" comments.
	if !checkCLARe.MatchString(rce.Comment.Body) {
		return nil
	}

	l.Infof("Calling cla-assistant.io to initialize recheck of PR %v.", rce.PullRequest.Number)

	return c.enforceClaRecheck(rce.Repo.Owner.Login, rce.Repo.Name, rce.PullRequest.Number)
}

func (c *claAssistantPlugin) handleStatusEvent(l *logrus.Entry, se *github.StatusEvent) error {

	org := se.Repo.Owner.Login
	repo := se.Repo.Name

	l.Debugf("Status for org/repo %s/%s in state %v for context %v received", org, repo, se.State, se.Context)

	if se.State == "" || se.Context == "" {
		return fmt.Errorf("invalid status event delivered with empty state/context")
	}

	// Status for context license/cla are arriving quite unreliably.
	// Thus, extract it from status list of the commit when status events with different contextes arrive.
	var claStatus github.Status
	if se.Context == claGithubContext {
		claStatus.Context = se.Context
		claStatus.Description = se.Description
		claStatus.State = se.State
		claStatus.TargetURL = se.TargetURL
	} else {
		b := backoff.NewExponentialBackOff()
		b.MaxElapsedTime = c.maxRetryTime
		err := backoff.Retry(
			func() error {
				status, err := c.ghc.ListStatuses(org, repo, se.SHA)
				if err != nil {
					return err
				}
				for _, s := range status {
					if s.Context == claGithubContext {
						claStatus = s
						break
					}
				}
				return nil
			},
			b,
		)
		if err != nil {
			return err
		}
	}

	if claStatus.Context != claGithubContext {
		// No CLA status, nothing to do
		return nil
	}

	if claStatus.State == github.StatusPending {
		// do nothing and wait for state to be updated.
		return nil
	}

	l.Info("Searching for PRs matching the commit.")

	var pullRequests []pullRequest
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = c.maxRetryTime

	err := backoff.Retry(
		func() error {
			var err error
			pullRequests, err = c.search(context.Background(), l, fmt.Sprintf("%s repo:%s/%s type:pr state:open", se.SHA, org, repo), org)
			if err != nil {
				return fmt.Errorf("error searching for issues matching commit: %w", err)
			}
			return nil
		},
		b,
	)

	if err != nil {
		return err
	}

	l.Infof("Found %d PRs matching commit.", len(pullRequests))

	for _, pullRequest := range pullRequests {
		pl := l.WithField("pr", pullRequest.Number)
		hasClaYes := pullRequest.hasLabel(labelClaYes)
		hasClaNo := pullRequest.hasLabel(labelClaNo)
		if hasClaYes && claStatus.State == github.StatusSuccess {
			// Nothing to update.
			pl.Infof("PR #%v has up-to-date %q label.", int(pullRequest.Number), labelClaYes)
			continue
		}

		if hasClaNo && (claStatus.State == github.StatusFailure || claStatus.State == github.StatusError) {
			// Nothing to update.
			pl.Infof("PR #%v has up-to-date %q label.", int(pullRequest.Number), labelClaNo)
			continue
		}

		pl.Info("PR labels may be out of date. Getting pull request info.")

		var pr *github.PullRequest

		b.Reset()
		err := backoff.Retry(
			func() error {
				pr, err = c.ghc.GetPullRequest(org, repo, int(pullRequest.Number))
				return err
			},
			b,
		)

		if err != nil {
			pl.WithError(err).Warningf("Unable to fetch PR #%d from %s/%s.", int(pullRequest.Number), org, repo)
			continue
		}

		// Check if this is the latest commit in the PR.
		if pr.Head.SHA != se.SHA {
			pl.Info("Event is not for PR HEAD, skipping.")
			continue
		}

		number := pr.Number
		if claStatus.State == github.StatusSuccess {
			if hasClaNo {
				// Remove "CLA no" label
				b.Reset()
				err = backoff.Retry(
					func() error { return c.ghc.RemoveLabel(org, repo, number, labelClaNo) },
					b,
				)
				if err != nil {
					pl.WithError(err).Warningf("Could not remove %s label from PR #%v.", labelClaNo, int(pullRequest.Number))
				}
			}
			// Add "CLA yes" label
			b.Reset()
			err = backoff.Retry(
				func() error { return c.ghc.AddLabel(org, repo, number, labelClaYes) },
				b,
			)
			if err != nil {
				pl.WithError(err).Warningf("Could not add %s label from PR #%v.", labelClaYes, int(pullRequest.Number))
			}
			continue
		}

		// If we end up here, the github status is a failure/error, so a potential CLA yes label needs to be removed.
		if hasClaYes {
			// Remove "CLA yes" label
			b.Reset()
			err = backoff.Retry(
				func() error { return c.ghc.RemoveLabel(org, repo, number, labelClaYes) },
				b,
			)
			if err != nil {
				pl.WithError(err).Warningf("Could not remove %s label from PR #%v.", labelClaYes, int(pullRequest.Number))
			}
		}
		// Add "CLA no" label
		b.Reset()
		err = backoff.Retry(
			func() error { return c.ghc.AddLabel(org, repo, number, labelClaNo) },
			b,
		)
		if err != nil {
			pl.WithError(err).Warningf("Could not add %s label from PR #%v.", labelClaNo, int(pullRequest.Number))
		}

	}
	return nil

}

func (c *claAssistantPlugin) helpProvider([]config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	var ph pluginhelp.PluginHelp

	ph.Description = `CLA assitant plugin attaches CLA labels to PRs according to results from cla-assistant.io. \n
						Additionally it can force rechecking the CLA status by /cla command.`

	ph.AddCommand(pluginhelp.Command{
		Usage:       "/cla",
		Description: "Forces a recheck of CLA status",
		WhoCanUse:   "Anyone",
		Examples:    []string{"/cla"},
		Featured:    true,
	})

	return &ph, nil
}

func (c *claAssistantPlugin) createClaAssistantURI(org, repo string, pullRequestNumber int) string {
	path := fmt.Sprintf(claAssistantURLPath, org, repo, pullRequestNumber)
	return c.baseURL + path
}

func (c *claAssistantPlugin) enforceClaRecheck(org string, repo string, pullRequestNumber int) error {
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = c.maxRetryTime

	err := backoff.Retry(
		func() error {
			resp, err := c.hc.Get(c.createClaAssistantURI(org, repo, pullRequestNumber))
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
				return nil
			}
			return fmt.Errorf("error reaching out to cla-assistant.io for rechecking PR %v - HTTP status code %v", pullRequestNumber, resp.StatusCode)
		},
		b,
	)

	if err == nil {
		c.log.Infof("Successfully reached out to cla-assistant.io to initialize recheck of PR %v", pullRequestNumber)
		c.createClaResultComment(
			org,
			repo,
			pullRequestNumber,
			fmt.Sprintf("Successfully reached out to cla-assistant.io to initialize recheck of PR #%v", pullRequestNumber),
		)
	} else {
		c.createClaResultComment(
			org,
			repo,
			pullRequestNumber,
			fmt.Sprintf("Could not reach out to cla-assistant.io for rechecking PR #%v", pullRequestNumber),
		)
	}

	return err
}

func (c *claAssistantPlugin) createClaResultComment(org string, repo string, pullRequestNumber int, comment string) error {
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = c.maxRetryTime

	err := backoff.Retry(
		func() error { return c.ghc.CreateComment(org, repo, pullRequestNumber, comment) },
		b,
	)
	return err
}

func (c *claAssistantPlugin) search(ctx context.Context, log *logrus.Entry, q, org string) ([]pullRequest, error) {
	var ret []pullRequest
	vars := map[string]interface{}{
		"query":        githubql.String(q),
		"searchCursor": (*githubql.String)(nil),
	}
	var totalCost int
	var remaining int
	requestStart := time.Now()
	var pageCount int
	for {
		pageCount++
		sq := searchQuery{}
		if err := c.ghc.QueryWithGitHubAppsSupport(ctx, &sq, vars, org); err != nil {
			return nil, err
		}
		totalCost += int(sq.RateLimit.Cost)
		remaining = int(sq.RateLimit.Remaining)
		for _, n := range sq.Search.Nodes {
			ret = append(ret, n.PullRequest)
		}
		if !sq.Search.PageInfo.HasNextPage {
			break
		}
		vars["searchCursor"] = githubql.NewString(sq.Search.PageInfo.EndCursor)
	}
	log = log.WithFields(logrus.Fields{
		"query":          q,
		"duration":       time.Since(requestStart).String(),
		"pr_found_count": len(ret),
		"search_pages":   pageCount,
		"cost":           totalCost,
		"remaining":      remaining,
	})
	log.Debug("Finished query")

	return ret, nil
}

// See: https://developer.github.com/v4/object/pullrequest/.
type pullRequest struct {
	Number githubql.Int
	Author struct {
		Login githubql.String
	}
	Repository struct {
		Name  githubql.String
		Owner struct {
			Login githubql.String
		}
	}
	Labels struct {
		Nodes []struct {
			Name githubql.String
		}
	} `graphql:"labels(first:100)"`
	Mergeable githubql.MergeableState
	State     githubql.PullRequestState
}

func (p *pullRequest) hasLabel(label string) bool {
	for _, l := range p.Labels.Nodes {
		if string(l.Name) == label {
			return true
		}
	}
	return false
}

// See: https://developer.github.com/v4/query/.
type searchQuery struct {
	RateLimit struct {
		Cost      githubql.Int
		Remaining githubql.Int
	}
	Search struct {
		PageInfo struct {
			HasNextPage githubql.Boolean
			EndCursor   githubql.String
		}
		Nodes []struct {
			PullRequest pullRequest `graphql:"... on PullRequest"`
		}
	} `graphql:"search(type: ISSUE, first: 100, after: $searchCursor, query: $query)"`
}
