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
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clgofake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/test-infra/prow/flagutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"
)

var (
	testImageBuilderPod types.NamespacedName = types.NamespacedName{Namespace: "test-pods", Name: "prow-job-image-build-pod"}
)

func unmarshalYAML(t *testing.T, v interface{}, s string) {
	t.Helper()
	if err := yaml.Unmarshal([]byte(strings.TrimSpace(s)), v); err != nil {
		t.Fatal(err)
	}
}

func createTestImageBuilderPod(t *testing.T) corev1.Pod {
	t.Helper()
	var ibPod corev1.Pod

	unmarshalYAML(t, &ibPod, `
apiVersion: v1
kind: Pod
metadata:
  labels:
    created-by-prow: "true"
    prow.k8s.io/build-id: "1234567890"
    prow.k8s.io/context: prow-job-name
    prow.k8s.io/id: prow-job-image-build-pod
    prow.k8s.io/job: prow-job-name
    prow.k8s.io/plank-version: latest
    prow.k8s.io/refs.base_ref: main
    prow.k8s.io/refs.org: git-org
    prow.k8s.io/refs.repo: git-repo
    prow.k8s.io/type: postsubmit
  name: prow-job-image-build-pod
  namespace: test-pods
  uid: "abcdef-1234567890"
spec:
  affinity:
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
      - podAffinityTerm:
        labelSelector:
        matchExpressions:
        - key: prow.k8s.io/job
          operator: In
          values:
          - prow-job-name
        topologyKey: kubernetes.io/hostname
      weight: 100
  containers:
  - image: registry.xyz/image-builder:latest
    name: test
  initContainers:
  - env:
    - name: CLONEREFS_OPTIONS
      value: '{"option1":"value1","option2":"value2"}'
    image: registry.xyz/clonerefs:latest
    name: clonerefs
  nodeSelector:
    dedicated: high-cpu
  tolerations:
  - effect: NoSchedule
    key: dedicated
    operator: Equal
    value: high-cpu
  - effect: NoExecute
    key: node.kubernetes.io/not-ready
    operator: Exists
    tolerationSeconds: 300
  - effect: NoExecute
    key: node.kubernetes.io/unreachable
    operator: Exists
    tolerationSeconds: 300
`)

	return ibPod
}

func createTestImageBuildController(t *testing.T, initObjs ...client.Object) *buildReconciler {
	t.Helper()

	ctx, cancelFunc := context.WithCancel(context.Background())
	logrus.SetLevel(logrus.DebugLevel)
	log := logrus.StandardLogger().WithContext(ctx)

	sc := runtime.NewScheme()
	err := corev1.AddToScheme(sc)
	if err != nil {
		t.Fatal(err)
	}
	client := ctrlfake.NewClientBuilder().WithScheme(sc).WithObjects(initObjs...).Build()
	clientset := clgofake.NewSimpleClientset()

	options := options{
		org:                "git-org",
		repo:               "git-repo",
		kanikoImage:        "registry.xyz/kaniko:latest",
		dockerConfigSecret: "docker-config-secret",
		dockerfile:         "dockerfile",
		registry:           "registry.xyz/build",
		cacheRegistry:      "registry.xyz/cache",
		addVersionTag:      false,
		addVersionSHATag:   false,
		addDateSHATag:      true,
		addLatestTag:       true,
		addFixTag:          "test",
		logLevel:           "debug",
		targets:            flagutil.NewStrings("target1", "target2", "target3"),
		pullBaseSHA:        "abcdef1234567890",
	}

	r := &buildReconciler{
		client:          client,
		scheme:          sc,
		clientset:       clientset,
		cancelFunc:      cancelFunc,
		imageBuilderPod: testImageBuilderPod,
		buildPodPhase:   make(map[types.NamespacedName]corev1.PodPhase),
		options:         options,
		log:             log,
	}
	return r
}

func TestEnsureBuildPodDefinition(t *testing.T) {

	// Preparation
	ctx := context.Background()
	testPod := createTestImageBuilderPod(t)
	r := createTestImageBuildController(t, &testPod)
	var ibPod corev1.Pod
	err := r.client.Get(ctx, testImageBuilderPod, &ibPod)
	if err != nil {
		t.Fatal(err)
	}

	// Test
	err = r.ensureBuildPodDefinition(ctx, &ibPod)
	assert.NoError(t, err)

	// One pod per target + one clonerefs pod
	assert.Len(t, r.buildPods, len(r.options.targets.Strings())+1)

	// Build pods must include an owner reference, node selector and tolerations from build pod
	for _, buildPod := range r.buildPods {
		if len(buildPod.pod.OwnerReferences) > 0 {
			assert.Equal(t, ibPod.UID, buildPod.pod.OwnerReferences[0].UID)
		} else {
			assert.Fail(t, "No owner reference in build pod")
		}
		assert.Equal(t, ibPod.Spec.NodeSelector, buildPod.pod.Spec.NodeSelector)
		assert.Equal(t, ibPod.Spec.Tolerations, buildPod.pod.Spec.Tolerations)
	}

	// pvc for build pods should be created with the same name as image-builder pod
	var pvc corev1.PersistentVolumeClaim
	err = r.client.Get(ctx, testImageBuilderPod, &pvc)
	assert.NoError(t, err)

}

func TestGetBuildPodName(t *testing.T) {

	type testCase struct {
		name   string
		target string
	}

	tests := []testCase{
		{
			name:   "short target",
			target: "s",
		},
		{
			name:   "target longer than 64 chars",
			target: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				// Preparation
				ctx := context.Background()
				testPod := createTestImageBuilderPod(t)
				r := createTestImageBuildController(t, &testPod)
				var ibPod corev1.Pod
				err := r.client.Get(ctx, testImageBuilderPod, &ibPod)
				if err != nil {
					t.Fatal(err)
				}

				// Test
				podName := r.getBuildPodName(&ibPod, test.target)
				assert.LessOrEqual(t, len(podName), 64)
			})
	}
}

func TestReconcileBuildPods(t *testing.T) {
	// Preparation
	ctx := context.Background()
	testPod := createTestImageBuilderPod(t)
	r := createTestImageBuildController(t, &testPod)
	var ibPod corev1.Pod
	err := r.client.Get(ctx, testImageBuilderPod, &ibPod)
	if err != nil {
		t.Fatal(err)
	}

	// Test
	err = r.ensureBuildPodDefinition(ctx, &ibPod)
	assert.NoError(t, err)

	err = r.reconcileBuildPods(ctx, &ibPod)
	assert.NoError(t, err)

	// One build pod should been created
	assert.Len(t, r.buildPodPhase, 1)
}

func TestReconcile(t *testing.T) {
	// Preparation
	ctx := context.Background()
	testPod := createTestImageBuilderPod(t)
	r := createTestImageBuildController(t, &testPod)
	var ibPod corev1.Pod
	err := r.client.Get(ctx, testImageBuilderPod, &ibPod)
	if err != nil {
		t.Fatal(err)
	}

	// Test
	_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: testImageBuilderPod})
	assert.NoError(t, err)

	// One pod per target + one clonerefs pod
	assert.Len(t, r.buildPods, len(r.options.targets.Strings())+1)

	// One build pod should been created
	assert.Len(t, r.buildPodPhase, 1)

	// pvc for build pods should be created with the same name as image-builder pod
	var pvc corev1.PersistentVolumeClaim
	err = r.client.Get(ctx, testImageBuilderPod, &pvc)
	assert.NoError(t, err)
}
