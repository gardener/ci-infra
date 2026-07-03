// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/yaml"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"sigs.k8s.io/prow/pkg/config/org"
	"sigs.k8s.io/prow/pkg/flagutil"
	"sigs.k8s.io/prow/pkg/github"
	"sigs.k8s.io/prow/pkg/logrusutil"
)

const (
	defaultMinAdmins = 5
)

type options struct {
	config         string
	minAdmins      int
	requiredAdmins flagutil.Strings

	logLevel string
}

func parseOptions() options {
	var o options
	if err := o.parseArgs(flag.CommandLine, os.Args[1:]); err != nil {
		logrus.Fatalf("Invalid flags: %v", err)
	}
	return o
}

func (o *options) parseArgs(flags *flag.FlagSet, args []string) error {
	o.requiredAdmins = flagutil.NewStrings()
	flags.Var(&o.requiredAdmins, "required-admins", "Ensure config specifies these users as admins")
	flags.StringVar(&o.config, "config-path", "", "Path to org config.yaml")
	flags.IntVar(&o.minAdmins, "min-admins", defaultMinAdmins, "Ensure config specifies at least this many admins")

	flags.StringVar(&o.logLevel, "log-level", logrus.InfoLevel.String(), fmt.Sprintf("Logging level, one of %v", logrus.AllLevels))
	if err := flags.Parse(args); err != nil {
		return err
	}
	level, err := logrus.ParseLevel(o.logLevel)
	if err != nil {
		return fmt.Errorf("--log-level invalid: %w", err)
	}
	logrus.SetLevel(level)
	logrus.SetReportCaller(level >= logrus.DebugLevel)

	if o.minAdmins < 2 {
		return fmt.Errorf("--min-admins=%d must be at least 2", o.minAdmins)
	}

	if o.config == "" {
		return errors.New("--config-path required")
	}
	return nil
}

// will check the configuration of peribolos
// performs the following checks:
// - yaml syntax check
// - checks whether the config is correct (correct keys etc.)
// - min-admins per org
// - required-admins present in each org
// - no user is both admin and member (post NormLogin)
// - all team members/maintainers are also org members/admins (recursive)
// - team names are unique within an org (including Previously names)
// - repo names are unique within an org (case-insensitive, including Previously)
// - nested teams (with parent or children) have privacy: closed
// - no repo config has archived: false (unarchiving is unsupported by the GH API)
func main() {
	logrusutil.ComponentInit()

	o := parseOptions()

	// check yaml
	cfg := checkYamlSyntax(&o)

	var errs []error
	for name, orgcfg := range cfg.Orgs {
		if err := checkOrg(&o, name, orgcfg); err != nil {
			errs = append(errs, err)
		}
	}

	if err := utilerrors.NewAggregate(errs); err != nil {
		logrus.WithError(err).Fatal("Configuration check failed.")
	}

	logrus.Info("Finished checking configuration.")
}

func checkYamlSyntax(o *options) org.FullConfig {
	raw, err := os.ReadFile(o.config)
	if err != nil {
		logrus.WithError(err).Fatal("Could not read --config-path file")
	}

	var cfg org.FullConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		logrus.WithError(err).Fatal("Failed to load configuration")
	}
	return cfg
}

func checkOrg(opt *options, orgName string, orgConfig org.Config) error {
	var errs []error

	// check min admins
	wantAdmins := sets.New[string](orgConfig.Admins...)
	if n := len(wantAdmins); n < opt.minAdmins {
		errs = append(errs, fmt.Errorf("%s must specify at least %d admins, only found %d", orgName, opt.minAdmins, n))
	}

	// check required admins
	var missing []string
	for _, r := range opt.requiredAdmins.Strings() {
		if !wantAdmins.Has(r) {
			missing = append(missing, r)
		}
	}
	if len(missing) > 0 {
		errs = append(errs, fmt.Errorf("%s must specify %v as admins, missing %v", orgName, opt.requiredAdmins, missing))
	}

	// no user in both admins and members (compare normalized logins, since GH treats logins case-insensitively)
	wantMembers := normalize(sets.New[string](orgConfig.Members...))
	normAdmins := normalize(wantAdmins)
	if both := normAdmins.Intersection(wantMembers); len(both) > 0 {
		errs = append(errs, fmt.Errorf("%s: users listed as both admin and member: %s", orgName, strings.Join(sets.List(both), ", ")))
	}
	orgUsers := normAdmins.Union(wantMembers)

	// team-level checks: collect team members recursively, verify uniqueness of team names
	state := &teamWalkState{
		members:        sets.Set[string]{},
		names:          sets.Set[string]{},
		duplicateNames: sets.Set[string]{},
	}
	walkTeams(orgConfig.Teams, false, state)

	if outside := normalize(state.members).Difference(orgUsers); len(outside) > 0 {
		errs = append(errs, fmt.Errorf("%s: team members/maintainers must also be org members: %s", orgName, strings.Join(sets.List(outside), ", ")))
	}

	if n := len(state.duplicateNames); n > 0 {
		errs = append(errs, fmt.Errorf("%s: team names must be unique (including previous names), %d duplicated: %s", orgName, n, strings.Join(sets.List(state.duplicateNames), ", ")))
	}

	if len(state.nestedNotClosed) > 0 {
		errs = append(errs, fmt.Errorf("%s: nested teams must have privacy: closed: %s", orgName, strings.Join(state.nestedNotClosed, ", ")))
	}

	if len(state.dualRoleTeams) > 0 {
		errs = append(errs, fmt.Errorf("%s: users listed as both member and maintainer of the same team: %s", orgName, strings.Join(state.dualRoleTeams, "; ")))
	}

	// repo-level checks
	if err := validateRepos(orgName, orgConfig.Repos); err != nil {
		errs = append(errs, err)
	}

	return utilerrors.NewAggregate(errs)
}

