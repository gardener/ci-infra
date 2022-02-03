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
	"fmt"
	"net/http"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/github"
)

// httpServer implements http.Handler. It validates incoming GitHub webhooks and
// then dispatches them to the appropriate plugins.
type httpServer struct {
	tokenGenerator func() []byte
	cla            *claAssistantPlugin
	log            *logrus.Entry
}

func newHttpServer(tokenGenerator func() []byte, cla *claAssistantPlugin, log *logrus.Entry) httpServer {
	return httpServer{tokenGenerator: tokenGenerator, cla: cla, log: log}
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
		go func() {
			if err := s.cla.handleIssueCommentEvent(l, &ice); err != nil {
				l.WithField("event-type", eventType).WithError(err).Info("Error handling event.")
			}
		}()
	case "pull_request_review_comment":
		var rce github.ReviewCommentEvent
		if err := json.Unmarshal(payload, &rce); err != nil {
			return err
		}
		go func() {
			if err := s.cla.handleReviewCommentEvent(l, &rce); err != nil {
				l.WithField("event-type", eventType).WithError(err).Info("Error handling event.")
			}
		}()
	case "status":
		var se github.StatusEvent
		if err := json.Unmarshal(payload, &se); err != nil {
			return err
		}
		go func() {
			if err := s.cla.handleStatusEvent(l, &se); err != nil {
				l.WithField("event-type", eventType).WithError(err).Info("Error handling event.")
			}
		}()
	default:
		s.log.Debugf("received an event of type %q but didn't ask for it", eventType)
	}
	return nil
}
