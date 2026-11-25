// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/prow/pkg/config"
	"sigs.k8s.io/prow/pkg/github"
	"sigs.k8s.io/prow/pkg/labels"
	"sigs.k8s.io/prow/pkg/pluginhelp"
	"sigs.k8s.io/prow/pkg/plugins"
	"sigs.k8s.io/prow/pkg/plugins/approve/approvers"
)

const pluginName = "protect-restricted-prow-labels"

// warningMarkerPrefix starts the invisible HTML comment appended to every warning this plugin
// posts, so it can find and replace its own previous warnings. The marker includes the label (see
// warningMarker), keeping at most one warning per label on a pull request.
const warningMarkerPrefix = "<!-- protect-restricted-prow-labels"

// warningMarker returns the marker identifying a warning about the given label.
func warningMarker(label string) string {
	return fmt.Sprintf("%s: %s -->", warningMarkerPrefix, strings.ToLower(label))
}

// manualApprovalText only appears in the approve plugin's notification when the `approved` label
// was added manually. Keep it in sync with the approve plugin's notification template:
// https://github.com/kubernetes-sigs/prow/blob/main/pkg/plugins/approve/approvers/owners.go
var (
	approvalNotifierPrefix = "[" + strings.ToUpper(approvers.ApprovalNotificationName) + "]"
	manualApprovalText     = "Approval requirements bypassed by manually added approval."
)

type githubClient interface {
	CreateComment(org, repo string, number int, comment string) error
	DeleteComment(org, repo string, id int) error
	FindIssuesWithOrg(org, query, sort string, asc bool) ([]github.Issue, error)
	ListIssueComments(org, repo string, number int) ([]github.IssueComment, error)
	ListIssueEvents(org, repo string, number int) ([]github.ListedIssueEvent, error)
	RemoveLabel(org, repo string, number int, label string) error
	TeamBySlugHasMember(org, teamSlug, memberLogin string) (bool, error)
}

// HelpProvider constructs the pluginhelp.PluginHelp for this plugin.
func HelpProvider(_ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	pluginHelp := &pluginhelp.PluginHelp{
		Description: fmt.Sprintf(`The %s plugin enforces that the %q and %q labels, as well as any label configured under `+
			"`label.restricted_labels`"+`, can only be added by the Prow user (the GitHub App this plugin runs as). `+
			`For restricted labels, the users and teams listed in `+"`allowed_users`/`allowed_teams`"+` are allowed too. `+
			`When anyone else adds such a label, the plugin removes it again and posts a comment (at most one per label) explaining that `+
			`manually setting these labels is not allowed in this GitHub organization. For the %q label it additionally `+
			`removes the manual-approval notification posted by the approve plugin.`,
			pluginName, labels.Approved, labels.LGTM, labels.Approved),
	}
	return pluginHelp, nil
}

// Server implements http.Handler. It validates incoming GitHub webhooks and then enforces
// that protected labels are only set by allowed users.
type Server struct {
	tokenGenerator func() []byte
	ghc            githubClient
	log            *logrus.Entry

	// configAgent provides live access to the plugins configuration (restricted_labels).
	configAgent *plugins.ConfigAgent
	// botUserChecker reports whether a login is the Prow user this plugin runs as. It already
	// handles the "[bot]" suffix that GitHub Apps carry.
	botUserChecker func(candidate string) bool

	// lastReconcile is the start time of the last successful reconciliation. It is only accessed
	// by the single reconciliation goroutine. The next pass only fetches events for pull requests
	// updated since then.
	lastReconcile time.Time
	// lastConfigFingerprint is the plugin-relevant config seen during the last reconciliation.
	// When it changes, the next pass resets lastReconcile and reconciles every pull request.
	lastConfigFingerprint string
}

// ServeHTTP validates an incoming webhook and handles it.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	eventType, eventGUID, payload, ok, _ := github.ValidateWebhook(w, r, s.tokenGenerator)
	if !ok {
		return
	}
	fmt.Fprint(w, "Event received. Have a nice day.")

	if err := s.handleEvent(eventType, eventGUID, payload); err != nil {
		s.log.WithError(err).Error("Error parsing event.")
	}
}

func (s *Server) handleEvent(eventType, eventGUID string, payload []byte) error {
	l := s.log.WithFields(logrus.Fields{
		"event-type":     eventType,
		github.EventGUID: eventGUID,
	})
	switch eventType {
	case "pull_request":
		var pre github.PullRequestEvent
		if err := json.Unmarshal(payload, &pre); err != nil {
			return err
		}
		go func() {
			if err := s.handlePullRequest(l, pre); err != nil {
				l.WithError(err).Info("Error handling pull_request event.")
			}
		}()
	default:
		l.Debugf("received an event of type %q but didn't ask for it", eventType)
	}
	return nil
}

// handlePullRequest removes protected labels that were added by a user who is not allowed to
// set them.
func (s *Server) handlePullRequest(l *logrus.Entry, pre github.PullRequestEvent) error {
	if pre.Action != github.PullRequestActionLabeled {
		return nil
	}

	var (
		org    = pre.Repo.Owner.Login
		repo   = pre.Repo.Name
		number = pre.Number
	)
	log := l.WithFields(logrus.Fields{
		github.OrgLogField:  org,
		github.RepoLogField: repo,
		github.PrLogField:   number,
		"label":             pre.Label.Name,
		"user":              pre.Sender.Login,
	})

	return s.enforceLabel(log, org, repo, number, pre.Label.Name, pre.Sender.Login)
}

