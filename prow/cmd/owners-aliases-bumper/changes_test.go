// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/prow/pkg/config/org"
	"sigs.k8s.io/prow/pkg/github"

	yaml4 "go.yaml.in/yaml/v4"
)

// fakeFileGetter is a minimal fileGetter for calculateAliasChanges.
type fakeFileGetter struct {
	content []byte
	err     error
}

func (f fakeFileGetter) GetFile(_, _, _, _ string) ([]byte, error) {
	return f.content, f.err
}

var _ = Describe("Changes", func() {
	Describe("#deleteValue", func() {
		It("removes all occurrences of the value", func() {
			Expect(deleteValue([]string{"a", "b", "a", "c"}, "a")).To(Equal([]string{"b", "c"}))
		})

		It("leaves the slice unchanged when the value is absent", func() {
			Expect(deleteValue([]string{"a", "b"}, "z")).To(Equal([]string{"a", "b"}))
		})

		It("returns empty when all elements match", func() {
			Expect(deleteValue([]string{"a", "a"}, "a")).To(BeEmpty())
		})
	})

	Describe("#calculateAliasChanges", func() {
		// localConfig builds a fullOrgAliases with a single org "gardener".
		localConfig := func(alias string, members ...string) *fullOrgAliases {
			f := newFullOrgAliases()
			cfg := f.getConfig("gardener")
			for _, m := range members {
				cfg.addMember(alias, m)
			}
			return f
		}

		It("returns nil when the repo has no OWNERS_ALIASES", func() {
			gh := fakeFileGetter{err: &github.FileNotFound{}}
			changes := calculateAliasChanges(gh, newFullOrgAliases(), "gardener", "ci-infra")
			Expect(changes).To(BeNil())
		})

		It("returns nil on a generic GetFile error", func() {
			gh := fakeFileGetter{err: errors.New("boom")}
			changes := calculateAliasChanges(gh, newFullOrgAliases(), "gardener", "ci-infra")
			Expect(changes).To(BeNil())
		})

		It("returns nil when the file cannot be parsed", func() {
			gh := fakeFileGetter{content: []byte("::: not yaml :::")}
			changes := calculateAliasChanges(gh, newFullOrgAliases(), "gardener", "ci-infra")
			Expect(changes).To(BeNil())
		})

		It("reports len(changes)=0 when repo and local config already match", func() {
			gh := fakeFileGetter{content: []byte("aliases:\n  team-a:\n  - alice\n  - bob\n")}
			changes := calculateAliasChanges(gh, localConfig("team-a", "alice", "bob"), "gardener", "ci-infra")
			Expect(changes).To(BeEmpty())
		})

		It("computes members to add and remove", func() {
			gh := fakeFileGetter{content: []byte("aliases:\n  team-a:\n  - alice\n  - carol\n")}
			// local has alice+bob; repo has alice+carol => add bob, remove carol
			changes := calculateAliasChanges(gh, localConfig("team-a", "alice", "bob"), "gardener", "ci-infra")
			Expect(changes).ToNot(BeEmpty())
			Expect(changes["team-a"].add).To(Equal(sets.New("bob")))
			Expect(changes["team-a"].remove).To(Equal(sets.New("carol")))
		})

		It("skips aliases that do not exist in the local config", func() {
			gh := fakeFileGetter{content: []byte("aliases:\n  unknown-team:\n  - alice\n")}
			changes := calculateAliasChanges(gh, localConfig("team-a", "alice"), "gardener", "ci-infra")
			Expect(changes).To(BeEmpty())
		})

		It("reports no change for an org that has no local teams", func() {
			// getConfig lazily creates an empty config for the org, so every
			// alias in the repo is skipped and nothing is reported as changed.
			gh := fakeFileGetter{content: []byte("aliases:\n  team-a:\n  - alice\n")}
			changes := calculateAliasChanges(gh, newFullOrgAliases(), "org-with-no-teams", "repo")
			Expect(changes).To(BeEmpty())
		})

		It("does not report a change when only casing/@ differ (normalization regression guard)", func() {
			// This is the end-to-end guard for the normalization contract: the
			// repo file uses mixed case and a leading @, while the local config
			// is built via addMembersFromTeams (which normalizes via NormLogin).
			// If either side stopped normalizing, this would report a spurious
			// remove+add and churn the file on every run.
			gh := fakeFileGetter{content: []byte("aliases:\n  Team-A:\n  - Alice\n  - \"@Bob\"\n")}

			f := newFullOrgAliases()
			addMembersFromTeams(f.getConfig("gardener"), map[string]org.Team{
				"Team-A": {
					Members:     []string{"Alice"},
					Maintainers: []string{"@Bob"},
				},
			}, "")

			changes := calculateAliasChanges(gh, f, "gardener", "ci-infra")
			Expect(changes).To(BeEmpty(), "casing/@ differences must not be treated as changes")
		})

		It("aggregates changes across multiple aliases in one repo", func() {
			gh := fakeFileGetter{content: []byte(
				"aliases:\n" +
					"  team-a:\n  - alice\n  - carol\n" + // differs: add bob, remove carol
					"  team-b:\n  - dave\n" + // matches exactly
					"  team-c:\n  - eve\n", // not in local config -> skipped
			)}

			f := newFullOrgAliases()
			cfg := f.getConfig("gardener")
			cfg.addMember("team-a", "alice")
			cfg.addMember("team-a", "bob")
			cfg.addMember("team-b", "dave")

			changes := calculateAliasChanges(gh, f, "gardener", "ci-infra")
			Expect(changes).ToNot(BeEmpty())
			Expect(changes["team-a"].add).To(Equal(sets.New("bob")))
			Expect(changes["team-a"].remove).To(Equal(sets.New("carol")))
			Expect(changes).ToNot(HaveKey("team-b"))
			Expect(changes).ToNot(HaveKey("team-c"))
		})
	})

	Describe("#writeChanges", func() {
		var path string

		writeFile := func(content string) {
			dir := GinkgoT().TempDir()
			path = filepath.Join(dir, "OWNERS_ALIASES")
			Expect(os.WriteFile(path, []byte(content), 0o644)).To(Succeed())
		}

		// readBack parses the file after writeChanges into the alias map.
		readBack := func() map[string][]string {
			raw, err := os.ReadFile(path)
			Expect(err).ToNot(HaveOccurred())
			var parsed ownersAliasesFile
			Expect(yaml4.Unmarshal(raw, &parsed)).To(Succeed())
			return parsed.Aliases
		}

		It("adds a member to an existing alias", func() {
			writeFile("aliases:\n  team-a:\n  - alice\n")
			err := writeChanges(path, map[string]change{
				"team-a": {add: sets.New("bob"), remove: sets.New[string]()},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(readBack()["team-a"]).To(ConsistOf("alice", "bob"))
		})

		It("removes a member from an existing alias", func() {
			writeFile("aliases:\n  team-a:\n  - alice\n  - bob\n")
			err := writeChanges(path, map[string]change{
				"team-a": {add: sets.New[string](), remove: sets.New("bob")},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(readBack()["team-a"]).To(ConsistOf("alice"))
		})

		It("both adds and removes members", func() {
			writeFile("aliases:\n  team-a:\n  - alice\n  - carol\n")
			err := writeChanges(path, map[string]change{
				"team-a": {add: sets.New("bob"), remove: sets.New("carol")},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(readBack()["team-a"]).To(ConsistOf("alice", "bob"))
		})

		It("warns and skips a change for an alias missing from the file", func() {
			writeFile("aliases:\n  team-a:\n  - alice\n")
			err := writeChanges(path, map[string]change{
				"missing-team": {add: sets.New("bob"), remove: sets.New[string]()},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(readBack()).ToNot(HaveKey("missing-team"))
			Expect(readBack()["team-a"]).To(ConsistOf("alice"))
		})

		It("returns an error when the file does not exist", func() {
			err := writeChanges(filepath.Join(GinkgoT().TempDir(), "does-not-exist"), map[string]change{})
			Expect(err).To(HaveOccurred())
		})

		It("returns an error for the unsupported {alias: {u1, u2}} map notation", func() {
			// The ownersAliasesFile type only models the string-array syntax;
			// a mapping value must be rejected rather than silently mangled.
			writeFile("aliases:\n  team-a:\n    alice: {}\n    bob: {}\n")
			err := writeChanges(path, map[string]change{
				"team-a": {add: sets.New("carol"), remove: sets.New[string]()},
			})
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("failed parsing file")))
		})

		It("appends a duplicate when asked to add a member that already exists", func() {
			// Documents current behavior: writeChanges does not dedupe on add.
			// In practice calculateAliasChanges only ever asks to add members
			// that are absent (set difference), so this path is not hit in the
			// normal flow, but the function itself does not guard against it.
			writeFile("aliases:\n  team-a:\n  - alice\n")
			err := writeChanges(path, map[string]change{
				"team-a": {add: sets.New("alice"), remove: sets.New[string]()},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(readBack()["team-a"]).To(Equal([]string{"alice", "alice"}))
		})
	})

	Describe("#writeChanges comment preservation", func() {
		// applyTo writes content to a temp file, applies the changes and returns
		// the resulting file as a string.
		applyTo := func(content string, changes map[string]change) string {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "OWNERS_ALIASES")
			Expect(os.WriteFile(path, []byte(content), 0o644)).To(Succeed())
			Expect(writeChanges(path, changes)).To(Succeed())
			raw, err := os.ReadFile(path)
			Expect(err).ToNot(HaveOccurred())
			return string(raw)
		}

		It("preserves a top-of-file header comment", func() {
			out := applyTo(
				"# managed by owners-aliases-bumper\naliases:\n  team-a:\n  - alice\n",
				map[string]change{"team-a": {add: sets.New("bob"), remove: sets.New[string]()}},
			)
			Expect(out).To(ContainSubstring("# managed by owners-aliases-bumper"))
			Expect(out).To(ContainSubstring("bob"))
		})

		It("preserves a comment attached to a team section", func() {
			out := applyTo(
				"aliases:\n  # the a-team\n  team-a:\n  - alice\n",
				map[string]change{"team-a": {add: sets.New("bob"), remove: sets.New[string]()}},
			)
			Expect(out).To(ContainSubstring("# the a-team"))
		})

		It("preserves a comment sitting between two teams", func() {
			out := applyTo(
				"aliases:\n  team-a:\n  - alice\n  # divider\n  team-b:\n  - bob\n",
				map[string]change{"team-b": {add: sets.New("carol"), remove: sets.New[string]()}},
			)
			Expect(out).To(ContainSubstring("# divider"))
			Expect(out).To(ContainSubstring("carol"))
		})

		It("preserves a footer comment", func() {
			out := applyTo(
				"aliases:\n  team-a:\n  - alice\n# footer\n",
				map[string]change{"team-a": {add: sets.New("bob"), remove: sets.New[string]()}},
			)
			Expect(out).To(ContainSubstring("# footer"))
		})

		It("preserves comments when only removing a member", func() {
			out := applyTo(
				"# header\naliases:\n  # the a-team\n  team-a:\n  - alice\n  - bob\n",
				map[string]change{"team-a": {add: sets.New[string](), remove: sets.New("bob")}},
			)
			Expect(out).To(ContainSubstring("# header"))
			Expect(out).To(ContainSubstring("# the a-team"))
			Expect(out).ToNot(MatchRegexp(`(?m)^\s*- bob\s*$`), "bob should have been removed")
		})

		It("preserves comments across changes to multiple teams", func() {
			out := applyTo(
				"# header\naliases:\n  # the a-team\n  team-a:\n  - alice\n  # the b-team\n  team-b:\n  - bob\n",
				map[string]change{
					"team-a": {add: sets.New("carol"), remove: sets.New[string]()},
					"team-b": {add: sets.New("dave"), remove: sets.New("bob")},
				},
			)
			Expect(out).To(ContainSubstring("# header"))
			Expect(out).To(ContainSubstring("# the a-team"))
			Expect(out).To(ContainSubstring("# the b-team"))
			Expect(out).To(ContainSubstring("carol"))
			Expect(out).To(ContainSubstring("dave"))
		})

		It("leaves the file untouched for a no-op change", func() {
			out := applyTo(
				"# header\naliases:\n  # the a-team\n  team-a:\n  - alice\n",
				map[string]change{"team-a": {add: sets.New[string](), remove: sets.New[string]()}},
			)
			Expect(out).To(ContainSubstring("# header"))
			Expect(out).To(ContainSubstring("# the a-team"))
			Expect(out).To(ContainSubstring("alice"))
		})

		// Inline comments (on the same line as a member or the team key) are
		// preserved everywhere except on the exact line being removed, where it
		// is impossible to keep them. Whitespace before the '#' may be
		// normalized, so assertions match the comment text, not exact spacing.
		It("preserves an inline comment on a surviving member when adding another", func() {
			out := applyTo(
				"aliases:\n  team-a:\n  - alice  # team lead\n  - bob\n",
				map[string]change{"team-a": {add: sets.New("carol"), remove: sets.New[string]()}},
			)
			Expect(out).To(ContainSubstring("# team lead"))
			Expect(out).To(ContainSubstring("carol"))
		})

		It("preserves an inline comment on a surviving member when removing a different member", func() {
			out := applyTo(
				"aliases:\n  team-a:\n  - alice  # team lead\n  - bob\n",
				map[string]change{"team-a": {add: sets.New[string](), remove: sets.New("bob")}},
			)
			Expect(out).To(ContainSubstring("# team lead"))
			Expect(out).ToNot(MatchRegexp(`(?m)^\s*- bob\s*$`), "bob should have been removed")
		})

		It("preserves an inline comment on the team key line", func() {
			out := applyTo(
				"aliases:\n  team-a:  # the a-team\n  - alice\n",
				map[string]change{"team-a": {add: sets.New("bob"), remove: sets.New[string]()}},
			)
			Expect(out).To(ContainSubstring("# the a-team"))
			Expect(out).To(ContainSubstring("bob"))
		})

		It("preserves an inline comment on a member for a no-op change", func() {
			out := applyTo(
				"aliases:\n  team-a:\n  - alice  # team lead\n",
				map[string]change{"team-a": {add: sets.New[string](), remove: sets.New[string]()}},
			)
			Expect(out).To(ContainSubstring("# team lead"))
		})

		It("drops only the inline comment on the member being removed", func() {
			out := applyTo(
				"aliases:\n  team-a:\n  - alice  # lead\n  - bob  # to be removed\n",
				map[string]change{"team-a": {add: sets.New[string](), remove: sets.New("bob")}},
			)
			// The comment on the surviving member stays...
			Expect(out).To(ContainSubstring("# lead"))
			// ...but the one anchored to the removed line is gone with it.
			Expect(out).ToNot(ContainSubstring("# to be removed"))
			Expect(out).ToNot(ContainSubstring("bob"))
		})
	})
})
