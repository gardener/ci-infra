// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package githubinteractor_test

import (
	"log"

	fgc "github.com/gardener/ci-infra/prow/pkg/git/fakegit"
	fghc "github.com/gardener/ci-infra/prow/pkg/github/fakegithub"
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

		Expect(err).To(BeNil())
	})

	It("should not be able to split the repoName", func() {
		repo, err := ghi.NewRepository(testOrg, fakeGithubServer)
		Expect(repo).To(BeNil())
		Expect(err).To(MatchError(ghi.ErrSplit))
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
	var rep *ghi.Repository

	BeforeEach(func() {
		fakeGcf := &fgc.FakeGitClientFactory{}
		fakeGhc := &fghc.FakeGithubClient{
			ProtectedBranches:   fakedProtectedBranches,
			UnprotectedBranches: fakedUnprotectedBranches,
		}
		fakeGc := &fgc.FakeCommitClient{}
		botUser := &github.UserData{
			Name:  testLogin,
			Login: testLogin,
			Email: testEmail,
		}
		fakeGithubServer := &ghi.GithubServer{
			Ghc:     fakeGhc,
			Gcf:     fakeGcf,
			Gc:      fakeGc,
			BotUser: botUser,
		}

		rep, _ = ghi.NewRepository(testFullRepoName, fakeGithubServer)
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
