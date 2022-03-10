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
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	dockerConfigVolume   string = "docker-config"
	logArtifactDirectory string = "/logs/artifacts"
)

type buildPod struct {
	pod        corev1.Pod
	buildGroup string
}

func buildTargets(ctx context.Context, log *logrus.Entry, podConfig *podConfiguration, options options) error {

	if podConfig.parentPod == nil {
		return errors.New("podConfiguration is not initialized")
	}

	kanikoPods, err := defineKanikoBuildPods(ctx, podConfig, options)
	if err != nil {
		return errors.Wrap(err, "define kaniko pods")
	}

	podListWatcher := cache.NewListWatchFromClient(podConfig.clientset.CoreV1().RESTClient(), "pods", podConfig.parentPod.GetNamespace(), fields.Everything())

	// create the workqueue
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	indexer, informer := cache.NewIndexerInformer(podListWatcher, &corev1.Pod{}, 0, cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add(key)
			}
		},
		UpdateFunc: func(old interface{}, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)
			if err == nil {
				queue.Add(key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			// IndexerInformer uses a delta queue, therefore for deletes we have to use this
			// key function.
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add(key)
			}
		},
	}, cache.Indexers{})

	controller := newBuildController(log, podConfig.clientset, queue, indexer, informer, kanikoPods)

	return controller.run()
}

func defineKanikoBuildPods(ctx context.Context, podConfig *podConfiguration, options options) ([]buildPod, error) {

	var buildPods []buildPod

	if podConfig.parentPod == nil {
		return buildPods, errors.New("podConfiguration is not initialized")
	}

	qtyZero, err := resource.ParseQuantity("0")
	if err != nil {
		return buildPods, errors.Wrap(err, "parse zero quantity")
	}

	gitClonePod, err := podConfig.getGitClonePod(ctx, options)
	if err != nil {
		return buildPods, errors.Wrap(err, "get git clone pod")
	}
	buildPods = append(buildPods, buildPod{pod: gitClonePod, buildGroup: "git-clone"})

	for i, target := range options.targets.Strings() {

		var buildGroup string
		pod := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podConfig.getPodName(fmt.Sprintf("%s-%s", options.repo, target)),
				Namespace: podConfig.parentPod.GetNamespace(),
			},
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyNever,
				Volumes: []corev1.Volume{
					{
						Name: dockerConfigVolume,
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: options.dockerConfigSecret,
							},
						},
					},
				},
			},
		}

		kanikoContainer := corev1.Container{
			Name:  "kaniko",
			Image: options.kanikoImage,
			Args: []string{
				"--skip-unused-stages",
				"--context=/code",
				fmt.Sprintf("--dockerfile=%s", options.dockerfile),
				fmt.Sprintf("--target=%s", target),
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      codeVolume,
					MountPath: "/code",
					SubPath:   "code",
				},
				{
					Name:      dockerConfigVolume,
					MountPath: "/kaniko/.docker",
				},
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    qtyZero,
					corev1.ResourceMemory: qtyZero,
				},
			},
		}

		destinations, err := defineDestinations(target, options)
		if err != nil {
			return buildPods, errors.Wrap(err, "construct destinations")
		}

		kanikoContainer.Args = append(kanikoContainer.Args, destinations...)

		if options.cacheRegistry != "" {
			kanikoContainer.Args = append(
				kanikoContainer.Args,
				"--cache=true",
				fmt.Sprintf("--cache-repo=%s", options.cacheRegistry),
			)
		}

		if i == 0 && options.cacheRegistry != "" {
			buildGroup = "createCache"
		} else {
			buildGroup = "parallelBuild"
		}

		pod.Spec.Containers = append(pod.Spec.Containers, kanikoContainer)

		err = podConfig.configurePod(ctx, &pod)
		if err != nil {
			return buildPods, errors.Wrap(err, "configure pod")
		}

		buildPods = append(buildPods, buildPod{pod: pod, buildGroup: buildGroup})
	}

	return buildPods, nil

}

