// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	prowjobv1 "sigs.k8s.io/prow/prow/apis/prowjobs/v1"
	"sigs.k8s.io/prow/prow/flagutil"
	"sigs.k8s.io/prow/prow/interrupts"
	"sigs.k8s.io/prow/prow/logrusutil"
	"sigs.k8s.io/prow/prow/pod-utils/downwardapi"
)

type options struct {
	dockerConfigSecret      string
	org                     string
	repo                    string
	headSHA                 string
	context                 string
	dockerfile              string
	targets                 flagutil.Strings
	kanikoArgs              flagutil.Strings
	buildVariant            string
	registry                string
	cacheRegistry           string
	kanikoImage             string
	addVersionTag           bool
	addVersionSHATag        bool
	addDateSHATag           bool
	addDateSHATagWithPrefix flagutil.Strings
	addDateSHATagWithSuffix flagutil.Strings
	addFixedTags            flagutil.Strings
	injectEffectiveVersion  bool

	logLevel string
}

func (o *options) Validate() error {
	if o.dockerfile == "" {
		return fmt.Errorf("\"dockerfile\" parameter must not be empty")
	}
	if o.buildVariant != "" && o.context == "" {
		return fmt.Errorf("specify a \"context\" when setting \"build-variant\" parameter")
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
	if (!o.addVersionTag && !o.addVersionSHATag && !o.addDateSHATag && len(o.addFixedTags.Strings()) == 0) && o.context == "" {
		return fmt.Errorf("please choose at least one tagging scheme")
	}
	if (o.addVersionTag || o.addVersionSHATag || o.addDateSHATag || len(o.addFixedTags.Strings()) > 0) && o.context != "" {
		return fmt.Errorf("\"add-*\" and \"context\" parameters are mutually exclusive")
	}
	if o.headSHA == "" {
		return errors.New("Head SHA must not be empty")
	}
	for _, kanikoArg := range o.kanikoArgs.Strings() {
		if strings.HasPrefix(kanikoArg, "--cache=") || strings.HasPrefix(kanikoArg, "--cache-repo=") {
			return fmt.Errorf("please use --cache-registry option to enable/disable cache")
		}
		if strings.HasPrefix(kanikoArg, "--destination=") || strings.HasPrefix(kanikoArg, "--target=") {
			return fmt.Errorf("please use --registry, --target and --add-[xyz]-tag options to define targets and destinations")
		}
		if strings.HasPrefix(kanikoArg, "--dockerfile=") {
			return fmt.Errorf("please use --dockerfile option to define the path to the dockerfile")
		}
	}
	return nil
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&o.dockerConfigSecret, "docker-config-secret", "", "secret which includes docker config.json file")
	fs.StringVar(&o.dockerfile, "dockerfile", "Dockerfile", "path to dockerfile to be built")
	fs.Var(&o.targets, "target", "target of dockerfile to be built")
	fs.Var(&o.kanikoArgs, "kaniko-arg", "kaniko-arg for the build")
	fs.StringVar(&o.context, "context", "", "(optional) context in github repository which includes the build definition (dockerfile, variants)")
	fs.StringVar(&o.buildVariant, "build-variant", "", "variant of a context which should be built. Builds all variants if empty")
	fs.StringVar(&o.registry, "registry", "", "container registry where build artifacts are being pushed")
	fs.StringVar(&o.cacheRegistry, "cache-registry", "", "container registry where cache artifacts are being pushed. Cache is disabled for empty value")
	fs.StringVar(&o.kanikoImage, "kaniko-image", "gcr.io/kaniko-project/executor:v1.20.1", "kaniko image for kaniko build")
	fs.BoolVar(&o.addVersionTag, "add-version-tag", false, "Add label from VERSION file of git root directory to image tags")
	fs.BoolVar(&o.addVersionSHATag, "add-version-sha-tag", false, "Add label from VERSION file of git root directory plus SHA from git HEAD to image tags")
	fs.BoolVar(&o.addDateSHATag, "add-date-sha-tag", false, "Using vYYYYMMDD-<rev short> scheme which is compatible to autobumper")
	fs.Var(&o.addDateSHATagWithPrefix, "add-date-sha-tag-with-prefix", "Add a <prefix>-vYYYYMMDD-<rev short> tag")
	fs.Var(&o.addDateSHATagWithSuffix, "add-date-sha-tag-with-suffix", "Add a vYYYYMMDD-<rev short>-<suffix> tag which is compatible to autobumper")
	fs.Var(&o.addFixedTags, "add-fixed-tag", "Add a fixed tag to images")
	fs.BoolVar(&o.injectEffectiveVersion, "inject-effective-version", false, "Inject EFFECTIVE_VERSION build-arg")

	fs.StringVar(&o.logLevel, "log-level", "info", fmt.Sprintf("Log level is one of %v.", logrus.AllLevels))

	err := fs.Parse(os.Args[1:])
	if err != nil {
		logrus.Fatalf("Unable to parse command line flags: %v", err)
	}

	jobSpec, err := downwardapi.ResolveSpecFromEnv()
	if err != nil {
		logrus.Fatalf("Unable to resolve prow job spec: %v", err)
	}

	switch jobSpec.Type {
	case prowjobv1.PeriodicJob:
		logrus.Fatal("Image-builder cannot be used in periodic prow jobs")
	case prowjobv1.BatchJob:
		logrus.Info("Skip build in batch job")
		os.Exit(0)
	case prowjobv1.PostsubmitJob:
		o.org = jobSpec.Refs.Org
		o.repo = jobSpec.Refs.Repo
		o.headSHA = jobSpec.Refs.BaseSHA
	case prowjobv1.PresubmitJob:
		o.org = jobSpec.Refs.Org
		o.repo = jobSpec.Refs.Repo
		o.headSHA = jobSpec.Refs.Pulls[0].SHA
	}

	return o
}

