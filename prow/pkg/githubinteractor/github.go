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

	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
)

var (
	// Gh is used as a global object to let other packages interact with Git / GitHub
	Gh GithubServer
	// ErrSplit occurs, when a repository name is tried to split, but it couldn't extract the Organisation and the Repository name
	ErrSplit = errors.New("couldn't split")
)

// InteractorOptions are the options which are used by the GitHubInteractor
type InteractorOptions struct {
	BotUserEmail string
	DryRun       bool

	Recursive bool

	Github prowflagutil.GitHubOptions
}

// GetFileNames returns the relative filepath to the `dir`, ignoring folders / files in `ignoredFileNames`, goes through subfolders, if `recursive` is `true`
func GetFileNames(dir string, ignoredFileNames []string, recursive bool) ([]string, error) {

	files, err := os.ReadDir(dir)
	if err != nil {
		return []string{}, err
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
			recursiveFileNames, err := GetFileNames(file.Name(), ignoredFileNames, recursive)
			if err != nil {
				return nil, err
			}
			fileNames = append(fileNames, recursiveFileNames...)
		}
		fileNames = append(fileNames, path.Join(dir, file.Name()))
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
	Commit(repoClient git.RepoClient, message string) error
}

// CommitClient is a custom Client to overwrite the prow Git Commit functionality, because it isn't implemented there
type CommitClient struct{}

// GithubServer contains all components to interact with Git and GitHub
type GithubServer struct {
	Ghc GithubClient
	Gcf git.ClientFactory

	BotUser *github.UserData

	Gc GitClient
}

// Repository handles the conversion between a fully qualified repository name and its organisation / repository name
type Repository struct {
	FullRepoName string
	Org          string
	Repo         string

	RepoClient git.RepoClient
}

// NewRepository creates a Repository object, by taking its fully qualified repository name (organisation/reponame) and splitting it into its counterparts
func NewRepository(fullRepoName string) (*Repository, error) {
	rep := Repository{
		FullRepoName: fullRepoName,
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

	unProtectedBranches, err := Gh.Ghc.GetBranches(r.Org, r.Repo, false)

	if err != nil {
		return nil, err
	}

	protectedBranches, err := Gh.Ghc.GetBranches(r.Org, r.Repo, true)

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
	rep, err := Gh.Gcf.ClientFor(r.Org, r.Repo)
	if err != nil {
		return err
	}

	r.RepoClient = rep
	return nil
}

func (r *Repository) ensureForkExists() (*Repository, error) {
	// fork repo if it doesn't exist
	repo, err := Gh.Ghc.EnsureFork(Gh.BotUser.Login, r.Org, r.Repo)
	if err != nil {
		return nil, err
	}
	return NewRepository(Gh.BotUser.Login + "/" + repo)
}

func execute(repoClient git.RepoClient, args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = repoClient.Directory()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Commit is a custom implementation of the Commit functionality. It works by setting the email and login of `Gh.BotUser`, staging all changes and committing them with `message`, all using the git binary.
// Therefore a git binary must be present in `$PATH`
func (gc *CommitClient) Commit(repoClient git.RepoClient, message string) error {
	err := execute(repoClient, "git", "config", "user.email", Gh.BotUser.Email)
	if err != nil {
		return err
	}

	err = execute(repoClient, "git", "config", "user.name", Gh.BotUser.Login)
	if err != nil {
		return err
	}

	err = execute(repoClient, "git", "add", "-A")
	if err != nil {
		return err
	}

	err = execute(repoClient, "git", "commit", "-m", `"`+message+`"`)
	if err != nil {
		return err
	}

	return nil
}

// PushChanges pushes changes to the `targetBranch` with `commitMessage` and opens a PR, if it is not open already
func (r *Repository) PushChanges(targetBranch, commitMessage, baseRepo, prTitle, templateBranch string, labels []string) error {

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
	if err := Gh.Gc.Commit(r.RepoClient, commitMessage); err != nil {
		return err
	}

	prs, err := Gh.Ghc.GetPullRequests(r.Org, r.Repo)
	if err != nil {
		return err
	}
	prNum := 0
	prFlag := true
	for _, pr := range prs {
		log.Printf("PR: %v, Head: %v, Base: %v\n", pr.Title, pr.Head.Repo.FullName, pr.Base.Repo.FullName)
		if pr.Head.Repo.FullName == fork.FullRepoName && pr.Base.Repo.FullName == baseRepo {
			log.Printf("There is already an open PR")
			prNum = pr.Number
			prFlag = false
			break
		}
	}

	if err := r.RepoClient.PushToNamedFork(fork.Repo, targetBranch, true); err != nil {
		return err
	}
	log.Printf("Pushed to NamedFork: %v,%v\n", fork.Repo, targetBranch)

	if prFlag {
		head := fmt.Sprintf("%s:%s", Gh.BotUser.Login, targetBranch)
		log.Printf("Head: %v\n", head)
		prNum, err = Gh.Ghc.CreatePullRequest(r.Org, r.Repo, prTitle, "", head, templateBranch, true)
		if err != nil {
			return err
		}
		log.Printf("Created new PR: %v\n", prNum)

	}
	if labels != nil {
		err = Gh.Ghc.AddLabels(r.Org, r.Repo, prNum, labels...)
		if err != nil {
			return err
		}
	}

	return nil
}
