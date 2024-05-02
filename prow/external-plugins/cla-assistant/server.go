// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/prow/pkg/github"
)

// httpServer implements http.Handler. It validates incoming GitHub webhooks and
// then dispatches them to the appropriate plugins.
type httpServer struct {
	tokenGenerator func() []byte
	cla            *claAssistantPlugin
	log            *logrus.Entry
}

func newHttpServer(tokenGenerator func() []byte, cla *claAssistantPlugin, log *logrus.Entry) *httpServer {
	return &httpServer{tokenGenerator: tokenGenerator, cla: cla, log: log}
}

// ServeHTTP validates an incoming webhook and puts it into the event channel.
func (s *httpServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	eventType, eventGUID, payload, ok, _ := github.ValidateWebhook(w, r, s.tokenGenerator)
	if !ok {
		return
	}
	fmt.Fprint(w, "Event received. Have a nice day.")
	logrus.Debug("Event received. Have a nice day.")

	if err := s.handleEvent(eventType, eventGUID, payload); err != nil {
		logrus.WithError(err).Error("Error parsing event.")
	}
}

func (s *httpServer) handleEvent(eventType, eventGUID string, payload []byte) error {
	l := s.log.WithFields(
		logrus.Fields{
			"event-type":     eventType,
			github.EventGUID: eventGUID,
		},
	)
	l.Debugf("New event of type: %s", eventType)
	switch eventType {
	case "issue_comment":
		var ice github.IssueCommentEvent
		if err := json.Unmarshal(payload, &ice); err != nil {
			return err
		}
		l = l.WithFields(
			logrus.Fields{
				"org":  ice.Repo.Owner.Login,
				"repo": ice.Repo.Name,
			},
		)
		go func() {
			if err := s.cla.handleIssueCommentEvent(l, &ice); err != nil {
				l.WithError(err).Info("Error handling event.")
			}
		}()
	case "pull_request_review_comment":
		var rce github.ReviewCommentEvent
		if err := json.Unmarshal(payload, &rce); err != nil {
			return err
		}
		l = l.WithFields(
			logrus.Fields{
				"org":  rce.Repo.Owner.Login,
				"repo": rce.Repo.Name,
			},
		)
		go func() {
			if err := s.cla.handleReviewCommentEvent(l, &rce); err != nil {
				l.WithError(err).Info("Error handling event.")
			}
		}()
	case "pull_request_review":
		var pre github.ReviewEvent
		if err := json.Unmarshal(payload, &pre); err != nil {
			return err
		}
		l = l.WithFields(
			logrus.Fields{
				"org":  pre.Repo.Owner.Login,
				"repo": pre.Repo.Name,
			},
		)
		go func() {
			if err := s.cla.handleReviewEvent(l, &pre); err != nil {
				l.WithError(err).Info("Error handling event.")
			}
		}()
	case "status":
		var se github.StatusEvent
		if err := json.Unmarshal(payload, &se); err != nil {
			return err
		}
		l = l.WithFields(
			logrus.Fields{
				"org":  se.Repo.Owner.Login,
				"repo": se.Repo.Name,
			},
		)
		go func() {
			if err := s.cla.handleStatusEvent(l, &se); err != nil {
				l.WithError(err).Info("Error handling event.")
			}
		}()
	default:
		s.log.Debugf("received an event of type %q but didn't ask for it", eventType)
	}
	return nil
}
