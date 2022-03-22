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
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	codeVolume           string = "code"
	dockerConfigVolume   string = "docker-config"
	logArtifactDirectory string = "/logs/artifacts"

	ownerReferencesUID string = "metadata.ownerReferences.uid"

	clonerefsContainerName string = "clonerefs"
	clonerefsEnvName       string = "CLONEREFS_OPTIONS"

	maxErrors int = 5
)

type buildPod struct {
	pod        corev1.Pod
	buildGroup string
}

// buildReconciler controls build process
type buildReconciler struct {
	client     client.Client
	scheme     *runtime.Scheme
	clientset  kubernetes.Interface
	cancelFunc context.CancelFunc
	canceled   bool

	imageBuilderPod types.NamespacedName
	buildPods       []buildPod
	buildPodPhase   map[types.NamespacedName]corev1.PodPhase

	err        error
	errorCount int

	options options
	log     *logrus.Entry
}

// Verifiy that Reconciler interface is implemented
var _ reconcile.Reconciler = &buildReconciler{}

// addImageBuilderController adds a new instace of buildReconciler to the manager
func addImageBuilderController(ctx context.Context, mgr manager.Manager, clientset kubernetes.Interface, cancelFunc context.CancelFunc, imageBuilderPod types.NamespacedName, options options, log *logrus.Entry) (*buildReconciler, error) {

	r := &buildReconciler{
		client:          mgr.GetClient(),
		scheme:          mgr.GetScheme(),
		clientset:       clientset,
		cancelFunc:      cancelFunc,
		imageBuilderPod: imageBuilderPod,
		buildPodPhase:   make(map[types.NamespacedName]corev1.PodPhase),
		options:         options,
		log:             log,
	}
	c, err := controller.New("image-builder-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, errors.Wrap(err, "create controller")
	}

	// Index OwnerReferences.UID
	err = mgr.GetCache().IndexField(ctx, &corev1.Pod{}, ownerReferencesUID, indexOwnerReferences)
	if err != nil {
		return nil, errors.Wrap(err, "add owenerReferences IndexField")
	}

	// Watch build pods
	err = c.Watch(
		&source.Kind{Type: &corev1.Pod{}},
		&handler.EnqueueRequestForOwner{OwnerType: &corev1.Pod{}, IsController: true},
	)
	if err != nil {
		return nil, errors.Wrap(err, "watch build pods")
	}

	// Watch image-builder pod
	err = c.Watch(
		&source.Kind{Type: &corev1.Pod{}},
		&handler.EnqueueRequestForObject{},
		predicate.NewPredicateFuncs(func(o client.Object) bool {
			if o.GetName() == r.imageBuilderPod.Name && o.GetNamespace() == r.imageBuilderPod.Namespace {
				return true
			}
			return false
		}),
	)
	if err != nil {
		return nil, errors.Wrap(err, "watch image-builder pod")
	}

	return r, nil

}

// indexOwnerReferences indexes resources by the UIDs of their owner references.
func indexOwnerReferences(o client.Object) (refs []string) {
	for _, ref := range o.GetOwnerReferences() {
		refs = append(refs, string(ref.UID))
	}
	return refs
}

