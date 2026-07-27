// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/prow/pkg/config/org"
	"sigs.k8s.io/prow/pkg/github"
	"sigs.k8s.io/prow/pkg/repoowners"
)

// orgAliases holds an alias -> member map for a single org.
type orgAliases repoowners.RepoAliases

func (a orgAliases) addMember(aliasName string, member string) {
	aliasMembers, exists := a[aliasName]

	if !exists {
		aliasMembers = sets.New(member)
		a[aliasName] = aliasMembers
		return
	}

	aliasMembers.Insert(member)
}

func (a orgAliases) getMembers(aliasName string) sets.Set[string] {
	aliasMembers, exists := a[aliasName]

	if !exists {
		return nil // return nil if this alias does not exist
	}

	return aliasMembers
}

// fullOrgAliases holds the aliases mapped per org.
type fullOrgAliases struct {
	aliases map[string]orgAliases
}

func newFullOrgAliases() *fullOrgAliases {
	return &fullOrgAliases{
		aliases: make(map[string]orgAliases),
	}
}

func (f *fullOrgAliases) getConfig(org string) orgAliases {
	a, exists := f.aliases[org]

	if !exists {
		a = make(orgAliases)
		f.aliases[org] = a
	}

	return a
}

func addMembersFromTeams(aliases orgAliases, teams map[string]org.Team, aliasPrefix string) {
	for teamName, team := range teams {
		alias := github.NormLogin(aliasPrefix + teamName)
		for _, member := range team.Members {
			aliases.addMember(alias, github.NormLogin(member))
		}
		for _, maintainer := range team.Maintainers {
			aliases.addMember(alias, github.NormLogin(maintainer))
		}
		// handle children recursively (children teams could have same name as any parent team -> aliasPrefix with '/' delimiter)
		addMembersFromTeams(aliases, team.Children, aliasPrefix+teamName+"/")
	}
}