// enforceLabel removes labelName if it is protected and actor is not allowed to set it, also
// removing the approve plugin's manual-approval notification (for `approved`) and posting a
// single warning. It is a no-op if the label is unprotected or the actor is allowed, and is
// idempotent so the webhook handler and the reconciliation can both call it.
func (s *Server) enforceLabel(log *logrus.Entry, org, repo string, number int, labelName, actor string) error {
	label := strings.ToLower(labelName)

	// The Prow user may set any label.
	if s.botUserChecker(actor) {
		return nil
	}

	protected, allowed, err := s.isAllowed(org, repo, label, actor)
	if err != nil {
		return fmt.Errorf("failed to determine whether %q may set label %q: %w", actor, label, err)
	}
	if !protected || allowed {
		return nil
	}

	log.Infof("Removing label %q that was added to %s/%s#%d by %q who is not allowed to set it.", labelName, org, repo, number, actor)
	if err := s.ghc.RemoveLabel(org, repo, number, labelName); err != nil {
		return fmt.Errorf("failed to remove label %q: %w", labelName, err)
	}

	// The approve plugin reacts to a manually added `approved` label by posting a
	// notification that approval requirements were bypassed. Remove it if present.
	if label == labels.Approved {
		if err := s.removeManualApprovalMessage(log, org, repo, number); err != nil {
			return err
		}
	}

	return s.postWarning(log, org, repo, number, labelName, actor)
}

// isAllowed reports whether a label is protected and, if so, whether the given user is allowed
// to set it. The `approved` and `lgtm` labels are always protected and may only be set by the
// Prow user (already excluded by the caller). All other protected labels come from
// `label.restricted_labels`; for those, the configured allowed_users and allowed_teams apply.
func (s *Server) isAllowed(org, repo, label, user string) (protected, allowed bool, err error) {
	if label == labels.Approved || label == labels.LGTM {
		return true, false, nil
	}

	restrictedLabels := s.configAgent.Config().Label.RestrictedLabelsFor(org, repo)
	restricted, isRestricted := restrictedLabels[label]
	if !isRestricted {
		return false, false, nil
	}

	for _, allowedUser := range restricted.AllowedUsers {
		if strings.EqualFold(allowedUser, user) {
			return true, true, nil
		}
	}
	for _, team := range restricted.AllowedTeams {
		isMember, err := s.ghc.TeamBySlugHasMember(org, team, user)
		if err != nil {
			return true, false, err
		}
		if isMember {
			return true, true, nil
		}
	}
	return true, false, nil
}

// removeManualApprovalMessage deletes the approve plugin's manual-approval notification
// comment, if present. This is best-effort: the approve plugin reacts to the same label event
// as this plugin, so the comment may not exist yet when this runs.
func (s *Server) removeManualApprovalMessage(log *logrus.Entry, org, repo string, number int) error {
	comments, err := s.ghc.ListIssueComments(org, repo, number)
	if err != nil {
		return fmt.Errorf("failed to list comments: %w", err)
	}
	for _, comment := range comments {
		if !s.botUserChecker(comment.User.Login) {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(comment.Body), approvalNotifierPrefix) && strings.Contains(comment.Body, manualApprovalText) {
			if err := s.ghc.DeleteComment(org, repo, comment.ID); err != nil {
				return fmt.Errorf("failed to delete manual approval notification %d: %w", comment.ID, err)
			}
			log.Info("Deleted manual approval notification.")
		}
	}
	return nil
}

// postWarning removes any previous warning this plugin posted for the same label and posts a
// fresh one, keeping at most one warning per label on the pull request.
func (s *Server) postWarning(log *logrus.Entry, org, repo string, number int, label, user string) error {
	marker := warningMarker(label)
	comments, err := s.ghc.ListIssueComments(org, repo, number)
	if err != nil {
		return fmt.Errorf("failed to list comments: %w", err)
	}
	for _, comment := range comments {
		if s.botUserChecker(comment.User.Login) && strings.Contains(comment.Body, marker) {
			if err := s.ghc.DeleteComment(org, repo, comment.ID); err != nil {
				return fmt.Errorf("failed to delete previous warning %d: %w", comment.ID, err)
			}
		}
	}

	message := fmt.Sprintf(
		"@%s the `%s` label cannot be added manually in the `%s` GitHub organization. It has been removed again.\n\n"+
			"Please use the `%s` Prow command.\n%s",
		user, label, org, prowCommandFor(label), marker)
	if err := s.ghc.CreateComment(org, repo, number, message); err != nil {
		return fmt.Errorf("failed to create warning comment: %w", err)
	}
	log.Info("Posted warning comment for manually added protected label.")
	return nil
}

// prowCommandFor returns the Prow command a user should use to request the given label instead
// of adding it manually.
func prowCommandFor(label string) string {
	switch strings.ToLower(label) {
	case labels.Approved:
		return "/approve"
	case labels.LGTM:
		return "/lgtm"
	default:
		return "/label " + label
	}
}
