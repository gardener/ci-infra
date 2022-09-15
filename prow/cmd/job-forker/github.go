package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
)

type githubClient interface {
	CreateFork(org, repo string) (string, error)
	CreatePullRequest(org, repo, title, body, head, base string, canModify bool) (int, error)
	EnsureFork(forkingUser, org, repo string) (string, error)

	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	GetPullRequestPatch(org, repo string, number int) ([]byte, error)
	GetPullRequests(org, repo string) ([]github.PullRequest, error)
	GetRepo(owner, name string) (github.FullRepo, error)
	GetBranches(org, repo string, onlyProtected bool) ([]github.Branch, error)
}

type githubServer struct {
	ghc githubClient
	gcf git.ClientFactory

	botUser         *github.UserData
	accessibleRepos []github.Repo
}

func getReleaseBranches(repo string) []github.Branch {
	splitName := strings.Split(repo, "/")
	if len(splitName) != 2 {
		log.Fatalf("Couldn't split repoName: %v\n", splitName)
	}
	unProtectedBranches, err := gh.ghc.GetBranches(splitName[0], splitName[1], false)

	if err != nil {
		log.Fatalf("Couldn't get branches: %v\n", err)
	}

	protectedBranches, err := gh.ghc.GetBranches(splitName[0], splitName[1], true)

	if err != nil {
		log.Fatalf("Couldn't get branches: %v\n", err)
	}

	branches := append(unProtectedBranches, protectedBranches...)

	var releaseBranches []github.Branch
	for _, branch := range branches {
		release := regexp.MustCompile(branchPrefix + `v\d+\.\d+`)
		if release.Match([]byte(branch.Name)) {
			releaseBranches = append(releaseBranches, branch)
		}

	}
	return releaseBranches
}
func cloneBaseRepo(repo string) git.RepoClient {
	nameSplit := strings.Split(repo, "/")
	if len(nameSplit) != 2 {
		log.Fatalf("Couldn't correctly split repository name in org and repo: %v\n", nameSplit)
	}
	r, err := gh.gcf.ClientFor(nameSplit[0], nameSplit[1])
	if err != nil {
		log.Fatalf("Error on getting client for %v: %v\n", repo, err)
	}

	return r
}

func generateVersionsFromBranches(branches []github.Branch) []string {
	versions := make([]string, len(branches))
	for i, branch := range branches {
		versions[i] = strings.ReplaceAll(branch.Name, branchPrefix, "")
	}

	return versions
}

func ensureForkExists(org, repo string) string {
	// fork repo if it doesn't exist
	repo, err := gh.ghc.EnsureFork(gh.botUser.Login, org, repo)
	if err != nil {
		log.Fatalf("Couldn't ensure Fork exists: %v\n", err)
	}

	return repo
}

func execute(repoClient git.RepoClient, args ...string) {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = repoClient.Directory()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		log.Fatalf("Couldn't execute %v: %v", args, err)
	}
}
func commit(repoClient git.RepoClient, message string) error {
	execute(repoClient, "git", "config", "user.email", o.email)
	execute(repoClient, "git", "config", "user.name", gh.botUser.Login)
	execute(repoClient, "git", "add", "-A")
	execute(repoClient, "git", "commit", "-m", `"`+message+`"`)

	return nil
}

func pushChanges(repoClient git.RepoClient) {

	splitNames := strings.Split(o.baseRepo, "/")

	if len(splitNames) != 2 {
		log.Fatalf("Couldn't resolve baseRepoName to Org and Repo: %v", splitNames)
	}

	org := splitNames[0]
	repo := splitNames[1]
	forkName := ensureForkExists(org, repo)
	targetBranch := targetBranchPrefix + "-" + o.jobName
	if repoClient.BranchExists(targetBranch) {
		if err := repoClient.Checkout(targetBranch); err != nil {
			log.Fatalf("Couldn't checkout branch: %v\n", err)
		}
	} else {
		if err := repoClient.CheckoutNewBranch(targetBranch); err != nil {
			log.Fatalf("Couldn't checkout new branch: %v\n", err)
		}
	}
	defer func() {
		err := repoClient.Clean()
		if err != nil {
			log.Fatalf("Error on cleaning up repo: %v\n", err)
		}
	}()
	if err := commit(repoClient, commitTitle); err != nil {
		log.Fatalf("Couldn't commit: %v\n", err)
	}

	prs, err := gh.ghc.GetPullRequests(org, repo)
	if err != nil {
		log.Fatalf("\n")
	}

	prFlag := true
	for _, pr := range prs {
		log.Printf("PR: %v, Head: %v, Base: %v\n", pr.Title, pr.Head.Repo.FullName, pr.Base.Repo.FullName)
		if pr.Head.Repo.FullName == gh.botUser.Login+"/"+forkName && pr.Base.Repo.FullName == o.baseRepo {
			log.Printf("There is already an open PR")
			prFlag = false
			break
		}
	}

	if err := repoClient.PushToNamedFork(forkName, targetBranch, true); err != nil {
		log.Fatalf("Couldn't push to named Fork%v\n", err)
	}
	log.Printf("Pushed to NamedFork%v,%v\n", forkName, targetBranch)

	if prFlag {
		head := fmt.Sprintf("%s:%s", gh.botUser.Login, targetBranch)
		log.Printf("Head: %v\n", head)
		createdNum, err := gh.ghc.CreatePullRequest(org, repo, prTitle, "", head, o.masterBranch, true)
		if err != nil {
			log.Fatalf("Couldn't create PR: %v\n", err)
		}
		log.Printf("Created new PR: %v\n", createdNum)

	}

}
