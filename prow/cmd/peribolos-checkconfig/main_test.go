// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"

	"sigs.k8s.io/prow/pkg/config/org"
	"sigs.k8s.io/prow/pkg/flagutil"
)

func TestOptions(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		expected *options
	}{
		{
			name: "missing --config",
			args: []string{},
		},
		{
			name: "--minAdmins too low",
			args: []string{"--config-path=foo", "--min-admins=1"},
		},
		{
			name: "bad --log-level",
			args: []string{"--config-path=foo", "--log-level=nonsense"},
		},
		{
			name: "minimal",
			args: []string{"--config-path=foo"},
			expected: &options{
				config:         "foo",
				minAdmins:      defaultMinAdmins,
				requiredAdmins: flagutil.NewStrings(),
				logLevel:       "info",
			},
		},
		{
			name: "minimal admins",
			args: []string{"--config-path=foo", "--min-admins=2"},
			expected: &options{
				config:         "foo",
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
				logLevel:       "info",
			},
		},
		{
			name: "required admins",
			args: []string{"--config-path=foo", "--required-admins=alice", "--required-admins=bob"},
			expected: &options{
				config:         "foo",
				minAdmins:      defaultMinAdmins,
				requiredAdmins: flagutil.NewStringsBeenSet("alice", "bob"),
				logLevel:       "info",
			},
		},
		{
			name: "debug log level",
			args: []string{"--config-path=foo", "--log-level=debug"},
			expected: &options{
				config:         "foo",
				minAdmins:      defaultMinAdmins,
				requiredAdmins: flagutil.NewStrings(),
				logLevel:       "debug",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			flags := flag.NewFlagSet(tc.name, flag.ContinueOnError)
			var actual options
			err := actual.parseArgs(flags, tc.args)
			switch {
			case err == nil && tc.expected == nil:
				t.Errorf("%s: failed to return an error", tc.name)
			case err != nil && tc.expected != nil:
				t.Errorf("%s: unexpected error: %v", tc.name, err)
			case tc.expected != nil && !reflect.DeepEqual(*tc.expected, actual):
				t.Errorf("%s: got incorrect options: %v", tc.name, cmp.Diff(actual, *tc.expected, cmp.AllowUnexported(options{}, flagutil.Strings{})))
			}
		})
	}
}

func strP(s string) *string { return &s }
func boolP(b bool) *bool    { return &b }
func privP(p org.Privacy) *org.Privacy {
	return &p
}

