package main

import (
	"flag"
	"strings"

	"log"
	"os"

	ghi "github.com/gardener/ci-infra/prow/pkg/githubinteractor"
	"k8s.io/test-infra/prow/git/v2"
)

const (
	// ForkDir is the name where the forked configs will be put in
	ForkDir = "forked/"
	// ForkAnnotation is the annotation a job has to have in order for it to be forked
	ForkAnnotation = "fork-per-release"
	// ForkedAnnotation is the annotation that will be added when a config has been forked
	ForkedAnnotation = "forked"
	// JobNameSuffix is the suffix a forked config will be given, so that it stays unique
	JobNameSuffix = "fork"
	// BranchPrefix is the Prefix of the targeted release branch
	BranchPrefix = "release-"
	// PRTitle is the title the PR gets when the forked configs will be pushed
	PRTitle = "Forked Release Jobs"
	// CommitTitle is the title of the commit under which the bot commits the changes
	CommitTitle = "Forked Release Jobs"
	// TargetBranchPrefix is the prefix under which the job-forker will create a branch in which the changes will be commited
	TargetBranchPrefix = "jobFork"
)

var (
	o options
)

type options struct {
	configsPath string
	//The repo in which the configs are read, which should be forked
	baseRepo       string
	templateBranch string
	jobName        string
	overrideLabels []string

	ghio ghi.InteractorOptions
}

func (o *options) gatherOptions() {

	var overrideLabels string

	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&o.baseRepo, "base-repo", "", "The repo in which the configs are read, which should be forked")
	fs.StringVar(&o.templateBranch, "template-branch", "master", "Name of the branch of which the configs should be forked")
	fs.StringVar(&o.configsPath, "configs-path", "", "Directory, from which files should be forked")
	fs.BoolVar(&o.ghio.DryRun, "dry-run", true, "DryRun")
	fs.StringVar(&o.ghio.BotUserEmail, "email", "", "E-Mail of the bot that commits the changes")
	fs.StringVar(&o.jobName, "job-name", "", "Name of the job: To make branch names unique, from which the PRs will be created")
	fs.BoolVar(&o.ghio.Recursive, "recursive", false, "When set to true, all subfolders of the given directory will be searched for forkable folders")
	fs.StringVar(&overrideLabels, "override-labels", "", "Labels which should be added to the PR")
	o.ghio.Github.AddFlags(fs)

	err := fs.Parse(os.Args[1:])
	if err != nil {
		log.Fatalf("Unable to parse command line flags: %v\n", err)
	}
	if overrideLabels != "" {
		o.overrideLabels = append(o.overrideLabels, strings.Split(overrideLabels, ",")...)
	} else {
		o.overrideLabels = nil
	}
}

func contains[K comparable](sl []K, s K) bool {
	for _, el := range sl {
		if el == s {
			return true
		}
	}
	return false
}

func main() {
	o.gatherOptions()
	githubClient, err := o.ghio.Github.GitHubClient(o.ghio.DryRun)
	if err != nil {
		log.Fatalf("Error getting GitHubClient: %v\n", err)
	}
	gitClient, err := o.ghio.Github.GitClient(o.ghio.DryRun)
	if err != nil {
		log.Fatalf("Error getting Git client: %v\n", err)
	}
	botUser, err := githubClient.BotUser()
	if err != nil {
		log.Fatalf("Error getting bot name: %v\n", err)
	}

	ghi.Gh = ghi.GithubServer{
		Ghc:     githubClient,
		Gcf:     git.ClientFactoryFrom(gitClient),
		Gc:      &ghi.CommitClient{},
		BotUser: botUser,
	}

	baseRepo, err := ghi.NewRepository(o.baseRepo)
	if err != nil {
		log.Fatalf("Couldn't create repository object: %v\n", err)
	}
	err = baseRepo.CloneRepo()
	if err != nil {
		log.Fatalf("Couldn't clone base repository: %v\n", err)
	}

	err = generateForkedConfigurations(baseRepo)
	if err != nil {
		log.Fatalf("Error during forking of configurations: %v\n", err)
	}

	err = baseRepo.PushChanges(TargetBranchPrefix+o.jobName, CommitTitle, o.baseRepo, PRTitle, o.templateBranch, o.overrideLabels)
	if err != nil {
		log.Fatalf("Error during pushing of changes: %v\n", err)
	}
}
