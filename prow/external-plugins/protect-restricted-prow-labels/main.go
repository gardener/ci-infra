// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/prow/pkg/config/secret"
	prowflagutil "sigs.k8s.io/prow/pkg/flagutil"
	pluginsflagutil "sigs.k8s.io/prow/pkg/flagutil/plugins"
	"sigs.k8s.io/prow/pkg/interrupts"
	"sigs.k8s.io/prow/pkg/logrusutil"
	"sigs.k8s.io/prow/pkg/pjutil"
	"sigs.k8s.io/prow/pkg/pluginhelp/externalplugins"
)

type options struct {
	port int

	dryRun                 bool
	github                 prowflagutil.GitHubOptions
	kubernetes             prowflagutil.KubernetesOptions
	pluginsConfig          pluginsflagutil.PluginOptions
	instrumentationOptions prowflagutil.InstrumentationOptions
	logLevel               string

	updatePeriod time.Duration

	webhookSecretFile string
}

func (o *options) Validate() error {
	for idx, group := range []prowflagutil.OptionGroup{&o.github, &o.kubernetes} {
		if err := group.Validate(o.dryRun); err != nil {
			return fmt.Errorf("%d: %w", idx, err)
		}
	}

	if !o.dryRun && o.github.TokenPath == "" && o.github.AppID == "" {
		return fmt.Errorf("please provide either --github-token-path or --github-app-id together with --github-app-private-key-path")
	}

	return nil
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	fs.IntVar(&o.port, "port", 8888, "Port to listen on.")
	fs.BoolVar(&o.dryRun, "dry-run", true, "Dry run for testing. Uses API tokens but does not mutate.")
	fs.StringVar(&o.webhookSecretFile, "hmac-secret-file", "/etc/webhook/hmac", "Path to the file containing the GitHub HMAC secret.")
	fs.StringVar(&o.logLevel, "log-level", "debug", fmt.Sprintf("Log level is one of %v.", logrus.AllLevels))
	fs.DurationVar(&o.updatePeriod, "update-period", time.Minute, "Period between periodic reconciliations of all open pull requests. Set to 0 to disable.")

	o.pluginsConfig.PluginConfigPathDefault = "/etc/plugins/plugins.yaml"
	for _, group := range []prowflagutil.OptionGroup{&o.github, &o.kubernetes, &o.pluginsConfig, &o.instrumentationOptions} {
		group.AddFlags(fs)
	}
	if err := fs.Parse(os.Args[1:]); err != nil {
		logrus.Fatalf("Unable to parse command line flags: %v", err)
	}
	return o
}

func main() {
	logrusutil.Init(&logrusutil.DefaultFieldsFormatter{
		PrintLineNumber:  true,
		WrappedFormatter: logrus.StandardLogger().Formatter,
	})
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	logLevel, err := logrus.ParseLevel(o.logLevel)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to parse loglevel")
	}
	logrus.SetLevel(logLevel)
	log := logrus.StandardLogger().WithField("plugin", pluginName)

	if err := secret.Add(o.webhookSecretFile); err != nil {
		logrus.WithError(err).Fatal("Error starting secrets agent.")
	}

	pluginAgent, err := o.pluginsConfig.PluginAgent()
	if err != nil {
		log.WithError(err).Fatal("Error loading plugin config.")
	}

	githubClient, err := o.github.GitHubClient(o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting GitHub client.")
	}

	botUserChecker, err := githubClient.BotUserChecker()
	if err != nil {
		logrus.WithError(err).Fatal("Error getting bot user checker.")
	}

	server := &Server{
		tokenGenerator: secret.GetTokenGenerator(o.webhookSecretFile),
		ghc:            githubClient,
		log:            log,
		configAgent:    pluginAgent,
		botUserChecker: botUserChecker,
	}

	health := pjutil.NewHealthOnPort(o.instrumentationOptions.HealthPort)
	health.ServeReady()

	defer interrupts.WaitForGracefulShutdown()

	// Periodically reconcile all open pull requests as a safety net for webhook events that were
	// missed, e.g. due to a transient GitHub error or a restart while handling an event.
	if o.updatePeriod > 0 {
		if o.dryRun {
			// There is no Kubernetes client in dry-run mode, so skip leader election and run the
			// reconciliation loop directly.
			log.Info("Dry-run mode: running periodic reconciliation without leader election.")
			interrupts.Run(func(ctx context.Context) {
				server.runReconcileLoop(ctx, o.updatePeriod)
			})
		} else {
			kubeClient, err := o.kubernetes.InfrastructureClusterClient(o.dryRun)
			if err != nil {
				log.WithError(err).Fatal("Error getting Kubernetes client for leader election.")
			}
			identity := leaderElectionIdentity()
			log.WithField("identity", identity).Info("Enabling leader-elected periodic reconciliation.")
			interrupts.Run(func(ctx context.Context) {
				server.reconcileWithLeaderElection(ctx, kubeClient, leaderElectionNamespace(), identity, o.updatePeriod)
			})
		}
	}

	mux := http.NewServeMux()
	mux.Handle("/", server)
	externalplugins.ServeExternalPluginHelp(mux, log, HelpProvider)
	httpServer := &http.Server{Addr: ":" + strconv.Itoa(o.port), Handler: mux}
	interrupts.ListenAndServe(httpServer, 5*time.Second)
}

// leaderElectionIdentity returns a unique identity for this replica in the leader election. It
// uses the pod name provided via the downward API and falls back to the hostname.
func leaderElectionIdentity() string {
	if name := os.Getenv("POD_NAME"); name != "" {
		return name
	}
	hostname, err := os.Hostname()
	if err != nil {
		logrus.WithError(err).Fatal("Error getting hostname for leader election identity.")
	}
	return hostname
}

// leaderElectionNamespace returns the namespace in which the leader election lease is created. It
// is provided via the downward API.
func leaderElectionNamespace() string {
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		logrus.Fatal("POD_NAMESPACE environment variable must be set for leader election.")
	}
	return namespace
}