func (r *buildReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {

	// reconcile image-builder pod only
	if request.NamespacedName != r.imageBuilderPod {
		r.log.Debugf("Skip reconciliation of pod %v - not the image-builder pod", request.NamespacedName)
		return reconcile.Result{}, nil
	}

	// Error handling - try maxErrors times to recover before canceling the build
	var err error
	errorHandler := func() {
		if err == nil {
			return
		}

		r.errorCount++
		if r.errorCount > maxErrors {
			r.log.Error("Too many errors, stopping build")
			r.stop(err)
		}

	}
	defer errorHandler()

	// Get image-builder pod
	var ibPod corev1.Pod
	err = r.client.Get(ctx, request.NamespacedName, &ibPod)
	if err != nil {
		r.log.WithError(err).Error("Could not get image-builder pod - this should not happen")
		return reconcile.Result{}, err
	}

	err = r.ensureBuildPodDefinition(ctx, &ibPod)
	if err != nil {
		r.log.WithError(err).Error("Error ensuring build pod definition")
		return reconcile.Result{}, err
	}

	err = r.reconcileBuildPods(ctx, &ibPod)
	if err != nil {
		r.log.WithError(err).Error("Error reconcile build pods")
		return reconcile.Result{}, err
	}

	return reconcile.Result{RequeueAfter: time.Second * 30}, nil
}

func (r *buildReconciler) stop(err error) {
	if !r.canceled {
		if err != nil {
			r.err = err
		}
		r.cancelFunc()
		r.canceled = true
	}
}

func (r *buildReconciler) ensureBuildPodDefinition(ctx context.Context, ibPod *corev1.Pod) error {
	if len(r.buildPods) != 0 {
		r.log.Debug("Build pods defined - skip")
		return nil
	}
	r.log.Info("Start defining build pods")

	pvc := &corev1.PersistentVolumeClaim{}
	// Use a PVC with the same name and namespace as the image-builder pod
	err := r.client.Get(ctx, types.NamespacedName{Namespace: ibPod.Namespace, Name: ibPod.Namespace}, pvc)
	if k8serrors.IsNotFound(err) {
		r.log.Info("Creating PVC for image build pods")
		pvc, err = r.createPVC(ctx, ibPod)
		if err != nil {
			return errors.Wrap(err, "create PVC")

		}
	} else if err != nil {
		return errors.Wrap(err, "get PVC")
	}

	err = r.defineBuildPods(ibPod, pvc)
	if err != nil {
		r.buildPods = nil
		return errors.Wrap(err, "define build pods")
	}

	r.log.Infof("%d build pods defined for %d targets", len(r.buildPods), len(r.options.targets.Strings()))

	return nil
}

func (r *buildReconciler) createPVC(ctx context.Context, ibPod *corev1.Pod) (*corev1.PersistentVolumeClaim, error) {

	// TODO: it might make sense to make these parameters configurable
	storageClassName := "gce-ssd"
	storageSize, err := resource.ParseQuantity("10Gi")
	if err != nil {
		return nil, errors.Wrap(err, "parse storage quantitiy")
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ibPod.Name,
			Namespace: ibPod.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &storageClassName,
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: storageSize,
				},
			},
		},
	}

	err = controllerutil.SetControllerReference(ibPod, pvc, r.scheme)
	if err != nil {
		return nil, errors.Wrap(err, "set controller reference")
	}

	err = r.client.Create(ctx, pvc, &client.CreateOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "create PVC")
	}

	return pvc, nil
}

func (r *buildReconciler) defineBuildPods(ibPod *corev1.Pod, pvc *corev1.PersistentVolumeClaim) error {

	qtyZero, err := resource.ParseQuantity("0")
	if err != nil {
		return errors.Wrap(err, "parse zero quantity")
	}

	// First pod clones git repository
	clonerefsPod, err := r.defineCloneRefsPod(ibPod, pvc)
	if err != nil {
		return errors.Wrap(err, "define clonerefs pod")
	}
	r.buildPods = append(r.buildPods, buildPod{pod: clonerefsPod, buildGroup: "clonerefs"})

	// Next pods build the targets
	for i, target := range r.options.targets.Strings() {

		var buildGroup string
		pod := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      r.getBuildPodName(ibPod, target),
				Namespace: ibPod.Namespace,
			},
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyNever,
				Volumes: []corev1.Volume{
					{
						Name: dockerConfigVolume,
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: r.options.dockerConfigSecret,
							},
						},
					},
				},
			},
		}

		kanikoContainer := corev1.Container{
			Name:  "kaniko",
			Image: r.options.kanikoImage,
			Args: []string{
				"--skip-unused-stages",
				"--context=/code",
				fmt.Sprintf("--dockerfile=%s", r.options.dockerfile),
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

		destinations, err := r.defineDestinations(target)
		if err != nil {
			return errors.Wrap(err, "construct destinations")
		}

		kanikoContainer.Args = append(kanikoContainer.Args, destinations...)

		if r.options.cacheRegistry != "" {
			kanikoContainer.Args = append(
				kanikoContainer.Args,
				"--cache=true",
				fmt.Sprintf("--cache-repo=%s", r.options.cacheRegistry),
			)
		}

		if i == 0 && r.options.cacheRegistry != "" {
			buildGroup = "createCache"
		} else {
			buildGroup = "parallelBuild"
		}

		pod.Spec.Containers = append(pod.Spec.Containers, kanikoContainer)

		// Configure the build pod with PVC, node assignment and controller reference
		r.assignPVC(pvc, &pod)
		r.setNodeAssignment(ibPod, &pod)
		err = controllerutil.SetControllerReference(ibPod, &pod, r.scheme)
		if err != nil {
			return errors.Wrap(err, "set controller reference")
		}

		r.buildPods = append(r.buildPods, buildPod{pod: pod, buildGroup: buildGroup})
	}

	return nil
}

