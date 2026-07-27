// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/prow/pkg/flagutil"
)

var _ = Describe("Options", func() {
	// Only the fields parseArgs is responsible for are asserted. ghOpts is
	// populated with internal defaults by flagutil.GitHubOptions.AddFlags and
	// is not meaningfully comparable via a struct literal.
	DescribeTable("#parseArgs",
		func(args []string, expected *options) {
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			var actual options
			err := actual.parseArgs(flags, args)
			if expected == nil {
				Expect(err).To(HaveOccurred(), "expected error, got none")
				return
			}
			Expect(err).ToNot(HaveOccurred())
			Expect(actual.peribolosConfig).To(Equal(expected.peribolosConfig))
			Expect(actual.applyChanges).To(Equal(expected.applyChanges))
			Expect(actual.logLevel).To(Equal(expected.logLevel))
			Expect(actual.skipRepos.Strings()).To(Equal(expected.skipRepos.Strings()))
		},
		Entry("missing --peribolos-conf",
			[]string{},
			nil),
		Entry("bad --log-level",
			[]string{"--peribolos-conf=foo", "--log-level=nonsense"},
			nil),
		Entry("minimal",
			[]string{"--peribolos-conf=foo"},
			&options{
				peribolosConfig: "foo",
				skipRepos:       flagutil.NewStrings(),
				logLevel:        "info",
			}),
		Entry("confirm flag",
			[]string{"--peribolos-conf=foo", "--confirm"},
			&options{
				peribolosConfig: "foo",
				applyChanges:    true,
				skipRepos:       flagutil.NewStrings(),
				logLevel:        "info",
			}),
		Entry("skip repos",
			[]string{"--peribolos-conf=foo", "--skip-repos=org/a", "--skip-repos=org/b"},
			&options{
				peribolosConfig: "foo",
				skipRepos:       flagutil.NewStringsBeenSet("org/a", "org/b"),
				logLevel:        "info",
			}),
		Entry("debug log level",
			[]string{"--peribolos-conf=foo", "--log-level=debug"},
			&options{
				peribolosConfig: "foo",
				skipRepos:       flagutil.NewStrings(),
				logLevel:        "debug",
			}),
	)

	Describe("#buildPRConfig", func() {
		It("uses the defaults when nothing is overridden", func() {
			o := options{prBranch: defaultPRBranch, prTitle: defaultPRTitle, commitTitle: defaultCommitTitle}
			cfg, err := o.buildPRConfig()
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg).To(Equal(prConfig{
				branch:      defaultPRBranch,
				commitTitle: defaultCommitTitle,
				commitBody:  defaultCommitBody,
				prTitle:     defaultPRTitle,
				prBody:      defaultPRBody,
			}))
		})

		It("prefers inline bodies over the defaults", func() {
			o := options{commitBody: "my commit body", prBody: "my pr body"}
			cfg, err := o.buildPRConfig()
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.commitBody).To(Equal("my commit body"))
			Expect(cfg.prBody).To(Equal("my pr body"))
		})

		It("reads bodies from files", func() {
			dir := GinkgoT().TempDir()
			commitPath := filepath.Join(dir, "commit.txt")
			prPath := filepath.Join(dir, "pr.txt")
			Expect(os.WriteFile(commitPath, []byte("file commit body"), 0o600)).To(Succeed())
			Expect(os.WriteFile(prPath, []byte("file pr body"), 0o600)).To(Succeed())

			o := options{commitBodyFile: commitPath, prBodyFile: prPath}
			cfg, err := o.buildPRConfig()
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.commitBody).To(Equal("file commit body"))
			Expect(cfg.prBody).To(Equal("file pr body"))
		})

		It("errors when both inline and file are set for the commit body", func() {
			o := options{commitBody: "inline", commitBodyFile: "some/path"}
			_, err := o.buildPRConfig()
			Expect(err).To(MatchError(ContainSubstring("--commit-body and --commit-body-file are mutually exclusive")))
		})

		It("errors when both inline and file are set for the PR body", func() {
			o := options{prBody: "inline", prBodyFile: "some/path"}
			_, err := o.buildPRConfig()
			Expect(err).To(MatchError(ContainSubstring("--pr-body and --pr-body-file are mutually exclusive")))
		})

		It("errors when a body file cannot be read", func() {
			o := options{prBodyFile: "/nonexistent/does-not-exist"}
			_, err := o.buildPRConfig()
			Expect(err).To(MatchError(ContainSubstring("unable to read --pr-body-file")))
		})
	})
})