// validateRepos checks:
// - repo names (including Previously) are unique within an org, case-insensitively
// - no repo is configured with archived: false (GH API cannot unarchive)
func validateRepos(orgName string, repos map[string]org.Repo) error {
	seen := map[string]string{}
	var dups []string
	var unarchive []string

	for wantName, repo := range repos {
		if repo.Archived != nil && !*repo.Archived {
			unarchive = append(unarchive, wantName)
		}
		toCheck := append([]string{wantName}, repo.Previously...)
		for _, name := range toCheck {
			normName := strings.ToLower(name)
			if seenName, have := seen[normName]; have {
				dups = append(dups, fmt.Sprintf("%s/%s", seenName, name))
			}
		}
		for _, name := range toCheck {
			seen[strings.ToLower(name)] = name
		}
	}

	var errs []error
	if len(dups) > 0 {
		errs = append(errs, fmt.Errorf("%s: found duplicate repo names (GitHub repo names are case-insensitive): %s", orgName, strings.Join(dups, ", ")))
	}
	if len(unarchive) > 0 {
		errs = append(errs, fmt.Errorf("%s: repos configured with archived: false — GitHub does not support unarchiving via the API: %s", orgName, strings.Join(unarchive, ", ")))
	}
	return utilerrors.NewAggregate(errs)
}

func normalize(s sets.Set[string]) sets.Set[string] {
	out := sets.Set[string]{}
	for i := range s {
		out.Insert(github.NormLogin(i))
	}
	return out
}

// teamWalkState accumulates findings while walkTeams recurses through a team tree.
type teamWalkState struct {
	members         sets.Set[string] // all team members and maintainers (raw logins)
	names           sets.Set[string] // team names seen so far (including Previously names)
	duplicateNames  sets.Set[string] // team names seen more than once
	nestedNotClosed []string         // "<team> (privacy=<p>)" for nested teams that aren't closed
	dualRoleTeams   []string         // "<team>: <users>" for teams where a user is both member and maintainer
}

// walkTeams recurses through a team tree, populating state with any findings.
// parentIsNested is true when the caller is already a nested team (so its
// children are transitively nested regardless of whether they have children).
func walkTeams(teams map[string]org.Team, parentIsNested bool, state *teamWalkState) {
	for name, team := range teams {
		state.members.Insert(team.Members...)
		state.members.Insert(team.Maintainers...)

		// A user must not be both a member and a maintainer of the same team.
		// Peribolos rejects this at apply time (see cmd/peribolos/main.go
		// configureMembers), but only when --fix-team-members is set — so
		// bad configs can accumulate silently. Check statically here.
		teamMemberSet := normalize(sets.New[string](team.Members...))
		teamMaintainerSet := normalize(sets.New[string](team.Maintainers...))
		if both := teamMemberSet.Intersection(teamMaintainerSet); len(both) > 0 {
			state.dualRoleTeams = append(state.dualRoleTeams, fmt.Sprintf("%s: %s", name, strings.Join(sets.List(both), ", ")))
		}

		if state.names.Has(name) {
			state.duplicateNames.Insert(name)
		}
		state.names.Insert(name)
		for _, n := range team.Previously {
			if state.names.Has(n) {
				state.duplicateNames.Insert(n)
			}
			state.names.Insert(n)
		}

		// Nested teams (either they have a parent, or they have children) must be "closed" per GitHub.
		isNested := parentIsNested || len(team.Children) > 0
		if isNested {
			if team.Privacy != nil && *team.Privacy != org.Closed {
				state.nestedNotClosed = append(state.nestedNotClosed, fmt.Sprintf("%s (privacy=%s)", name, *team.Privacy))
			}
		}

		walkTeams(team.Children, true, state)
	}
}
