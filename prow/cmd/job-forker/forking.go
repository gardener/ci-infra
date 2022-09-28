package main

import (
	"errors"
	"path"

	"log"
	"os"
	"path/filepath"
	"strings"

	ghi "github.com/gardener/ci-infra/prow/pkg/githubinteractor"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"

	"sigs.k8s.io/yaml"
)

func generateVersionsFromBranches(branches []github.Branch, branchPrefix string) []string {
	versions := make([]string, len(branches))
	for i, branch := range branches {
		versions[i] = strings.ReplaceAll(branch.Name, branchPrefix, "")
	}

	return versions
}

func generatePresubmits(j config.JobConfig, version, repository string) []config.Presubmit {
	newPresubmits := []config.Presubmit{}
	for _, presubmit := range j.PresubmitsStatic[repository] {
		if presubmit.Annotations[ForkAnnotation] != "true" {
			continue
		}
		delete(presubmit.Annotations, ForkAnnotation)
		presubmit.Annotations[ForkedAnnotation] = "true"
		// Check if branch has no forked config yet
		presubmit.Name = presubmit.Name + JobNameSuffix + strings.ReplaceAll(version, ".", "-")
		presubmit.Branches = []string{BranchPrefix + version}
		presubmit.SkipBranches = nil

		newPresubmits = append(newPresubmits, presubmit)
	}
	return newPresubmits
}

func generatePostsubmits(j config.JobConfig, version, repository string) []config.Postsubmit {
	newPostsubmits := []config.Postsubmit{}
	for _, postsubmit := range j.PostsubmitsStatic[repository] {
		if postsubmit.Annotations[ForkAnnotation] != "true" {
			continue
		}
		delete(postsubmit.Annotations, ForkAnnotation)
		postsubmit.Annotations[ForkedAnnotation] = "true"
		// Check if branch has no forked config yet
		postsubmit.Name = postsubmit.Name + JobNameSuffix + strings.ReplaceAll(version, ".", "-")
		postsubmit.Branches = []string{BranchPrefix + version}
		postsubmit.SkipBranches = nil

		newPostsubmits = append(newPostsubmits, postsubmit)
	}
	return newPostsubmits
}

func generatePeriodics(j config.JobConfig, version, repository string) []config.Periodic {
	newPeriodics := []config.Periodic{}
	for _, periodic := range j.Periodics {
		if periodic.Annotations[ForkAnnotation] != "true" {
			continue
		}

		isRelatedToRepo := false
		for _, ref := range periodic.ExtraRefs {
			if ref.OrgRepoString() != repository {
				continue
			}
			isRelatedToRepo = true
			ref.BaseRef = BranchPrefix + version
		}

		if !isRelatedToRepo {
			continue
		}

		newPeriodics = append(newPeriodics, periodic)

	}

	return newPeriodics
}

func forkConfig(files []string, baseRepoJobsDirPath, repo, version string) {
	fileVersion := strings.ReplaceAll(version, ".", "-")
	repoString := strings.ReplaceAll(repo, "/", "-")

	presubmits := []config.Presubmit{}
	postsubmits := []config.Postsubmit{}
	periodics := []config.Periodic{}

	for _, file := range files {
		j, err := config.ReadJobConfig(file)
		if err != nil {
			log.Fatalf("Couldn't read jobConfig: %v\n", err)
		}
		presubmits = append(presubmits, generatePresubmits(j, version, repo)...)
		postsubmits = append(postsubmits, generatePostsubmits(j, version, repo)...)
		periodics = append(periodics, generatePeriodics(j, version, repo)...)
	}
	targetDir := path.Join(baseRepoJobsDirPath, ForkDir)
	newFileName := repoString + "-" + fileVersion + ".yaml"

	output := path.Join(targetDir, filepath.Base(newFileName))

	payload := make(map[string]interface{})

	if len(presubmits) != 0 {
		payload["presubmits"] = presubmits
	}

	if len(postsubmits) != 0 {
		payload["postsubmits"] = postsubmits
	}

	if len(periodics) != 0 {
		payload["periodics"] = periodics
	}

	if len(presubmits) == 0 && len(postsubmits) == 0 && len(periodics) == 0 {
		log.Printf("%v has no forkable configs for version %v\n", repo, version)
		return
	}

	err := os.MkdirAll(targetDir, os.ModePerm)
	if err != nil {
		log.Fatalf("Couldn't create Directory: %v\n", err)
	}

	data, err := yaml.Marshal(payload)
	if err != nil {
		log.Fatalf("Couldn't marshal presubmits: %v\n", err)
	}

	newf, err := os.Create(output)
	if err != nil {
		log.Fatalf("Couldn't create outputFile: %v\n", err)
	}
	defer newf.Close()

	if _, err = newf.Write(data); err != nil {
		log.Fatalf("Couldn't write to outputFile: %v\n", err)
	}
	log.Printf("%v has forked %v Presubmits, %v Postsubmits, %v Periodics for version %v into %v\n",
		repo,
		len(presubmits),
		len(postsubmits),
		len(periodics),
		version,
		output,
	)
}

