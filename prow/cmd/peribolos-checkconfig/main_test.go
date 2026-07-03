// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"reflect"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/prow/pkg/config/org"
	"sigs.k8s.io/prow/pkg/flagutil"
)

func strP(s string) *string { return &s }
func boolP(b bool) *bool    { return &b }
func privP(p org.Privacy) *org.Privacy {
	return &p
}

var _ = Describe("PeribolosCheckconfig", func() {
	DescribeTable("#parseArgs",
		func(args []string, expected *options) {
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			var actual options
			err := actual.parseArgs(flags, args)
			switch {
			case expected == nil:
				Expect(err).To(HaveOccurred(), "expected error, got none")
			default:
				Expect(err).ToNot(HaveOccurred())
				Expect(reflect.DeepEqual(*expected, actual)).To(BeTrue(),
					"got incorrect options: %s", cmp.Diff(actual, *expected, cmp.AllowUnexported(options{}, flagutil.Strings{})))
			}
		},
		Entry("missing --config",
			[]string{},
			nil),
		Entry("--minAdmins too low",
			[]string{"--config-path=foo", "--min-admins=1"},
			nil),
		Entry("bad --log-level",
			[]string{"--config-path=foo", "--log-level=nonsense"},
			nil),
		Entry("minimal",
			[]string{"--config-path=foo"},
			&options{
				config:         "foo",
				minAdmins:      defaultMinAdmins,
				requiredAdmins: flagutil.NewStrings(),
				logLevel:       "info",
			}),
		Entry("minimal admins",
			[]string{"--config-path=foo", "--min-admins=2"},
			&options{
				config:         "foo",
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
				logLevel:       "info",
			}),
		Entry("required admins",
			[]string{"--config-path=foo", "--required-admins=alice", "--required-admins=bob"},
			&options{
				config:         "foo",
				minAdmins:      defaultMinAdmins,
				requiredAdmins: flagutil.NewStringsBeenSet("alice", "bob"),
				logLevel:       "info",
			}),
		Entry("debug log level",
			[]string{"--config-path=foo", "--log-level=debug"},
			&options{
				config:         "foo",
				minAdmins:      defaultMinAdmins,
				requiredAdmins: flagutil.NewStrings(),
				logLevel:       "debug",
			}),
	)

	closed := org.Closed
	secret := org.Secret

	DescribeTable("#checkOrg",
		func(opt options, orgName string, orgConfig org.Config, expectError bool) {
			err := checkOrg(&opt, orgName, orgConfig)
			if expectError {
				Expect(err).To(HaveOccurred(), "expected error, got none")
			} else {
				Expect(err).ToNot(HaveOccurred(), "unexpected error")
			}
		},
		Entry("happy path",
			options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings("alice"),
			},
			"myorg",
			org.Config{
				Admins:  []string{"alice", "bob"},
				Members: []string{"carol"},
			},
			false),
		Entry("too few admins",
			options{
				minAdmins:      3,
				requiredAdmins: flagutil.NewStrings(),
			},
			"myorg",
			org.Config{
				Admins: []string{"alice", "bob"},
			},
			true),
		Entry("missing required admin",
			options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings("alice", "missing"),
			},
			"myorg",
			org.Config{
				Admins: []string{"alice", "bob"},
			},
			true),
		Entry("user in both admins and members",
			options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			"myorg",
			org.Config{
				Admins:  []string{"alice", "bob"},
				Members: []string{"alice"},
			},
			true),
		Entry("user in both admins and members, case differs",
			options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			"myorg",
			org.Config{
				Admins:  []string{"Alice", "bob"},
				Members: []string{"alice"},
			},
			true),
		Entry("required-admins matches case-insensitively is not enough — currently strict",
			options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings("alice"),
			},
			"myorg",
			org.Config{
				// "Alice" != "alice" for the required-admins check (matches peribolos behavior)
				Admins: []string{"Alice", "bob"},
			},
			true),
		Entry("team member is not an org member",
			options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			"myorg",
			org.Config{
				Admins:  []string{"alice", "bob"},
				Members: []string{"carol"},
				Teams: map[string]org.Team{
					"team-a": {
						Members: []string{"dave"},
					},
				},
			},
			true),
		Entry("team maintainer is not an org member",
			options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			"myorg",
			org.Config{
				Admins: []string{"alice", "bob"},
				Teams: map[string]org.Team{
					"team-a": {
						Maintainers: []string{"eve"},
					},
				},
			},
			true),
		Entry("team member is an admin — ok",
			options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			"myorg",
			org.Config{
				Admins: []string{"alice", "bob"},
				Teams: map[string]org.Team{
					"team-a": {
						Maintainers: []string{"alice"},
						Members:     []string{"bob"},
					},
				},
			},
			false),
		Entry("child team member is not an org member",
			options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			"myorg",
			org.Config{
				Admins: []string{"alice", "bob"},
				Teams: map[string]org.Team{
					"parent": {
						Maintainers: []string{"alice"},
						TeamMetadata: org.TeamMetadata{
							Privacy: privP(closed),
						},
						Children: map[string]org.Team{
							"child": {
								Members: []string{"stranger"},
								TeamMetadata: org.TeamMetadata{
									Privacy: privP(closed),
								},
							},
						},
					},
				},
			},
			true),
		Entry("duplicate team name",
			options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			"myorg",
			org.Config{
				Admins: []string{"alice", "bob"},
				Teams: map[string]org.Team{
					"team-a": {
						Maintainers: []string{"alice"},
					},
					"team-b": {
						Maintainers: []string{"alice"},
						Previously:  []string{"team-a"},
					},
				},
			},
			true),
		Entry("team with parent must be closed",
			options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			"myorg",
			org.Config{
				Admins: []string{"alice", "bob"},
				Teams: map[string]org.Team{
					"parent": {
						Maintainers: []string{"alice"},
						TeamMetadata: org.TeamMetadata{
							Privacy: privP(closed),
						},
						Children: map[string]org.Team{
							"child": {
								Maintainers: []string{"alice"},
								TeamMetadata: org.TeamMetadata{
									Privacy: privP(secret),
								},
							},
						},
					},
				},
			},
			true),
		Entry("team with children privacy unset — allowed (defaults handled by peribolos)",
			options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			"myorg",
			org.Config{
				Admins: []string{"alice", "bob"},
				Teams: map[string]org.Team{
					"parent": {
						Maintainers: []string{"alice"},
						// Privacy unset: peribolos will set it to closed at apply time,
						// so we do not flag it here.
						Children: map[string]org.Team{
							"child": {
								Maintainers: []string{"alice"},
							},
						},
					},
				},
			},
			false),
		Entry("user is both team member and maintainer",
			options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			"myorg",
			org.Config{
				Admins: []string{"alice", "bob"},
				Teams: map[string]org.Team{
					"team-a": {
						Maintainers: []string{"alice"},
						Members:     []string{"alice"},
					},
				},
			},
			true),
		Entry("user is both team member and maintainer, case differs",
			options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			"myorg",
			org.Config{
				Admins: []string{"alice", "bob"},
				Teams: map[string]org.Team{
					"team-a": {
						Maintainers: []string{"Alice"},
						Members:     []string{"alice"},
					},
				},
			},
			true),
		Entry("user is member of one team and maintainer of another — allowed",
			options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			"myorg",
			org.Config{
				Admins: []string{"alice", "bob"},
				Teams: map[string]org.Team{
					"team-a": {
						Maintainers: []string{"alice"},
					},
					"team-b": {
						Maintainers: []string{"bob"},
						Members:     []string{"alice"},
					},
				},
			},
			false),
		Entry("user is maintainer of parent and member of child — allowed",
			options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			"myorg",
			org.Config{
				Admins: []string{"alice", "bob"},
				Teams: map[string]org.Team{
					"parent": {
						Maintainers: []string{"alice"},
						Children: map[string]org.Team{
							"child": {
								Members: []string{"alice"},
							},
						},
					},
				},
			},
			false),
		Entry("duplicate repo names case-insensitively",
			options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			"myorg",
			org.Config{
				Admins: []string{"alice", "bob"},
				Repos: map[string]org.Repo{
					"repo": {Description: strP("desc")},
					"Repo": {Description: strP("desc")},
				},
			},
			true),
		Entry("repo archived: false is rejected",
			options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			"myorg",
			org.Config{
				Admins: []string{"alice", "bob"},
				Repos: map[string]org.Repo{
					"repo": {Archived: boolP(false)},
				},
			},
			true),
		Entry("repo archived: true is allowed",
			options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			"myorg",
			org.Config{
				Admins: []string{"alice", "bob"},
				Repos: map[string]org.Repo{
					"repo": {Archived: boolP(true)},
				},
			},
			false),
	)

	Describe("#validateRepos", func() {
		description := "cool repo"

		DescribeTable("validates repo configs",
			func(config map[string]org.Repo, expectError bool) {
				err := validateRepos("myorg", config)
				if expectError {
					Expect(err).To(HaveOccurred(), "expected error, got none")
				} else {
					Expect(err).ToNot(HaveOccurred(), "unexpected error")
				}
			},
			Entry("handles nil map",
				nil,
				false),
			Entry("handles empty map",
				map[string]org.Repo{},
				false),
			Entry("handles valid config",
				map[string]org.Repo{
					"repo": {Description: &description},
				},
				false),
			Entry("finds repo names duplicate when normalized",
				map[string]org.Repo{
					"repo": {Description: &description},
					"Repo": {Description: &description},
				},
				true),
			Entry("finds name conflict between previous and current names",
				map[string]org.Repo{
					"repo":     {Previously: []string{"conflict"}},
					"conflict": {Description: &description},
				},
				true),
			Entry("finds name conflict between two previous names",
				map[string]org.Repo{
					"repo":         {Previously: []string{"conflict"}},
					"another-repo": {Previously: []string{"conflict"}},
				},
				true),
			Entry("flags archived: false",
				map[string]org.Repo{
					"repo": {Archived: boolP(false)},
				},
				true),
			Entry("allows archived: true",
				map[string]org.Repo{
					"repo": {Archived: boolP(true)},
				},
				false),
		)
	})
})
