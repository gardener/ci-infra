// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

const leaderElectionLockName = "protect-restricted-prow-labels"

// reconcileWithLeaderElection runs the periodic reconciliation on a single replica at a time.
// All replicas compete for a Lease; only the holder reconciles. This prevents every replica of
// an HA deployment from reconciling in parallel, which would multiply the GitHub API load and
// could race when posting warning comments. It blocks until ctx is cancelled.
func (s *Server) reconcileWithLeaderElection(ctx context.Context, kubeClient kubernetes.Interface, namespace, identity string, period time.Duration) {
	lock, err := resourcelock.New(
		resourcelock.LeasesResourceLock,
		namespace,
		leaderElectionLockName,
		kubeClient.CoreV1(),
		kubeClient.CoordinationV1(),
		resourcelock.ResourceLockConfig{Identity: identity},
	)
	if err != nil {
		s.log.WithError(err).Fatal("Failed to create leader election lock.")
	}

	config := leaderelection.LeaderElectionConfig{
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   15 * time.Second,
		RenewDeadline:   10 * time.Second,
		RetryPeriod:     2 * time.Second,
		Name:            pluginName,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				s.log.Info("Acquired leader lease, starting periodic reconciliation.")
				s.runReconcileLoop(ctx, period)
			},
			OnStoppedLeading: func() {
				s.log.Info("Lost leader lease, stopping periodic reconciliation.")
			},
			OnNewLeader: func(leader string) {
				if leader != identity {
					s.log.WithField("leader", leader).Debug("Another replica is the reconciliation leader.")
				}
			},
		},
	}

	// RunOrDie blocks while this replica is a candidate or the leader. It returns when this
	// replica loses leadership; re-enter so it competes for the lease again until shutdown.
	for ctx.Err() == nil {
		leaderelection.RunOrDie(ctx, config)
	}
}

// runReconcileLoop reconciles immediately and then once per period until ctx is cancelled.
func (s *Server) runReconcileLoop(ctx context.Context, period time.Duration) {
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	for {
		start := time.Now()
		if err := s.reconcile(); err != nil {
			s.log.WithError(err).Error("Error during periodic reconciliation.")
		}
		s.log.WithField("duration", fmt.Sprintf("%v", time.Since(start))).Info("Periodic reconciliation complete.")

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
