// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path"

	"github.com/Masterminds/semver"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/prow/prow/git/localgit"
	gitv2 "sigs.k8s.io/prow/prow/git/v2"
)

var _ = Describe("ReleaseHandler", func() {
	DescribeTable("#classifyVersionChange",
		func(currentVersion, previousVersion string, versionChange versionChange, errorMsg string) {
			cv := semver.MustParse(currentVersion)
			pv := semver.MustParse(previousVersion)
			v, err := classifyVersionChange(cv, pv)
			if errorMsg == "" {
				Expect(err).ToNot(HaveOccurred())
				Expect(v.currentVersion).To(Equal(cv))
				Expect(v.previousVersion).To(Equal(pv))
				Expect(v.versionChange).To(Equal(versionChange))
			} else {
				Expect(err).To(MatchError(ContainSubstring(errorMsg)))
			}
		},
		Entry("new release", "v1.75.0", "v1.75.0-dev", newRelease, ""),
		Entry("new release", "v1.75.1", "v1.75.0", newRelease, ""),
		Entry("new release", "v1.76.0", "v1.75.0", newRelease, ""),
		Entry("next dev cycle", "v1.75.1-dev", "v1.75.0", nextDevCycle, ""),
		Entry("next dev cycle", "v1.76.0-dev", "v1.75.0", nextDevCycle, ""),
		Entry("new major minor version", "v1.76.0-dev", "v1.75.0-dev", prepareNewMajorMinorVersion, ""),
		Entry("new major minor version", "v1.77.0-dev", "v1.75.0-dev", prepareNewMajorMinorVersion, ""),
		Entry("new patch version", "v1.75.1-dev", "v1.75.0-dev", prepareNewPatchVersion, ""),
		Entry("new patch version", "v1.75.2-dev", "v1.75.0-dev", prepareNewPatchVersion, ""),
		Entry("no change", "v1.75.0-dev", "v1.75.0-dev", undefinedVersionChange, "version does not increase"),
		Entry("downgrade", "v1.75.0-dev", "v1.76.0-dev", undefinedVersionChange, "version does not increase"),
	)

	Describe("#getSemverFromFile", func() {
		const (
			validVersionFile              = "valid-version"
			validDevVersionFile           = "valid-dev-version"
			emptyVersionFile              = "empty-version"
			invalidVersionFile            = "invalid-version"
			invalidDevVersionFile         = "invalid-dev-version"
			validVersionWithEmptyLineFile = "valid-version-empty-line"
			validVersionInSecondLineFile  = "valid-version-in-second-line"

			validVersionContent              = `v1.75.0`
			validDevVersionContent           = `v1.75.0-dev`
			emptyVersionContent              = ``
			invalidVersionContent            = `foobar1.75.0`
			invalidDevVersionContent         = `v1.75.0.0.0.0-dev`
			validVersionWithEmptyLineContent = `v1.75.0

`
			validVersionInSecondLineContent = `foobar
v1.75.0`
		)

		var (
			testFileNames   map[string]string
			testFileContent map[string]string
		)

		BeforeEach(func() {
			By("Create test files")
			testFileNames = map[string]string{}
			testFileContent = map[string]string{
				validVersionFile:              validVersionContent,
				validDevVersionFile:           validDevVersionContent,
				emptyVersionFile:              emptyVersionContent,
				invalidVersionFile:            invalidVersionContent,
				invalidDevVersionFile:         invalidDevVersionContent,
				validVersionWithEmptyLineFile: validVersionWithEmptyLineContent,
				validVersionInSecondLineFile:  validVersionInSecondLineContent,
			}
			for f, c := range testFileContent {
				file, err := os.CreateTemp("", f)
				Expect(err).NotTo(HaveOccurred())
				testFileNames[f] = file.Name()
				_, err = file.WriteString(c)
				Expect(err).NotTo(HaveOccurred())
			}

			DeferCleanup(func() {
				By("Remove test files")
				for _, f := range testFileNames {
					err := os.Remove(f)
					Expect(err).NotTo(HaveOccurred())
				}
			})
		})

		test := func(file, errorMsg string) {
			version, err := getSemverFromFile(testFileNames[file])
			if errorMsg == "" {
				Expect(err).ToNot(HaveOccurred())
				Expect(version).ToNot(BeNil())
			} else {
				Expect(err).To(MatchError(ContainSubstring(errorMsg)))
				Expect(version).To(BeNil())
			}
		}

		It("should succeed for a valid version", func() {
			test(validVersionFile, "")
		})

		It("should succeed for a valid dev version", func() {
			test(validDevVersionFile, "")
		})

		It("should fail for an empty version", func() {
			test(emptyVersionFile, "is empty")
		})

		It("should fail for an invalid version", func() {
			test(invalidVersionFile, "invalid semver version in first line of file")
		})

		It("should fail for an invalid dev version", func() {
			test(invalidDevVersionFile, "invalid semver version in first line of file")
		})

		It("should succeed for a valid version in first line and a empty second line", func() {
			test(validVersionWithEmptyLineFile, "")
		})

		It("should fail for a version in the second line", func() {
			test(validVersionInSecondLineFile, "invalid semver version in first line of file")
		})
	})

	Describe("#releaseHandler", func() {
		const (
			currentDevVersion     = "v1.75.0-dev"
			nextDevVersion        = "v1.76.0-dev"
			nextPatchDevVersion   = "v1.75.1-dev"
			currentReleaseVersion = "v1.75.0"
			previousDevVersion    = "v1.74.0-dev"
			invalidVersion        = "foobar"

			versionFile = "VERSION"
		)

		var (
			fakeGit           *localgit.LocalGit
			fakeClientFactory gitv2.ClientFactory
			rh                *releaseHandler

			fakeOrg    = "foo"
			fakeRepo   = "bar"
			mainBranch = "main"
		)

		BeforeEach(func() {
			var err error
			fakeGit, fakeClientFactory, err = localgit.NewV2()
			Expect(err).ToNot(HaveOccurred())
			fakeGit.InitialBranch = mainBranch

			rh = &releaseHandler{
				gitClientFactory: fakeClientFactory,
				options: options{
					releaseBranchPrefix: "release-",
					versionFilePath:     versionFile,
					baseSHA:             mainBranch,
				},
			}

			DeferCleanup(func() {
				Expect(fakeGit.Clean()).To(Succeed())
			})

			rh.options.org = fakeOrg
			rh.options.repo = fakeRepo
			Expect(fakeGit.MakeFakeRepo(fakeOrg, fakeRepo)).To(Succeed())
		})

		addFileCommit := func(versionFileContent string) {
			filename := versionFile
			content := versionFileContent
			if content == "" {
				filename = "foo"
				content = "bar"
			}
			Expect(fakeGit.AddCommit(fakeOrg, fakeRepo, map[string][]byte{filename: []byte(content)})).To(Succeed())
		}

		removeFileCommit := func(filenames []string) {
			Expect(fakeGit.RmCommit(fakeOrg, fakeRepo, filenames)).To(Succeed())
		}

		Context("run", func() {
			It("should succeed for a new minor dev version", func() {
				addFileCommit(currentDevVersion)
				addFileCommit(nextDevVersion)
				Expect(rh.run()).To(Succeed())
			})

			It("should succeed for a new patch dev version", func() {
				addFileCommit(currentDevVersion)
				addFileCommit(nextPatchDevVersion)
				Expect(rh.run()).To(Succeed())
			})

			It("should succeed for the current release", func() {
				addFileCommit(currentDevVersion)
				addFileCommit(currentReleaseVersion)
				Expect(rh.run()).To(Succeed())
			})

			It("should succeed when the next dev cycle starts", func() {
				addFileCommit(currentReleaseVersion)
				addFileCommit(nextDevVersion)
				Expect(rh.run()).To(Succeed())
			})

			It("should succeed when version file does not change", func() {
				addFileCommit(currentDevVersion)
				addFileCommit("")
				Expect(rh.run()).To(Succeed())
			})

			It("should succeed when there is no version file", func() {
				addFileCommit("")
				Expect(rh.run()).To(Succeed())
			})

			It("should fail when downgrading the version", func() {
				addFileCommit(currentDevVersion)
				addFileCommit(previousDevVersion)
				Expect(rh.run()).To(MatchError(ContainSubstring("version does not increase")))
			})

			It("should fail when there is only the initial commit", func() {
				Expect(rh.run()).ToNot(Succeed())
			})

			It("should fail when the version file is created", func() {
				addFileCommit(currentDevVersion)
				Expect(rh.run()).To(MatchError(ContainSubstring("open")))
			})

			It("should fail when the version file is removed", func() {
				addFileCommit(currentDevVersion)
				removeFileCommit([]string{versionFile})
				Expect(rh.run()).To(MatchError(ContainSubstring("open")))
			})

			It("should fail when the previous version file is invalid", func() {
				addFileCommit(invalidVersion)
				addFileCommit(currentDevVersion)
				Expect(rh.run()).To(MatchError(ContainSubstring("Invalid Semantic Version")))
			})

			It("should fail when the current version file is invalid", func() {
				addFileCommit(previousDevVersion)
				addFileCommit(invalidVersion)
				Expect(rh.run()).To(MatchError(ContainSubstring("Invalid Semantic Version")))
			})
		})

		Context("compareVersions", func() {
			JustBeforeEach(func() {
				var err error
				rh.repoClient, err = fakeClientFactory.ClientFromDir(fakeOrg, fakeRepo, path.Join(fakeGit.Dir, fakeOrg, fakeRepo))
				Expect(err).ToNot(HaveOccurred())
				Expect(rh.repoClient.Checkout(mainBranch)).To(Succeed())
			})

			It("should identify the next major/minor dev version", func() {
				addFileCommit(currentDevVersion)
				addFileCommit(nextDevVersion)
				c, err := rh.compareVersions()
				Expect(err).ToNot(HaveOccurred())
				Expect(c).To(Equal(&versionClassification{
					currentVersion:  semver.MustParse(nextDevVersion),
					previousVersion: semver.MustParse(currentDevVersion),
					versionChange:   prepareNewMajorMinorVersion,
				}))
			})

			It("should identify the next patch dev version", func() {
				addFileCommit(currentDevVersion)
				addFileCommit(nextPatchDevVersion)
				c, err := rh.compareVersions()
				Expect(err).ToNot(HaveOccurred())
				Expect(c).To(Equal(&versionClassification{
					currentVersion:  semver.MustParse(nextPatchDevVersion),
					previousVersion: semver.MustParse(currentDevVersion),
					versionChange:   prepareNewPatchVersion,
				}))
			})

			It("should identify the next release", func() {
				addFileCommit(currentDevVersion)
				addFileCommit(currentReleaseVersion)
				c, err := rh.compareVersions()
				Expect(err).ToNot(HaveOccurred())
				Expect(c).To(Equal(&versionClassification{
					currentVersion:  semver.MustParse(currentReleaseVersion),
					previousVersion: semver.MustParse(currentDevVersion),
					versionChange:   newRelease,
				}))
			})

			It("should identify the next dev cycle", func() {
				addFileCommit(currentReleaseVersion)
				addFileCommit(nextDevVersion)
				c, err := rh.compareVersions()
				Expect(err).ToNot(HaveOccurred())
				Expect(c).To(Equal(&versionClassification{
					currentVersion:  semver.MustParse(nextDevVersion),
					previousVersion: semver.MustParse(currentReleaseVersion),
					versionChange:   nextDevCycle,
				}))
			})

			It("should fail when version file does not change", func() {
				addFileCommit(currentDevVersion)
				addFileCommit("")
				c, err := rh.compareVersions()
				Expect(err).To(MatchError(ContainSubstring("version does not increase")))
				Expect(c).To(BeNil())
			})

			It("should fail when downgrading the version", func() {
				addFileCommit(currentDevVersion)
				addFileCommit(previousDevVersion)
				c, err := rh.compareVersions()
				Expect(err).To(MatchError(ContainSubstring("version does not increase")))
				Expect(c).To(BeNil())
			})

			It("should fail when the previous version file is invalid", func() {
				addFileCommit(invalidVersion)
				addFileCommit(currentDevVersion)
				c, err := rh.compareVersions()
				Expect(err).To(MatchError(ContainSubstring("Invalid Semantic Version")))
				Expect(c).To(BeNil())
			})

			It("should fail when the current version file is invalid", func() {
				addFileCommit(previousDevVersion)
				addFileCommit(invalidVersion)
				c, err := rh.compareVersions()
				Expect(err).To(MatchError(ContainSubstring("Invalid Semantic Version")))
				Expect(c).To(BeNil())
			})
		})
	})
})
