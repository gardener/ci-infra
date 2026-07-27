// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/prow/pkg/config/org"
)

var _ = Describe("Aliases", func() {
	Describe("orgAliases", func() {
		It("adds a member to a new alias", func() {
			a := orgAliases{}
			a.addMember("team-a", "alice")
			Expect(a.getMembers("team-a")).To(Equal(sets.New("alice")))
		})

		It("adds multiple members to the same alias without duplicates", func() {
			a := orgAliases{}
			a.addMember("team-a", "alice")
			a.addMember("team-a", "bob")
			a.addMember("team-a", "alice")
			Expect(a.getMembers("team-a")).To(Equal(sets.New("alice", "bob")))
		})

		It("returns nil for an unknown alias", func() {
			a := orgAliases{}
			Expect(a.getMembers("missing")).To(BeNil())
		})
	})

	Describe("fullOrgAliases", func() {
		It("lazily creates and reuses the config for an org", func() {
			f := newFullOrgAliases()
			c1 := f.getConfig("gardener")
			c1.addMember("team-a", "alice")

			c2 := f.getConfig("gardener")
			Expect(c2.getMembers("team-a")).To(Equal(sets.New("alice")),
				"getConfig should return the same underlying map for the same org")
		})

		It("keeps orgs isolated from each other", func() {
			f := newFullOrgAliases()
			f.getConfig("org-a").addMember("team", "alice")
			f.getConfig("org-b").addMember("team", "bob")

			Expect(f.getConfig("org-a").getMembers("team")).To(Equal(sets.New("alice")))
			Expect(f.getConfig("org-b").getMembers("team")).To(Equal(sets.New("bob")))
		})
	})

	Describe("#addMembersFromTeams", func() {
		It("adds members and maintainers under the team-named alias", func() {
			a := orgAliases{}
			addMembersFromTeams(a, map[string]org.Team{
				"team-a": {
					Members:     []string{"alice"},
					Maintainers: []string{"bob"},
				},
			}, "")
			Expect(a.getMembers("team-a")).To(Equal(sets.New("alice", "bob")))
		})

		It("normalizes team names and logins to lowercase without leading @", func() {
			a := orgAliases{}
			addMembersFromTeams(a, map[string]org.Team{
				"Team-A": {
					Members:     []string{"Alice"},
					Maintainers: []string{"@Bob"},
				},
			}, "")
			Expect(a.getMembers("team-a")).To(Equal(sets.New("alice", "bob")),
				"both the alias key and the members must be normalized to match repoowners.ParseAliasesConfig")
			Expect(a.getMembers("Team-A")).To(BeNil())
		})

		It("prefixes child team aliases with the parent path", func() {
			a := orgAliases{}
			addMembersFromTeams(a, map[string]org.Team{
				"parent": {
					Members: []string{"alice"},
					Children: map[string]org.Team{
						"child": {
							Members: []string{"bob"},
						},
					},
				},
			}, "")
			Expect(a.getMembers("parent")).To(Equal(sets.New("alice")))
			Expect(a.getMembers("parent/child")).To(Equal(sets.New("bob")))
		})

		It("applies the given prefix to top-level teams", func() {
			a := orgAliases{}
			addMembersFromTeams(a, map[string]org.Team{
				"team-a": {Members: []string{"alice"}},
			}, "prefix/")
			Expect(a.getMembers("prefix/team-a")).To(Equal(sets.New("alice")))
		})
	})
})
