// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package githubinteractor_test

import (
	"fmt"

	fgc "github.com/gardener/ci-infra/prow/pkg/git/fakegit"
	fghc "github.com/gardener/ci-infra/prow/pkg/github/fakegithub"
	ghi "github.com/gardener/ci-infra/prow/pkg/githubinteractor"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/prow/pkg/github"
)

const (
	testOrg          = "testOrg"
	testRepo         = "testRepo"
	testFullRepoName = testOrg + "/" + testRepo
	testLogin        = "testUser"
	testEmail        = "test@test.test"
)

var (
	fakedProtectedBranches = []github.Branch{
		{
			Name:      "release-v1.00",
			Protected: true,
		},
		{
			Name:      "release-v1.01",
			Protected: true,
		},
		{
			Name:      "notRelease-v1.00",
			Protected: true,
		},
	}

	fakedUnprotectedBranches = []github.Branch{
		{
			Name:      "release-v1.02",
			Protected: false,
		},
	}
)

var _ = Describe("Testing NewRepository()", func() {
	var fakeGithubServer *ghi.GithubServer

	BeforeEach(func() {
		fakeGhc := &fghc.FakeGithubClient{
			ProtectedBranches:   fakedProtectedBranches[:],
			UnprotectedBranches: fakedUnprotectedBranches[:],
		}

		fakeGithubServer = &ghi.GithubServer{
			Ghc: fakeGhc,
		}
	})

	It("should be able to split the repoName", func() {
		repo, err := ghi.NewRepository(testFullRepoName, fakeGithubServer)

		Expect(repo.FullRepoName).To(Equal(testFullRepoName))
		Expect(repo.Org).To(Equal(testOrg))
		Expect(repo.Repo).To(Equal(testRepo))

		Expect(err).NotTo(HaveOccurred())
	})

	It("should not be able to split the repoName", func() {
		repo, err := ghi.NewRepository(testOrg, fakeGithubServer)
		Expect(repo).To(BeNil())
		Expect(err).To(MatchError(fmt.Sprintf("repo %s cannot be split into org/repo", testOrg)))
	})
})

var _ = Describe("Testing GetMatchingBranches()", func() {
	var rep *ghi.Repository

	BeforeEach(func() {
		fakeGhc := &fghc.FakeGithubClient{
			ProtectedBranches:   fakedProtectedBranches,
			UnprotectedBranches: fakedUnprotectedBranches,
		}

		fakeGithubServer := &ghi.GithubServer{
			Ghc: fakeGhc,
		}

		rep, _ = ghi.NewRepository(testFullRepoName, fakeGithubServer)

	})

	It("should get 3 release branches", func() {
		Expect(rep.GetMatchingBranches("release-v\\d+\\.\\d+")).To(HaveLen(3))
	})

	It("should get all branches", func() {
		Expect(rep.GetMatchingBranches(`.*`)).To(HaveLen(len(fakedProtectedBranches) + len(fakedUnprotectedBranches)))
	})
})

var _ = Describe("Testing PushChanges()", func() {
	var (
		fakeGhc     *fghc.FakeGithubClient
		fakeGcf     *fgc.FakeGitClientFactory
		fakeGc      *fgc.FakeCommitClient
		botUser     *github.UserData
		buildServer func(appAuth bool) *ghi.GithubServer
	)

	BeforeEach(func() {
		fakeGcf = &fgc.FakeGitClientFactory{}
		fakeGhc = &fghc.FakeGithubClient{
			ProtectedBranches:   fakedProtectedBranches,
			UnprotectedBranches: fakedUnprotectedBranches,
		}
		fakeGc = &fgc.FakeCommitClient{}
		botUser = &github.UserData{
			Name:  testLogin,
			Login: testLogin,
			Email: testEmail,
		}
		buildServer = func(appAuth bool) *ghi.GithubServer {
			return &ghi.GithubServer{
				Ghc:     fakeGhc,
				Gcf:     fakeGcf,
				Gc:      fakeGc,
				BotUser: botUser,
				AppAuth: appAuth,
			}
		}
	})

	It("should commit and push changes to a fork", func() {
		rep, err := ghi.NewRepository(testFullRepoName, buildServer(false))
		Expect(err).NotTo(HaveOccurred())
		Expect(rep.CloneRepo()).To(Succeed())

		err = rep.PushChanges("test", "test", rep.FullRepoName, "test", "test", []string{"Label1", "Label2"}, true)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should commit and push changes to a branch when authenticating as GitHub App", func() {
		rep, err := ghi.NewRepository(testFullRepoName, buildServer(true))
		Expect(err).NotTo(HaveOccurred())
		Expect(rep.CloneRepo()).To(Succeed())

		err = rep.PushChanges("test", "test", rep.FullRepoName, "test", "test", []string{"Label1", "Label2"}, false)
		Expect(err).NotTo(HaveOccurred())
	})
})

var _ = Describe("Testing GithubServer committer identity", func() {
	It("should prefer the configured email and bot user name", func() {
		server := &ghi.GithubServer{
			BotUser: &github.UserData{Name: testLogin, Login: testLogin, Email: testEmail},
			Email:   "configured@test.test",
		}
		Expect(server.GetName()).To(Equal(testLogin))
		Expect(server.GetEmail()).To(Equal("configured@test.test"))
	})

	It("should prefer the configured name and email over the bot user", func() {
		server := &ghi.GithubServer{
			BotUser: &github.UserData{Name: testLogin, Login: testLogin, Email: testEmail},
			Name:    "configured-user",
			Email:   "configured@test.test",
		}
		Expect(server.GetName()).To(Equal("configured-user"))
		Expect(server.GetEmail()).To(Equal("configured@test.test"))
	})

	It("should derive a bot identity for GitHub App auth when name and email are empty", func() {
		server := &ghi.GithubServer{
			BotUser: &github.UserData{Login: "my-app"},
			AppAuth: true,
		}
		Expect(server.GetName()).To(Equal("my-app[bot]"))
		Expect(server.GetEmail()).To(Equal("my-app[bot]@users.noreply.github.com"))
	})

	It("should use the app slug and ignore the API display name and email for GitHub App auth", func() {
		server := &ghi.GithubServer{
			BotUser: &github.UserData{
				Name:  "Gardener Prow Presubmit Generator",
				Login: "gardener-prow-presubmit-generator",
				Email: "gardener-prow-presubmit-generator@users.noreply.github.com",
			},
			AppAuth: true,
		}
		Expect(server.GetName()).To(Equal("gardener-prow-presubmit-generator[bot]"))
		Expect(server.GetEmail()).To(Equal("gardener-prow-presubmit-generator[bot]@users.noreply.github.com"))
	})

	It("should enforce the [bot] suffix on the committer name for GitHub App auth even if a display name is set", func() {
		server := &ghi.GithubServer{
			BotUser: &github.UserData{Name: "my-app", Login: "my-app"},
			AppAuth: true,
		}
		Expect(server.GetName()).To(Equal("my-app[bot]"))
	})

	It("should append the [bot] suffix to an explicitly configured name without a login", func() {
		server := &ghi.GithubServer{
			BotUser: &github.UserData{Name: "gardener-ci-robot"},
			AppAuth: true,
		}
		Expect(server.GetName()).To(Equal("gardener-ci-robot[bot]"))
	})

	It("should not duplicate the [bot] suffix when the login already carries it", func() {
		server := &ghi.GithubServer{
			BotUser: &github.UserData{Login: "my-app[bot]"},
			AppAuth: true,
		}
		Expect(server.GetName()).To(Equal("my-app[bot]"))
	})
})