func removeDeprecatedConfigs(repo, baseRepoJobsDirPath string, versions []string) {

	repoString := strings.ReplaceAll(repo, "/", "-")
	log.Printf("repoString: %v\n", repoString)
	forkedDir := path.Join(baseRepoJobsDirPath, ForkDir)
	forkedFiles, err := ghi.GetFileNames(forkedDir, []string{}, false)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf("Couldn't read from forked files Directory: %v\n", err)
	}
	log.Printf("forkedFiles: %v\n", forkedFiles)

	for _, forkedFile := range forkedFiles {
		if !strings.Contains(forkedFile, repoString) {
			log.Printf("%v didn't match %v\n", forkedFile, repoString)
			continue
		}
		log.Printf("%v matched %v\n", forkedFile, repoString)
		// branched File belongs to repo
		matches := false
		for _, version := range versions {
			fileVersion := strings.ReplaceAll(version, ".", "-")

			if strings.Contains(forkedFile, repoString+"-"+fileVersion) {
				// branched File has corresponding branch
				matches = true
				break
			}

		}

		if !matches {
			// File is deprecated and has no corresponding branch to it anymore
			log.Printf("Removing %v, because it's config is deprecated\n", forkedFile)
			if err = os.Remove(forkedFile); err != nil {
				log.Printf("warn: couldn't remove file %v\n", err)
			}
		}
	}
}

func readReposFromJobs(files []string) []string {
	repos := []string{}
	for _, file := range files {
		j, err := config.ReadJobConfig(file)
		if err != nil {
			log.Fatalf("Couldn't read jobConfig: %v\n", err)
		}
		for repo := range j.PresubmitsStatic {
			if !contains(repos, repo) {
				repos = append(repos, repo)
			}
		}

		for repo := range j.PostsubmitsStatic {
			if !contains(repos, repo) {
				repos = append(repos, repo)
			}
		}

		for _, periodic := range j.Periodics {
			for _, ref := range periodic.ExtraRefs {
				repo := ref.OrgRepoString()

				if !contains(repos, repo) {
					repos = append(repos, repo)
				}
			}
		}
	}

	return repos
}

func generateForkedConfigurations(baseRepo *ghi.Repository) error {
	baseRepoJobsDirPath := path.Join(baseRepo.RepoClient.Directory(), o.configsPath)
	fileNames, err := ghi.GetFileNames(baseRepoJobsDirPath, []string{ForkDir}, o.ghio.Recursive)
	log.Printf("Files in baseRepoJobsDirPath: %v\n", fileNames)
	if err != nil {
		return err
	}

	repos := readReposFromJobs(fileNames)
	for _, repoString := range repos {
		rep, err := ghi.NewRepository(repoString)
		if err != nil {
			return err
		}

		releaseBranches, err := rep.GetMatchingBranches(BranchPrefix + `v\d+\.\d+`)
		if err != nil {
			return err
		}

		log.Printf("There are %v release branches for repo %v\n", len(releaseBranches), rep.FullRepoName)
		versions := generateVersionsFromBranches(releaseBranches, BranchPrefix)
		log.Printf("Corresponding versions: %v\n", versions)
		// Check if there is a release branch without a corresponding forked config
		for _, version := range versions {
			forkConfig(fileNames, baseRepoJobsDirPath, rep.FullRepoName, version)
		}
		removeDeprecatedConfigs(rep.FullRepoName, baseRepoJobsDirPath, versions)
	}
	return nil
}
