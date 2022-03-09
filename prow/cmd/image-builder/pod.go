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
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes"
)

const (
	codeVolume string = "code"
)

type podConfiguration struct {
	parentPod *corev1.Pod
	clientset *kubernetes.Clientset
	pvc       *corev1.PersistentVolumeClaim
	options   options
}

func newPodConfiguration(ctx context.Context, name, namespace string, clientset *kubernetes.Clientset, options options) (*podConfiguration, error) {

	parentPod, err := clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "get parent pod")
	}

	// Type meta are returned by API server
	parentPod.APIVersion = "v1"
	parentPod.Kind = "Pod"

	return &podConfiguration{parentPod: parentPod, clientset: clientset, options: options}, nil

}

func (p *podConfiguration) configurePod(ctx context.Context, pod *corev1.Pod) error {
	if p.pvc == nil {
		err := p.createPVC(ctx)
		if err != nil {
			return errors.Wrap(err, "create PVC")
		}
	}

	err := p.assignPVC(pod)
	if err != nil {
		return errors.Wrap(err, "assign PVC")
	}

	err = p.setNodeAssignment(pod)
	if err != nil {
		return errors.Wrap(err, "set node assignment")
	}

	p.setOwnerReferences(pod)

	return nil
}

func (p *podConfiguration) getPodName(suffix string) string {

	parent := p.parentPod.GetName()
	parentLen := len(p.parentPod.GetName())

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

func (p *podConfiguration) getGitClonePod(ctx context.Context, options options) (corev1.Pod, error) {
	if p.parentPod == nil {
		return corev1.Pod{}, errors.New("Parent pod not initialized")
	}

	gitCloneCmd := fmt.Sprintf("git clone https://github.com/%s/%s.git /pvc/code", p.options.org, p.options.repo)
	gitCheckoutCmd := fmt.Sprintf("git checkout %s", options.pullBaseSHA)

	qtyZero, err := resource.ParseQuantity("0")
	if err != nil {
		return corev1.Pod{}, errors.Wrap(err, "parse zero quantity")
	}

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.getPodName("git"),
			Namespace: p.parentPod.GetNamespace(),
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:    "git",
					Image:   p.options.gitImage,
					Command: []string{"sh"},
					Args: []string{
						"-c",
						fmt.Sprintf("set -e\n%s\ncd /pvc/code\n%s", gitCloneCmd, gitCheckoutCmd),
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      codeVolume,
							MountPath: "/pvc",
						},
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    qtyZero,
							corev1.ResourceMemory: qtyZero,
						},
					},
				},
			},
		},
	}

	err = p.configurePod(ctx, &pod)
	if err != nil {
		return corev1.Pod{}, errors.Wrap(err, "configure pod")
	}

	return pod, nil

}

func (p *podConfiguration) createPVC(ctx context.Context) error {
	if p.parentPod == nil {
		return errors.New("Parent pod not initialized")
	}

	if p.pvc != nil {
		return errors.New("PVC already created")
	}

	// TODO: it might make sense to make these parameters configurable
	storageClassName := "gce-ssd"
	storageSize, err := resource.ParseQuantity("10Gi")
	if err != nil {
		return errors.Wrap(err, "parse storage quantitiy")
	}

	pvc := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.parentPod.Name,
			Namespace: p.parentPod.Namespace,
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

	pvc.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: p.parentPod.APIVersion,
			Kind:       p.parentPod.Kind,
			Name:       p.parentPod.Name,
			UID:        p.parentPod.UID,
		},
	})

	p.pvc, err = p.clientset.CoreV1().PersistentVolumeClaims(p.parentPod.Namespace).Create(ctx, &pvc, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, "create PVC")
	}

	return nil
}

func (p *podConfiguration) assignPVC(pod *corev1.Pod) error {
	if p.parentPod == nil {
		return errors.New("Parent pod not initialized")
	}

	if p.pvc == nil {
		return errors.New("PVC not initialized")
	}

	pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
		Name: codeVolume,
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: p.pvc.Name,
			},
		},
	})

	return nil
}

func (p *podConfiguration) setNodeAssignment(pod *corev1.Pod) error {
	if p.parentPod == nil {
		return errors.New("Parent pod not initialized")
	}

	pod.Spec.NodeSelector = p.parentPod.Spec.NodeSelector
	pod.Spec.Tolerations = p.parentPod.Spec.Tolerations
	// Run on the same node as parent pod that PVC can be used for multiple pods
	pod.Spec.Affinity = &corev1.Affinity{
		PodAffinity: &corev1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{MatchLabels: p.parentPod.Labels},
					TopologyKey:   "kubernetes.io/hostname",
				},
			},
		}}

	return nil
}

func (p *podConfiguration) setOwnerReferences(pod *corev1.Pod) {

	pod.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: p.parentPod.APIVersion,
			Kind:       p.parentPod.Kind,
			Name:       p.parentPod.Name,
			UID:        p.parentPod.UID,
		},
	})
}
