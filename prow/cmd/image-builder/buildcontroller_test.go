// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clgofake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/prow/prow/flagutil"
	"sigs.k8s.io/prow/prow/interrupts"
	"sigs.k8s.io/yaml"
)

var (
	testImageBuilderPod types.NamespacedName = types.NamespacedName{Namespace: "test-pods", Name: "prow-job-image-build-pod"}
)

const (
	testContext string = "images/test"
	testGitOrg  string = "git-org"
	testGitRepo string = "git-repo"
)

func createTestFileSystem(t *testing.T) fstest.MapFS {
	t.Helper()

	version := "1.1-test"

	versionFile := fstest.MapFile{
		Data:    []byte(version),
		Mode:    fs.ModePerm,
		ModTime: time.Now(),
	}

	variantsYaml := `apiVersion: variants/v1alpha1
kind: Variants
variants:
  v1:
    BUILD_ARG1: v1
    BUILD_ARG2: v1
  v2:
    BUILD_ARG1: v2
    BUILD_ARG2: v2
`

	variantsYamlFile := fstest.MapFile{
		Data:    []byte(variantsYaml),
		Mode:    fs.ModePerm,
		ModTime: time.Now(),
	}

	mapFS := fstest.MapFS{
		fmt.Sprintf("github.com/%s/%s/VERSION", testGitOrg, testGitRepo): &versionFile,
		fmt.Sprintf("%s/%s", testContext, variantsFile):                  &variantsYamlFile,
	}

	return mapFS

}

func unmarshalYAML(t *testing.T, v interface{}, s string) {
	t.Helper()
	if err := yaml.Unmarshal([]byte(strings.TrimSpace(s)), v); err != nil {
		t.Fatal(err)
	}
}

func createTestImageBuilderPod(t *testing.T) corev1.Pod {
	t.Helper()
	var ibPod corev1.Pod

	unmarshalYAML(t, &ibPod, `apiVersion: v1
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

	ctx := interrupts.Context()
	logrus.SetLevel(logrus.DebugLevel)
	log := logrus.StandardLogger().WithContext(ctx)

	sc := runtime.NewScheme()
	err := corev1.AddToScheme(sc)
	if err != nil {
		t.Fatal(err)
	}
	client := ctrlfake.NewClientBuilder().WithScheme(sc).WithObjects(initObjs...).Build()
	clientset := clgofake.NewSimpleClientset()

	mapFS := createTestFileSystem(t)

	options := options{
		org:                     testGitOrg,
		repo:                    testGitRepo,
		kanikoImage:             "registry.xyz/kaniko:latest",
		dockerConfigSecret:      "docker-config-secret",
		dockerfile:              "Dockerfile.test",
		registry:                "registry.xyz/build",
		cacheRegistry:           "registry.xyz/cache",
		addVersionTag:           true,
		addVersionSHATag:        true,
		addDateSHATag:           true,
		addDateSHATagWithPrefix: flagutil.NewStrings("pre"),
		addDateSHATagWithSuffix: flagutil.NewStrings("suf"),
		addFixedTags:            flagutil.NewStrings("test"),
		logLevel:                "debug",
		targets:                 flagutil.NewStrings("target1", "target2", "target3"),
		kanikoArgs:              flagutil.NewStrings("--build-arg=buildarg1=abc", "--build-arg=buildarg1=xyz"),
		headSHA:                 "abcdef1234567890",
	}

	r := &buildReconciler{
		client:          client,
		scheme:          sc,
		clientset:       clientset,
		imageBuilderPod: testImageBuilderPod,
		buildPodPhase:   make(map[types.NamespacedName]corev1.PodPhase),
		options:         options,
		fileSystem:      mapFS,
		readFiler:       mapFS.ReadFile,
		log:             log,
	}
	return r
}

func TestEnsureBuildPodDefinition(t *testing.T) {

	type testCase struct {
		name    string
		context string
	}

	tests := []testCase{
		{
			name:    "empty context",
			context: "",
		},
		{
			name:    "non-empty context",
			context: testContext,
		},
	}

	ctx := context.Background()

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				// Preparation
				testPod := createTestImageBuilderPod(t)
				r := createTestImageBuildController(t, &testPod)
				r.options.context = test.context
				var ibPod corev1.Pod
				err := r.client.Get(ctx, testImageBuilderPod, &ibPod)
				if err != nil {
					t.Fatal(err)
				}

				// Test
				err = r.ensureBuildPodDefinition(ctx, &ibPod)
				assert.NoError(t, err)

				if test.context == "" {
					// One pod per target + one clonerefs pod
					assert.Len(t, r.buildPods, len(r.options.targets.Strings())+1)
				} else {
					// One pod per target * number of variants (2 defined in createTestReadFiler()) + one clonerefs pod
					assert.Len(t, r.buildPods, len(r.options.targets.Strings())*2+1)
				}

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
			})
	}
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

func TestGetVariants(t *testing.T) {
	type testCase struct {
		name         string
		buildVariant string

		expectedVariants int
	}

	tests := []testCase{
		{
			name:         "all variants",
			buildVariant: "",

			expectedVariants: 2,
		},
		{
			name:         "variant v1",
			buildVariant: "v1",

			expectedVariants: 1,
		},
		{
			name:         "non existing variant",
			buildVariant: "xyz",

			expectedVariants: 0,
		},
	}

	ctx := context.Background()

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				// Preparation
				testPod := createTestImageBuilderPod(t)
				r := createTestImageBuildController(t, &testPod)
				r.options.context = testContext
				r.options.buildVariant = test.buildVariant
				var ibPod corev1.Pod
				err := r.client.Get(ctx, testImageBuilderPod, &ibPod)
				if err != nil {
					t.Fatal(err)
				}

				// Test
				variants, err := r.getVariants()
				assert.NoError(t, err)

				assert.Len(t, variants, test.expectedVariants)
			})
	}
}