func TestCheckOrg(t *testing.T) {
	closed := org.Closed
	secret := org.Secret

	cases := []struct {
		name        string
		opt         options
		orgName     string
		orgConfig   org.Config
		expectError bool
	}{
		{
			name: "happy path",
			opt: options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings("alice"),
			},
			orgName: "myorg",
			orgConfig: org.Config{
				Admins:  []string{"alice", "bob"},
				Members: []string{"carol"},
			},
		},
		{
			name: "too few admins",
			opt: options{
				minAdmins:      3,
				requiredAdmins: flagutil.NewStrings(),
			},
			orgName: "myorg",
			orgConfig: org.Config{
				Admins: []string{"alice", "bob"},
			},
			expectError: true,
		},
		{
			name: "missing required admin",
			opt: options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings("alice", "missing"),
			},
			orgName: "myorg",
			orgConfig: org.Config{
				Admins: []string{"alice", "bob"},
			},
			expectError: true,
		},
		{
			name: "user in both admins and members",
			opt: options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			orgName: "myorg",
			orgConfig: org.Config{
				Admins:  []string{"alice", "bob"},
				Members: []string{"alice"},
			},
			expectError: true,
		},
		{
			name: "user in both admins and members, case differs",
			opt: options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			orgName: "myorg",
			orgConfig: org.Config{
				Admins:  []string{"Alice", "bob"},
				Members: []string{"alice"},
			},
			expectError: true,
		},
		{
			name: "required-admins matches case-insensitively is not enough — currently strict",
			opt: options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings("alice"),
			},
			orgName: "myorg",
			orgConfig: org.Config{
				// "Alice" != "alice" for the required-admins check (matches peribolos behavior)
				Admins: []string{"Alice", "bob"},
			},
			expectError: true,
		},
		{
			name: "team member is not an org member",
			opt: options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			orgName: "myorg",
			orgConfig: org.Config{
				Admins:  []string{"alice", "bob"},
				Members: []string{"carol"},
				Teams: map[string]org.Team{
					"team-a": {
						Members: []string{"dave"},
					},
				},
			},
			expectError: true,
		},
		{
			name: "team maintainer is not an org member",
			opt: options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			orgName: "myorg",
			orgConfig: org.Config{
				Admins: []string{"alice", "bob"},
				Teams: map[string]org.Team{
					"team-a": {
						Maintainers: []string{"eve"},
					},
				},
			},
			expectError: true,
		},
		{
			name: "team member is an admin — ok",
			opt: options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			orgName: "myorg",
			orgConfig: org.Config{
				Admins: []string{"alice", "bob"},
				Teams: map[string]org.Team{
					"team-a": {
						Maintainers: []string{"alice"},
						Members:     []string{"bob"},
					},
				},
			},
		},
		{
			name: "child team member is not an org member",
			opt: options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			orgName: "myorg",
			orgConfig: org.Config{
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
			expectError: true,
		},
		{
			name: "duplicate team name",
			opt: options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			orgName: "myorg",
			orgConfig: org.Config{
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
			expectError: true,
		},
		{
			name: "team with parent must be closed",
			opt: options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			orgName: "myorg",
			orgConfig: org.Config{
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
			expectError: true,
		},
		{
			name: "team with children privacy unset — allowed (defaults handled by peribolos)",
			opt: options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			orgName: "myorg",
			orgConfig: org.Config{
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
		},
		{
			name: "user is both team member and maintainer",
			opt: options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			orgName: "myorg",
			orgConfig: org.Config{
				Admins: []string{"alice", "bob"},
				Teams: map[string]org.Team{
					"team-a": {
						Maintainers: []string{"alice"},
						Members:     []string{"alice"},
					},
				},
			},
			expectError: true,
		},
		{
			name: "user is both team member and maintainer, case differs",
			opt: options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			orgName: "myorg",
			orgConfig: org.Config{
				Admins: []string{"alice", "bob"},
				Teams: map[string]org.Team{
					"team-a": {
						Maintainers: []string{"Alice"},
						Members:     []string{"alice"},
					},
				},
			},
			expectError: true,
		},
		{
			name: "user is member of one team and maintainer of another — allowed",
			opt: options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			orgName: "myorg",
			orgConfig: org.Config{
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
		},
		{
			name: "user is maintainer of parent and member of child — allowed",
			opt: options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			orgName: "myorg",
			orgConfig: org.Config{
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
		},
		{
			name: "duplicate repo names case-insensitively",
			opt: options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			orgName: "myorg",
			orgConfig: org.Config{
				Admins: []string{"alice", "bob"},
				Repos: map[string]org.Repo{
					"repo": {Description: strP("desc")},
					"Repo": {Description: strP("desc")},
				},
			},
			expectError: true,
		},
		{
			name: "repo archived: false is rejected",
			opt: options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			orgName: "myorg",
			orgConfig: org.Config{
				Admins: []string{"alice", "bob"},
				Repos: map[string]org.Repo{
					"repo": {Archived: boolP(false)},
				},
			},
			expectError: true,
		},
		{
			name: "repo archived: true is allowed",
			opt: options{
				minAdmins:      2,
				requiredAdmins: flagutil.NewStrings(),
			},
			orgName: "myorg",
			orgConfig: org.Config{
				Admins: []string{"alice", "bob"},
				Repos: map[string]org.Repo{
					"repo": {Archived: boolP(true)},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkOrg(&tc.opt, tc.orgName, tc.orgConfig)
			if err == nil && tc.expectError {
				t.Errorf("%s: expected error, got none", tc.name)
			} else if err != nil && !tc.expectError {
				t.Errorf("%s: unexpected error: %v", tc.name, err)
			}
		})
	}
}

func TestValidateRepos(t *testing.T) {
	description := "cool repo"
	testCases := []struct {
		description string
		config      map[string]org.Repo
		expectError bool
	}{
		{
			description: "handles nil map",
		},
		{
			description: "handles empty map",
			config:      map[string]org.Repo{},
		},
		{
			description: "handles valid config",
			config: map[string]org.Repo{
				"repo": {Description: &description},
			},
		},
		{
			description: "finds repo names duplicate when normalized",
			config: map[string]org.Repo{
				"repo": {Description: &description},
				"Repo": {Description: &description},
			},
			expectError: true,
		},
		{
			description: "finds name conflict between previous and current names",
			config: map[string]org.Repo{
				"repo":     {Previously: []string{"conflict"}},
				"conflict": {Description: &description},
			},
			expectError: true,
		},
		{
			description: "finds name conflict between two previous names",
			config: map[string]org.Repo{
				"repo":         {Previously: []string{"conflict"}},
				"another-repo": {Previously: []string{"conflict"}},
			},
			expectError: true,
		},
		{
			description: "flags archived: false",
			config: map[string]org.Repo{
				"repo": {Archived: boolP(false)},
			},
			expectError: true,
		},
		{
			description: "allows archived: true",
			config: map[string]org.Repo{
				"repo": {Archived: boolP(true)},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			err := validateRepos("myorg", tc.config)
			if err == nil && tc.expectError {
				t.Errorf("%s: expected error, got none", tc.description)
			} else if err != nil && !tc.expectError {
				t.Errorf("%s: unexpected error: %v", tc.description, err)
			}
		})
	}
}
