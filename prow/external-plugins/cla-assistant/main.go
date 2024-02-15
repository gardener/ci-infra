// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/pkg/flagutil"
	"k8s.io/test-infra/prow/config/secret"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	pluginsflagutil "k8s.io/test-infra/prow/flagutil/plugins"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pluginhelp/externalplugins"
)

type options struct {
	port int

	pluginsConfig          pluginsflagutil.PluginOptions
	dryRun                 bool
	github                 prowflagutil.GitHubOptions
	instrumentationOptions prowflagutil.InstrumentationOptions
	logLevel               string

	updatePeriod time.Duration

	webhookSecretFile string
}

func (o *options) Validate() error {
	for _, group := range []flagutil.OptionGroup{&o.github} {
		if err := group.Validate(o.dryRun); err != nil {
			return err
		}
	}

	return nil
}

func gatherOptions() options {
	o := options{pluginsConfig: pluginsflagutil.PluginOptions{PluginConfigPathDefault: "/etc/plugins/plugins.yaml"}}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.IntVar(&o.port, "port", 8080, "Port HTTP server listens on.")
	fs.BoolVar(&o.dryRun, "dry-run", true, "Dry run for testing. Uses API tokens but does not mutate.")
	fs.DurationVar(&o.updatePeriod, "update-period", time.Hour*1, "Period duration for periodic scans of all PRs.")
	fs.StringVar(&o.webhookSecretFile, "hmac-secret-file", "/etc/webhook/hmac", "Path to the file containing the GitHub HMAC secret.")
	fs.StringVar(&o.logLevel, "log-level", "debug", fmt.Sprintf("Log level is one of %v.", logrus.AllLevels))

	for _, group := range []flagutil.OptionGroup{&o.github, &o.instrumentationOptions, &o.pluginsConfig} {
		group.AddFlags(fs)
	}
	err := fs.Parse(os.Args[1:])
	if err != nil {
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
		log.WithError(err).Fatal("Error starting secrets agent.")
	}

	pa, err := o.pluginsConfig.PluginAgent()
	if err != nil {
		log.WithError(err).Fatal("Error loading plugin config")
	}

	githubClient, err := o.github.GitHubClient(o.dryRun)
	if err != nil {
		log.WithError(err).Fatal("Error getting GitHub client.")
	}

	cla := newClaAssistantPlugin(githubClient, log)

	hs := newHttpServer(secret.GetTokenGenerator(o.webhookSecretFile), cla, log)

	defer interrupts.WaitForGracefulShutdown()

	health := pjutil.NewHealthOnPort(o.instrumentationOptions.HealthPort)

	mux := http.NewServeMux()
	mux.Handle("/", hs)
	externalplugins.ServeExternalPluginHelp(mux, log, cla.helpProvider)
	httpServer := &http.Server{Addr: ":" + strconv.Itoa(o.port), Handler: mux}

	interrupts.TickLiteral(func() {
		start := time.Now()
		if err := cla.handleAllPRs(log, pa.Config()); err != nil {
			log.WithError(err).Error("Error during periodic check all PRs.")
		}
		log.WithField("duration", fmt.Sprintf("%v", time.Since(start))).Info("Periodic update complete.")
	}, o.updatePeriod)

	health.ServeReady()
	interrupts.ListenAndServe(httpServer, 5*time.Second)

}