func defineDestinations(target string, options options) ([]string, error) {

	var destinations []string

	if options.addVersionTag || options.addVersionSHATag {
		var version string

		versionFile, err := os.Open(fmt.Sprintf("/home/prow/go/src/github.com/%s/%s/VERSION", options.org, options.repo))
		if err != nil {
			return destinations, errors.Wrap(err, "open VERSION file from git root directory")
		}
		defer versionFile.Close()

		scanner := bufio.NewScanner(versionFile)

		for scanner.Scan() {
			version = scanner.Text()
			break
		}
		if scanner.Err() != nil {
			return destinations, errors.Wrap(err, "scan VERSION file")
		}

		if version == "" {
			return destinations, errors.New("no version in VERSION file")
		}

		if options.addVersionTag {
			destination := fmt.Sprintf("--destination=%s/%s:%s", options.registry, target, version)
			destinations = append(destinations, destination)
		}

		if options.addVersionSHATag {
			tag := fmt.Sprintf("%s-%s", version, options.pullBaseSHA)
			destination := fmt.Sprintf("--destination=%s/%s:%s", options.registry, target, tag)
			destinations = append(destinations, destination)
		}
	}

	if options.addDateSHATag {
		if len(options.pullBaseSHA) < 7 {
			return destinations, fmt.Errorf("pullBaseSHA %v is it a correct SHA", options.pullBaseSHA)
		}
		tag := fmt.Sprintf("v%s-%s", time.Now().Format("20060102"), options.pullBaseSHA[:7])
		destination := fmt.Sprintf("--destination=%s/%s:%s", options.registry, target, tag)
		destinations = append(destinations, destination)
	}

	if options.addLatestTag {
		destinations = append(destinations, fmt.Sprintf("--destination=%s/%s:latest", options.registry, target))
	}

	return destinations, nil

}

func newBuildController(
	log *logrus.Entry,
	clientset *kubernetes.Clientset,
	queue workqueue.RateLimitingInterface,
	indexer cache.Indexer, informer cache.Controller,
	buildPods []buildPod) *buildController {
	return &buildController{
		log:              log,
		clientset:        clientset,
		informer:         informer,
		indexer:          indexer,
		queue:            queue,
		buildPods:        buildPods,
		runningBuildPods: make(map[string]bool),
		stopCh:           make(chan struct{}),
	}
}

type buildController struct {
	log               *logrus.Entry
	clientset         *kubernetes.Clientset
	indexer           cache.Indexer
	queue             workqueue.RateLimitingInterface
	informer          cache.Controller
	buildPods         []buildPod
	buildPodsIndex    int
	runningBuildPods  map[string]bool
	updateBuildPodsMu sync.Mutex
	stopController    bool
	stopCh            chan struct{}
	err               error
}

func (c *buildController) run() error {
	defer runtime.HandleCrash()
	defer c.queue.ShutDown()

	c.log.Info("Starting build controller")

	go c.informer.Run(c.stopCh)

	if !cache.WaitForCacheSync(c.stopCh, c.informer.HasSynced) {
		return errors.New("Timed out waiting for caches to sync")
	}

	go wait.Until(c.runWorker, time.Second, c.stopCh)

	c.updateBuildPodsMu.Lock()
	_, err := c.startNextBuildGroup()
	if err != nil {
		c.updateBuildPodsMu.Unlock()
		return errors.Wrap(err, "creating pods for first build group")
	}
	c.updateBuildPodsMu.Unlock()

	<-c.stopCh
	c.log.Info("Build completed - collecting logs")
	c.collectBuildPodLogs()
	return c.err
}

func (c *buildController) stop(err error) {
	if err != nil {
		c.err = err
	}
	c.stopController = true
}

func (c *buildController) runWorker() {
	for c.processNextItem() {
	}
}

func (c *buildController) processNextItem() bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	c.updateBuildPodsMu.Lock()
	defer c.updateBuildPodsMu.Unlock()

	err := c.handleBuildPods(key.(string))
	c.handleErr(err, key)

	// Stop controller when build is stopped and there are no more running build pods
	if c.stopController && len(c.runningBuildPods) == 0 {
		close(c.stopCh)
		return false
	}

	return true
}

