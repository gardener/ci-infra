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

package githubinteractor

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"

	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
)

var (
	// ErrSplit occurs, when a repository name is tried to split, but it couldn't extract the Organisation and the Repository name
	ErrSplit = errors.New("couldn't split")
)

// GetFileNames returns the relative filepath to the `dir`, ignoring folders / files in `ignoredFileNames`, goes through subfolders, if `recursive` is `true`
func GetFileNames(dir string, ignoredFileNames []string, recursive bool) ([]string, error) {

	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var fileNames []string
	for _, file := range files {
		ignored := false
		for _, ignoredFileName := range ignoredFileNames {
			if file.Name() == ignoredFileName {
				ignored = true
				break
			}
		}
		if ignored {
			continue
		}

		if file.IsDir() {
			if !recursive {
				continue
			}
			recursiveFileNames, err := GetFileNames(path.Join(dir, file.Name()), ignoredFileNames, recursive)
			if err != nil {
				return nil, err
			}
			fileNames = append(fileNames, recursiveFileNames...)
		} else {
			fileNames = append(fileNames, path.Join(dir, file.Name()))
		}
	}
	return fileNames, nil
}

// GithubClient defines the methods used to interact with GitHub
type GithubClient interface {
	CreatePullRequest(org, repo, title, body, head, base string, canModify bool) (int, error)
	EnsureFork(forkingUser, org, repo string) (string, error)

	GetPullRequests(org, repo string) ([]github.PullRequest, error)
	GetBranches(org, repo string, onlyProtected bool) ([]github.Branch, error)
	AddLabels(org, repo string, number int, label ...string) error
}

// GitClient is used to swap the functions for faked functions during testing
type GitClient interface {
	Commit(directory, name, email, message string, signoff bool) error
}

// CommitClient is a custom Client to overwrite the prow Git Commit functionality, because it isn't implemented there
type CommitClient struct {
}

// Commit is a custom implementation of the Commit functionality. It works by setting the email and login of `Gh.BotUser`, staging all changes and committing them with `message`, all using the git binary.
// Therefore a git binary must be present in `$PATH`
func (gc *CommitClient) Commit(directory, name, email, message string, signoff bool) error {
	if err := executeCmd(directory, "git", "add", "-A"); err != nil {
		return fmt.Errorf("git add: %w", err)
	}
	commitArgs := []string{"commit", "-m", message}
	if name != "" && email != "" {
		commitArgs = append(commitArgs, "--author", fmt.Sprintf("%s <%s>", name, email))
	}
	if signoff {
		commitArgs = append(commitArgs, "--signoff")
	}
	if err := executeCmd(directory, "git", commitArgs...); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	return nil
}

// GithubServer contains all components to interact with Git and GitHub
type GithubServer struct {
	Ghc GithubClient
	Gcf git.ClientFactory
	Gc  GitClient

	BotUser *github.UserData
	Email   string
}

// GetEmail tries to get the email of GithubServer from different sources
func (g *GithubServer) GetEmail() string {
	email := g.Email
	if email == "" {
		email = g.BotUser.Email
	}
	return email
}

// Repository handles the conversion between a fully qualified repository name and its organisation / repository name
type Repository struct {
	FullRepoName string
	Org          string
	Repo         string

	Gh         *GithubServer
	RepoClient git.RepoClient
}

// NewRepository creates a Repository object, by taking its fully qualified repository name (organisation/reponame) and splitting it into its counterparts
func NewRepository(fullRepoName string, gh *GithubServer) (*Repository, error) {
	rep := Repository{
		FullRepoName: fullRepoName,
		Gh:           gh,
	}
	err := rep.splitRepoName()
	if err != nil {
		return nil, err
	}
	return &rep, nil
}

func (r *Repository) splitRepoName() error {
	splitName := strings.Split(r.FullRepoName, "/")
	if len(splitName) != 2 {
		return ErrSplit
	}
	r.Org = splitName[0]
	r.Repo = splitName[1]

	return nil
}