func (r *buildReconciler) defineCloneRefsPod(ibPod *corev1.Pod, pvc *corev1.PersistentVolumeClaim) (corev1.Pod, error) {

	qtyZero, err := resource.ParseQuantity("0")
	if err != nil {
		return corev1.Pod{}, errors.Wrap(err, "parse zero quantity")
	}

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.getBuildPodName(ibPod, "clonerefs"),
			Namespace: ibPod.Namespace,
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Volumes: []corev1.Volume{
				{
					Name: "logs",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
		},
	}

	// Configure the build pod with PVC, node assignment and controller reference
	r.assignPVC(pvc, &pod)
	r.setNodeAssignment(ibPod, &pod)
	err = controllerutil.SetControllerReference(ibPod, &pod, r.scheme)
	if err != nil {
		return corev1.Pod{}, errors.Wrap(err, "set controller reference")
	}

	for _, ic := range ibPod.Spec.InitContainers {
		if ic.Name == clonerefsContainerName {

			for _, env := range ic.Env {
				if env.Name == clonerefsEnvName {
					c := corev1.Container{
						Name:  ic.Name,
						Image: ic.Image,
						Env:   []corev1.EnvVar{env},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      codeVolume,
								MountPath: fmt.Sprintf("/home/prow/go/src/github.com/%s/%s", r.options.org, r.options.repo),
								SubPath:   "code",
							},
							{
								Name:      "logs",
								MountPath: "/logs",
							},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    qtyZero,
								corev1.ResourceMemory: qtyZero,
							},
						},
					}
					pod.Spec.Containers = append(pod.Spec.Containers, c)
					return pod, nil
				}
			}
		}
	}

	return corev1.Pod{}, errors.New("no clonerefs init container in image-builder pod")
}

func (r *buildReconciler) defineDestinations(target string) ([]string, error) {

	var destinations []string

	if r.options.addVersionTag || r.options.addVersionSHATag {
		var version string

		versionFile, err := os.Open(fmt.Sprintf("/home/prow/go/src/github.com/%s/%s/VERSION", r.options.org, r.options.repo))
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

		if r.options.addVersionTag {
			destination := fmt.Sprintf("--destination=%s/%s:%s", r.options.registry, target, version)
			destinations = append(destinations, destination)
		}

		if r.options.addVersionSHATag {
			tag := fmt.Sprintf("%s-%s", version, r.options.pullBaseSHA)
			destination := fmt.Sprintf("--destination=%s/%s:%s", r.options.registry, target, tag)
			destinations = append(destinations, destination)
		}
	}

	if r.options.addDateSHATag {
		if len(r.options.pullBaseSHA) < 7 {
			return destinations, fmt.Errorf("pullBaseSHA %v is it a correct SHA", r.options.pullBaseSHA)
		}
		tag := fmt.Sprintf("v%s-%s", time.Now().Format("20060102"), r.options.pullBaseSHA[:7])
		destination := fmt.Sprintf("--destination=%s/%s:%s", r.options.registry, target, tag)
		destinations = append(destinations, destination)
	}

	if r.options.addLatestTag {
		destinations = append(destinations, fmt.Sprintf("--destination=%s/%s:latest", r.options.registry, target))
	}

	if r.options.addFixTag != "" {
		destinations = append(destinations, fmt.Sprintf("--destination=%s/%s:%s", r.options.registry, target, r.options.addFixTag))
	}

	return destinations, nil

}

func (r *buildReconciler) getBuildPodName(ibPod *corev1.Pod, target string) string {

	parent := ibPod.Name
	parentLen := len(ibPod.Name)

	suffix := fmt.Sprintf("%s-%s", r.options.repo, target)
	suffixLen := len(suffix)

	var name string

	switch {
	case parentLen+suffixLen <= 64:
		name = fmt.Sprintf("%s-%s", parent, suffix)
	case parentLen+9 <= 64:
		name = fmt.Sprintf("%s-%s", parent, suffix)
		name = fmt.Sprintf("%s-%s", name[:59], rand.SafeEncodeString(rand.String(3)))
	default:
		name = rand.SafeEncodeString(rand.String(64))
	}

	return name
}

func (r *buildReconciler) assignPVC(pvc *corev1.PersistentVolumeClaim, pod *corev1.Pod) {

	pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
		Name: codeVolume,
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: pvc.Name,
			},
		},
	})
}