func (c *buildController) handleBuildPods(key string) error {
	// Not one of our pods, do nothing
	if !c.runningBuildPods[key] {
		return nil
	}

	obj, exists, err := c.indexer.GetByKey(key)
	if err != nil {
		c.log.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exists {
		delete(c.runningBuildPods, key)
		c.log.Errorf("Build pod %s does not exist anymore. Aborting build", key)
		c.stop(fmt.Errorf("build pod %s does not exist anymore. Aborting build", key))
	}

	pod := obj.(*corev1.Pod)

	if pod.Status.Phase == corev1.PodSucceeded {
		delete(c.runningBuildPods, key)
		c.log.Infof("Build pod %s succeeded", pod.GetName())
	}

	if pod.Status.Phase == corev1.PodFailed {
		delete(c.runningBuildPods, key)
		c.log.Errorf("Build pod %s failed. Aborting build", pod.GetName())
		c.stop(fmt.Errorf("build pod %s failed. Aborting build", pod.GetName()))
	}

	if !c.stopController && len(c.runningBuildPods) == 0 {
		podsCreated, err := c.startNextBuildGroup()
		if err != nil {
			// Add pod to running pods again, that it can be requeued
			c.runningBuildPods[key] = true
			return errors.Wrap(err, "start next build group")
		}
		if !podsCreated {
			c.log.Info("Last build pods completed. Stopping build controller")
			c.stop(nil)
		}
	}

	return nil
}

func (c *buildController) handleErr(err error, key interface{}) {
	if err == nil {
		c.queue.Forget(key)
		return
	}
	if c.queue.NumRequeues(key) < 5 {
		c.log.WithError(err).Errorf("Error processing build pod %v", key)

		c.queue.AddRateLimited(key)
		return
	}

	c.queue.Forget(key)
	c.log.WithError(err).Errorf("Aborting build process because of unrecoverable error at build pod %v", key)
	c.stop(err)
}

func (c *buildController) startNextBuildGroup() (bool, error) {

	ctx := context.Background()

	buildGroup := ""

	podsCreated := false

	for i, buildPod := range c.buildPods {
		if i < c.buildPodsIndex {
			continue
		}

		// Stop when a new build group begins
		if buildGroup != "" && buildGroup != buildPod.buildGroup {
			break
		}
		buildGroup = buildPod.buildGroup
		c.log.Debugf("Start creating build pods for build group %s", buildGroup)

		// Create build pod
		pod, err := c.clientset.CoreV1().Pods(buildPod.pod.GetNamespace()).Create(ctx, &buildPod.pod, metav1.CreateOptions{})
		if err != nil {
			return podsCreated, errors.Wrap(err, "create build pod")
		}

		c.log.Infof("Build pod %s created", pod.GetName())

		key, err := cache.MetaNamespaceKeyFunc(pod)
		if err != nil {
			return podsCreated, errors.Wrap(err, "create NamespaceKey")
		}
		c.runningBuildPods[key] = true

		c.buildPodsIndex = i + 1
		podsCreated = true
	}

	return podsCreated, nil
}

func (c *buildController) collectBuildPodLogs() {

	ctx := context.Background()

	for i, buildPod := range c.buildPods {
		c.log.Infof("Collecting logs for pod %s", buildPod.pod.GetName())

		req := c.clientset.CoreV1().Pods(buildPod.pod.Namespace).GetLogs(buildPod.pod.Name, &corev1.PodLogOptions{})

		logStream, err := req.Stream(ctx)
		if err != nil {
			c.log.WithError(err).Errorf("Error creating log stream for pod %s", buildPod.pod.GetName())
			continue
		}
		defer logStream.Close()

		logFile, err := os.Create(fmt.Sprintf("%s/%03d-%s-build-log.txt", logArtifactDirectory, i, buildPod.pod.GetName()))
		if err != nil {
			c.log.WithError(err).Errorf("Error creating log file for pod %s", buildPod.pod.GetName())
			continue
		}
		defer logFile.Close()

		_, err = io.Copy(logFile, logStream)
		if err != nil {
			c.log.WithError(err).Errorf("Error writing log stream for pod %s to file", buildPod.pod.GetName())
			continue
		}
	}
}
