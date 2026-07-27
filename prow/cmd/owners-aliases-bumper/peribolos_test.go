// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/prow/pkg/config/org"
)

var _ = Describe("Peribolos", func() {
	Describe("#removeExcludedRepos", func() {
		newConfig := func() org.FullConfig {
			return org.FullConfig{
				Orgs: map[string]org.Config{
					"gardener": {
						Repos: map[string]org.Repo{
							"ci-infra":   {},
							"gardener":   {},
							"test-infra": {},
						},
					},
				},
			}
		}

		It("removes a single excluded repo, leaving the rest", func() {
			cfg := newConfig()
			removeExcludedRepos(&cfg, []string{"gardener/ci-infra"})

			repos := cfg.Orgs["gardener"].Repos
			Expect(repos).ToNot(HaveKey("ci-infra"))
			Expect(repos).To(HaveKey("gardener"))
			Expect(repos).To(HaveKey("test-infra"))
		})

		It("removes multiple excluded repos", func() {
			cfg := newConfig()
			removeExcludedRepos(&cfg, []string{"gardener/ci-infra", "gardener/test-infra"})

			repos := cfg.Orgs["gardener"].Repos
			Expect(repos).To(HaveLen(1))
			Expect(repos).To(HaveKey("gardener"))
		})

		It("is a no-op when nothing is excluded", func() {
			cfg := newConfig()
			removeExcludedRepos(&cfg, nil)
			Expect(cfg.Orgs["gardener"].Repos).To(HaveLen(3))
		})

		It("tolerates excluding a repo that is not defined", func() {
			cfg := newConfig()
			Expect(func() {
				removeExcludedRepos(&cfg, []string{"gardener/does-not-exist"})
			}).ToNot(Panic())
			Expect(cfg.Orgs["gardener"].Repos).To(HaveLen(3))
		})
	})
})
