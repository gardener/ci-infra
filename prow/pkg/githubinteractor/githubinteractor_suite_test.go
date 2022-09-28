package githubinteractor_test

import (
	"log"
	"testing"

	fgc "github.com/gardener/ci-infra/prow/pkg/fakes/fakegitclient"
	fghc "github.com/gardener/ci-infra/prow/pkg/fakes/fakegithubclient"
	ghi "github.com/gardener/ci-infra/prow/pkg/githubinteractor"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/test-infra/prow/github"
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
			Name:      "release-v1.02",
			Protected: true,
		},
		{
			Name:      "notRelease-v1.00",
			Protected: true,
		},
	}

	fakedUnprotectedBranches = []github.Branch{}
)

func TestProwjobconfigurator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Prowjobconfigurator Suite")
}

var _ = Describe("Testing NewRepository()", func() {
	It("should be able to split the repoName", func() {
		repo, err := ghi.NewRepository(testFullRepoName)

		Expect(*repo).To(Equal(ghi.Repository{
			FullRepoName: testFullRepoName,
			Org:          testOrg,
			Repo:         testRepo,
		}))

		Expect(err).To(BeNil())
	})

	It("should not be able to split the repoName", func() {
		repo, err := ghi.NewRepository(testOrg)
		Expect(repo).To(BeNil())
		Expect(err).To(MatchError(ghi.ErrSplit))
	})
})

var _ = Describe("Testing GetMatchingBranches()", func() {
	var rep *ghi.Repository

	BeforeEach(func() {
		ghi.Gh.Ghc = &fghc.FakeGithubClient{
			FakedProtectedBranches:   fakedProtectedBranches,
			FakedUnprotectedBranches: fakedUnprotectedBranches,
		}
		rep, _ = ghi.NewRepository(testFullRepoName)

	})

	It("should get 3 release branches", func() {
		Expect(rep.GetMatchingBranches(`release-v\d+\.\d+`)).To(HaveLen(3))
	})

	It("should get all branches", func() {
		Expect(rep.GetMatchingBranches(`.*`)).To(HaveLen(len(fakedProtectedBranches) + len(fakedUnprotectedBranches)))
	})
})

var _ = Describe("Testing PushChanges()", func() {
	var rep *ghi.Repository

	BeforeEach(func() {
		ghi.Gh.Gcf = &fgc.FakeGitClientFactory{}
		ghi.Gh.Ghc = &fghc.FakeGithubClient{
			FakedProtectedBranches:   fakedProtectedBranches,
			FakedUnprotectedBranches: fakedUnprotectedBranches,
		}

		ghi.Gh.Gc = &fgc.FakeCommitClient{}

		ghi.Gh.BotUser = &github.UserData{
			Name:  testLogin,
			Login: testLogin,
			Email: testEmail,
		}
		rep, _ = ghi.NewRepository(testFullRepoName)
		err := rep.CloneRepo()
		if err != nil {
			log.Printf("Error cloning repo: %v", err)
		}
	})

	It("should commit and push changes", func() {
		err := rep.PushChanges("test", "test", rep.FullRepoName, "test", "test", []string{"Label1", "Label2"})
		Expect(err).To(BeNil())
	})
})