// GetMatchingBranches returns all branches of the repository which match `releaseBranchPattern` RegEx
func (r *Repository) GetMatchingBranches(releaseBranchPattern string) ([]github.Branch, error) {

	unProtectedBranches, err := r.Gh.Ghc.GetBranches(r.Org, r.Repo, false)

	if err != nil {
		return nil, err
	}

	protectedBranches, err := r.Gh.Ghc.GetBranches(r.Org, r.Repo, true)

	if err != nil {
		return nil, err
	}

	branches := append(unProtectedBranches, protectedBranches...)

	var releaseBranches []github.Branch
	release := regexp.MustCompile(releaseBranchPattern)
	for _, branch := range branches {
		if release.MatchString(branch.Name) {
			releaseBranches = append(releaseBranches, branch)
		}

	}
	return releaseBranches, nil
}

// CloneRepo clones the repository onto the filesystem and sets the `RepoClient` of the repository to interact with it on the filesystem
func (r *Repository) CloneRepo() error {
	rep, err := r.Gh.Gcf.ClientFor(r.Org, r.Repo)
	if err != nil {
		return err
	}

	r.RepoClient = rep
	return nil
}

func (r *Repository) ensureForkExists() (*Repository, error) {
	// fork repo if it doesn't exist
	repo, err := r.Gh.Ghc.EnsureFork(r.Gh.BotUser.Login, r.Org, r.Repo)
	if err != nil {
		return nil, err
	}
	return NewRepository(fmt.Sprintf("%s/%s", r.Gh.BotUser.Login, repo), r.Gh)
}

// PushChanges pushes changes to the `targetBranch` with `commitMessage` and opens a PR, if it is not open already
func (r *Repository) PushChanges(upstreamRepo, upstreamBranch, targetBranch, commitMessage, prTitle string, labels []string) error {
	if err := r.RepoClient.Config("user.name", r.Gh.BotUser.Name); err != nil {
		return fmt.Errorf("failed to configure git user: %w", err)
	}
	if err := r.RepoClient.Config("user.email", r.Gh.GetEmail()); err != nil {
		return fmt.Errorf("failed to configure git email: %w", err)
	}

	fork, err := r.ensureForkExists()
	if err != nil {
		return err
	}
	log.Println("Ensured fork exists")
	if r.RepoClient.BranchExists(targetBranch) {
		if err := r.RepoClient.Checkout(targetBranch); err != nil {
			return err
		}
	} else if err := r.RepoClient.CheckoutNewBranch(targetBranch); err != nil {
		return err
	}
	defer func() {
		err := r.RepoClient.Clean()
		if err != nil {
			log.Printf("Error on cleaning up repo: %v\n", err)
		}
	}()
	if err := r.Gh.Gc.Commit(r.RepoClient.Directory(), r.Gh.BotUser.Name, r.Gh.GetEmail(), commitMessage, false); err != nil {
		return err
	}

	prs, err := r.Gh.Ghc.GetPullRequests(r.Org, r.Repo)
	if err != nil {
		return err
	}

	var (
		prNumber int
		prExists bool
	)

	for _, pr := range prs {
		log.Printf("PR: %v, Head: %v, Base: %v\n", pr.Title, pr.Head.Repo.FullName, pr.Base.Repo.FullName)
		if pr.Head.Repo.FullName == fork.FullRepoName && pr.Base.Repo.FullName == upstreamRepo {
			log.Printf("There is already an open PR")
			prNumber = pr.Number
			prExists = true
			break
		}
	}

	if err := r.RepoClient.PushToNamedFork(fork.Repo, targetBranch, true); err != nil {
		return err
	}
	log.Printf("Pushed to branch %s on %s/%s\n", targetBranch, r.Gh.BotUser.Login, fork.Repo)

	if !prExists {
		head := fmt.Sprintf("%s:%s", r.Gh.BotUser.Login, targetBranch)
		log.Printf("Head: %v\n", head)
		prNumber, err = r.Gh.Ghc.CreatePullRequest(r.Org, r.Repo, prTitle, "", head, upstreamBranch, true)
		if err != nil {
			return err
		}
		log.Printf("Created new PR: %v\n", prNumber)

	}
	if labels != nil {
		err = r.Gh.Ghc.AddLabels(r.Org, r.Repo, prNumber, labels...)
		if err != nil {
			return err
		}
	}

	return nil
}

func executeCmd(directory, command string, args ...string) error {
	cmd := exec.Command(command, args...)
	cmd.Dir = directory
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
