package main

import (
	"errors"
	"flag"
	"path"

	"log"
	"os"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/git/v2"

)

const (
	forkDir      = "forked/"
	forkAnnotation = "fork-per-release"
	forkedAnnotation = "forked"
	jobNameSuffix  = "fork"
	branchPrefix   = "release-"
	prTitle        = "Forked Release Jobs"
	commitTitle    = "Forked Release Jobs"
	targetBranchPrefix   = "jobFork"
)

var (
	o options
	gh githubServer
)

type options struct {
	dir    string
	email  string
	dryRun bool
	//The repo in which the configs are read, which should be forked
	baseRepo string
	masterBranch string
	jobName string

	recursive bool

	github prowflagutil.GitHubOptions
}

func (o *options) gatherOptions() {
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&o.dir, "dir", "", "Directory which files should be forked")
	fs.BoolVar(&o.dryRun, "dry-run", true, "DryRun")
	fs.StringVar(&o.baseRepo, "base-repo", "", "The repo in which the configs are read, which should be forked")
	fs.StringVar(&o.email, "email", "gardener.ci.robot@gmail.com", "E-Mail of the bot that commits the changes")
	fs.StringVar(&o.masterBranch, "master", "master", "Branchname of the master branch")
	fs.StringVar(&o.jobName, "jobName", "", "Name of the job: To make branch names unique, from which the PRs will be created")
	fs.BoolVar(&o.recursive, "recursive", false, "When set to true, all subfolders of the given directory will be searched for forkable folders")

	o.github.AddFlags(fs)

	err := fs.Parse(os.Args[1:])
	if err != nil {
		log.Fatalf("Unable to parse command line flags: %v\n", err)
	}
}

func getFileNames(dir string) []string {
	files, err := os.ReadDir(dir)
	if err != nil {
		log.Fatalf("Couldn't read directory: %v\n", err)
	}

	var fileNames []string
	for _, file := range files {
		if file.IsDir() {
			if o.recursive && file.Name() != forkDir {
				fileNames = append(fileNames, getFileNames(file.Name())...)
			} else {
				continue
			}
		}
		fileNames = append(fileNames, path.Join(dir, file.Name()))
	}
	return fileNames
}
func contains[K comparable](sl []K, s K) bool {
	for _, el := range sl {
		if el == s {
			return true
		}
	}
	return false
}

func checkFileExists(filePath string) bool {
	_, error := os.Stat(filePath)
	return !errors.Is(error, os.ErrNotExist)

}

func main() {
	o.gatherOptions()
	githubClient, err := o.github.GitHubClient(o.dryRun)
	if err != nil {
		log.Fatalf("Error getting GitHubClient: %v\n", err)
	}
	gitClient, err := o.github.GitClient(o.dryRun)
	if err != nil {
		log.Fatalf("Error getting Git client: %v\n", err)
	}
	botUser, err := githubClient.BotUser()
	if err != nil {
		log.Fatalf("Error getting bot name: %v\n", err)
	}
	accessibleRepos, err := githubClient.GetRepos(botUser.Login, true)
	if err != nil {
		log.Fatalf("Error listing bot repositories: %v\n", err)
	}

	gh = githubServer{
		ghc:             githubClient,
		gcf:             git.ClientFactoryFrom(gitClient),
		botUser:         botUser,
		accessibleRepos: accessibleRepos,
	}

	clonedBaseRepo := cloneBaseRepo(o.baseRepo)

	baseRepoJobsDirPath := path.Join(clonedBaseRepo.Directory(), o.dir)
	fileNames := getFileNames(baseRepoJobsDirPath)

	repos := readReposFromJobs(fileNames)
	for _, repo := range repos {
		releaseBranches := getReleaseBranches(repo)
		versions := generateVersionsFromBranches(releaseBranches)
		// Check if there is a release branch without a corresponding forked config
		for _, version := range versions {
			if !hasForkedConfig(repo, version) {
				forkConfig(fileNames, baseRepoJobsDirPath, repo, version)
			}
		}
		removeDeprecatedConfigs(repo, baseRepoJobsDirPath, versions)
	}

	pushChanges(clonedBaseRepo)
}