func (r *buildReconciler) setNodeAssignment(ibPod *corev1.Pod, pod *corev1.Pod) {
	pod.Spec.NodeSelector = ibPod.Spec.NodeSelector
	pod.Spec.Tolerations = ibPod.Spec.Tolerations
	// Run on the same node as parent pod that PVC can be used for multiple pods
	pod.Spec.Affinity = &corev1.Affinity{
		PodAffinity: &corev1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{MatchLabels: ibPod.Labels},
					TopologyKey:   "kubernetes.io/hostname",
				},
			},
		}}
}

func (r *buildReconciler) reconcileBuildPods(ctx context.Context, ibPod *corev1.Pod) error {

	var buildPods corev1.PodList
	err := r.client.List(ctx, &buildPods, client.MatchingFields{ownerReferencesUID: string(ibPod.UID)})
	if err != nil {
		return errors.Wrap(err, "list build pods")
	}

	var (
		runningPods int
		failedPods  int
	)

	// Collect build pod status
	for _, pod := range buildPods.Items {
		if pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodPending {
			runningPods++
		}
		if pod.Status.Phase == corev1.PodFailed {
			failedPods++
		}

		namespacedName := types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}
		if r.buildPodPhase[namespacedName] != pod.Status.Phase {
			r.log.Infof("Build pod %s entered phase %s", pod.Name, pod.Status.Phase)
			if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
				r.log.Infof("Collecting logs of pod %s", pod.Name)
				err := r.collectBuildPodLogs(ctx, namespacedName)
				if err != nil {
					return errors.Wrap(err, "collect logs")
				}
			}
			r.buildPodPhase[namespacedName] = pod.Status.Phase
		}
	}

	if runningPods == 0 {
		if failedPods != 0 {
			r.log.Errorf("%d failed build pods, stopping image-builder", failedPods)
			r.stop(errors.New("build pods ended in phase failed"))
			return nil
		}
		podsStarted, err := r.startNextBuildPods(ctx)
		if err != nil {
			return errors.Wrap(err, "start build pods")
		}
		if !podsStarted {
			r.log.Info("Last build pods completed. Stopping build controller")
			r.stop(nil)
		}
	}

	return nil
}

func (r *buildReconciler) collectBuildPodLogs(ctx context.Context, namespacedName types.NamespacedName) error {

	var podNumber int
	for i, bp := range r.buildPods {
		if bp.pod.Namespace == namespacedName.Namespace && bp.pod.Name == namespacedName.Name {
			podNumber = i
		}
	}

	// controller-runtime does not support log subresource https://github.com/kubernetes-sigs/controller-runtime/issues/452
	req := r.clientset.CoreV1().Pods(namespacedName.Namespace).GetLogs(namespacedName.Name, &corev1.PodLogOptions{})

	logStream, err := req.Stream(ctx)
	if err != nil {
		return errors.Wrap(err, "create log stream")
	}
	defer logStream.Close()

	logFile, err := os.Create(fmt.Sprintf("%s/%03d-%s-build-log.txt", logArtifactDirectory, podNumber, namespacedName.Name))
	if err != nil {
		return errors.Wrap(err, "create log file")
	}
	defer logFile.Close()

	_, err = io.Copy(logFile, logStream)
	if err != nil {
		return errors.Wrap(err, "write log stream to file")
	}

	return nil
}

func (r *buildReconciler) startNextBuildPods(ctx context.Context) (bool, error) {

	var buildGroup string
	podsCreated := false

	for _, buildPod := range r.buildPods {

		namespacedName := types.NamespacedName{Namespace: buildPod.pod.Namespace, Name: buildPod.pod.Name}

		if _, found := r.buildPodPhase[namespacedName]; found {
			continue
		}

		// Stop when a new build group begins
		if buildGroup != "" && buildGroup != buildPod.buildGroup {
			break
		}
		buildGroup = buildPod.buildGroup
		r.log.Debugf("Start creating build pods for build group %s", buildGroup)

		// Create build pod
		err := r.client.Create(ctx, &buildPod.pod, &client.CreateOptions{})
		if err != nil {
			return podsCreated, errors.Wrap(err, "create build pod")
		}

		r.buildPodPhase[namespacedName] = buildPod.pod.Status.Phase
		r.log.Infof("Build pod %s created", buildPod.pod.Name)

		podsCreated = true
	}

	return podsCreated, nil
}
