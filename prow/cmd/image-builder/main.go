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
	"flag"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/logrusutil"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type options struct {
	dockerConfigSecret string
	org                string
	repo               string
	pullBaseSHA        string
	dockerfile         string
	targets            flagutil.Strings
	registry           string
	cacheRegistry      string
	kanikoImage        string
	addVersionTag      bool
	addVersionSHATag   bool
	addDateSHATag      bool
	addLatestTag       bool
	addFixTag          string

	logLevel string
}

func (o *options) Validate() error {
	if o.dockerfile == "" {
		return fmt.Errorf("\"dockerfile\" parameter must not be empty")
	}
	if o.dockerConfigSecret == "" {
		return fmt.Errorf("\"docker-config-secret\" parameter must not be empty")
	}
	if len(o.targets.Strings()) == 0 {
		return fmt.Errorf("specify at least one \"target\"")
	}
	if o.registry == "" {
		return fmt.Errorf("\"registry\" parameter must not be empty")
	}
	if !o.addVersionTag && !o.addVersionSHATag && !o.addDateSHATag && !o.addLatestTag {
		return fmt.Errorf("please choose at least one tagging scheme")
	}
	return nil
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&o.dockerConfigSecret, "docker-config-secret", "", "secret which includes docker config.json file")
	fs.StringVar(&o.dockerfile, "dockerfile", "Dockerfile", "path to dockerfile to be built")
	fs.Var(&o.targets, "target", "target of dockerfile to be built")
	fs.StringVar(&o.registry, "registry", "", "container registry where build artifacts are beeing pushed")
	fs.StringVar(&o.cacheRegistry, "cache-registry", "", "container registry where cache artifacts are beeing pushed")
	fs.StringVar(&o.kanikoImage, "kaniko-image", "gcr.io/kaniko-project/executor:v1.7.0", "kaniko image for kaniko build")
	fs.BoolVar(&o.addVersionTag, "add-version-tag", false, "Add label from VERSION file of git root directory to image tags")
	fs.BoolVar(&o.addVersionSHATag, "add-version-sha-tag", false, "Add label from VERSION file of git root directory plus SHA from git HEAD to image tags")
	fs.BoolVar(&o.addDateSHATag, "add-date-sha-tag", false, "Using YYYYMMDD-<rev short> scheme which is compatible to autobumper")
	fs.BoolVar(&o.addLatestTag, "add-latest-tag", false, "Add 'latest' tag to images")
	fs.StringVar(&o.addFixTag, "add-fix-tag", "", "Add a fix tag to images")

	fs.StringVar(&o.logLevel, "log-level", "debug", fmt.Sprintf("Log level is one of %v.", logrus.AllLevels))

	err := fs.Parse(os.Args[1:])
	if err != nil {
		logrus.Fatalf("Unable to parse command line flags: %v", err)
	}
	return o
}

func main() {
	logrusutil.Init(&logrusutil.DefaultFieldsFormatter{
		PrintLineNumber:  true,
		WrappedFormatter: &logrus.TextFormatter{},
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
	log := logrus.StandardLogger()

	podName := os.Getenv("POD_NAME")
	if podName == "" {
		log.Fatal("Environment variable \"POD_NAME\" is not set")
	}

	podNamespace := os.Getenv("POD_NAMESPACE")
	if podNamespace == "" {
		log.Fatal("Environment variable \"POD_NAMESPACE\" is not set")
	}

	o.org = os.Getenv("REPO_OWNER")
	if o.org == "" {
		log.Fatal("Environment variable \"REPO_OWNER\" is not set")
	}

	o.repo = os.Getenv("REPO_NAME")
	if o.repo == "" {
		log.Fatal("Environment variable \"REPO_NAME\" is not set")
	}

	o.pullBaseSHA = os.Getenv("PULL_BASE_SHA")
	if o.pullBaseSHA == "" {
		log.Fatal("Environment variable \"PULL_BASE_SHA\" is not set")
	}

	if o.cacheRegistry == "" {
		log.Info("cache-registry parameter is not set. Building without using cache")
	}

	restConfig, err := rest.InClusterConfig()
	if err != nil {
		log.WithError(err).Fatal("Error getting kubernetes in cluster config")
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		log.WithError(err).Fatal("Error getting kubernetes clientset")
	}

	sc := runtime.NewScheme()
	err = corev1.AddToScheme(sc)
	if err != nil {
		log.WithError(err).Fatal("Unable to add corev1 to scheme")
	}
	mgr, err := manager.New(config.GetConfigOrDie(), manager.Options{Scheme: sc, Namespace: podNamespace})
	if err != nil {
		log.WithError(err).Fatal("Unable to create controller manager")
	}

	log.Info("Setting up build-controller")
	ctx, cancelFunc := context.WithCancel(context.Background())
	controller, err := addImageBuilderController(ctx, mgr, clientset, cancelFunc, types.NamespacedName{Name: podName, Namespace: podNamespace}, o, log.WithContext(ctx))
	if err != nil {
		log.WithError(err).Fatal("Unable to setup build-controller")
	}

	log.Info("Starting controller manager")
	err = mgr.Start(ctx)
	if err != nil {
		log.WithError(err).Fatal("Unable to start controller manager")
	}

	if controller.err != nil {
		log.WithError(err).Panic("Build failed")
	}
	log.Info("Build successfull")
}