func getPodNamespace() (string, error) {
	var namespace string

	versionFile, err := os.Open("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "", errors.Wrap(err, "open /var/run/secrets/kubernetes.io/serviceaccount/namespace")
	}
	defer versionFile.Close()

	scanner := bufio.NewScanner(versionFile)

	for scanner.Scan() {
		namespace = scanner.Text()
		break
	}
	if err := scanner.Err(); err != nil {
		return "", errors.Wrap(err, "scan namespace file")
	}

	if namespace == "" {
		return "", errors.New("no namespace in namespace file")
	}

	return namespace, nil
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
	log := logrus.StandardLogger()

	jobSpec, err := downwardapi.ResolveSpecFromEnv()
	if err != nil {
		log.Fatalf("Unable to resolve prow job spec: %v", err)
	}

	podName := jobSpec.ProwJobID

	podNamespace, err := getPodNamespace()
	if err != nil {
		log.Fatalf("Unable to identify pod namespace %v", err)
	}

	if o.cacheRegistry == "" {
		log.Info("cache-registry parameter is not set. Building without using cache")
	}

	restConfig := config.GetConfigOrDie()

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		log.WithError(err).Fatal("Error getting kubernetes clientset")
	}

	sc := runtime.NewScheme()
	err = corev1.AddToScheme(sc)
	if err != nil {
		log.WithError(err).Fatal("Unable to add corev1 to scheme")
	}
	mgr, err := manager.New(restConfig, manager.Options{Scheme: sc, Namespace: podNamespace})
	if err != nil {
		log.WithError(err).Fatal("Unable to create controller manager")
	}

	log.Info("Setting up build-controller")
	ctx := interrupts.Context()
	controller, err := addImageBuilderController(ctx, mgr, clientset, types.NamespacedName{Name: podName, Namespace: podNamespace}, o, log.WithContext(ctx))
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
	log.Info("Build successful")
}
