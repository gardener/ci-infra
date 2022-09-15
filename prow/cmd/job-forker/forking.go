package main

import (
	"path"

	"log"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/test-infra/prow/config"

	"sigs.k8s.io/yaml"
)

func generatePresubmits(j config.JobConfig, version, repository string) []config.Presubmit {
	newPresubmits := []config.Presubmit{}
	for repo, presubmits := range j.PresubmitsStatic {
		if repo != repository {
			continue
		}
		for _, presubmit := range presubmits {
			if presubmit.Annotations[forkAnnotation] != "true" {
				continue
			}
			delete(presubmit.Annotations, forkAnnotation)
			presubmit.Annotations[forkedAnnotation] = "true"
			// Check if branch has no forked config yet
			presubmit.Name = presubmit.Name + jobNameSuffix + strings.ReplaceAll(version, ".", "-")
			presubmit.Branches = []string{branchPrefix + version}
			presubmit.SkipBranches = nil

			newPresubmits = append(newPresubmits, presubmit)
		}
	}
	return newPresubmits
}

func generatePostsubmits(j config.JobConfig, version, repository string) []config.Postsubmit {
	newPostsubmits := []config.Postsubmit{}
	for repo, postsubmits := range j.PostsubmitsStatic {
		if repo != repository {
			continue
		}
		for _, postsubmit := range postsubmits {
			if postsubmit.Annotations[forkAnnotation] != "true" {
				continue
			}

			postsubmit.Name = postsubmit.Name + jobNameSuffix + strings.ReplaceAll(version, ".", "-")
			postsubmit.Branches = []string{branchPrefix + version}
			postsubmit.SkipBranches = nil

			newPostsubmits = append(newPostsubmits, postsubmit)
		}
	}
	return newPostsubmits
}

func generatePeriodics(j config.JobConfig, version, repository string) []config.Periodic {
	newPeriodics := []config.Periodic{}
	for _, periodic := range j.Periodics {
		if periodic.Annotations[forkAnnotation] != "true" {
			continue
		}
		
		isRelatedToRepo := false
		for _, ref := range periodic.ExtraRefs {
			if ref.OrgRepoString() != repository {
				continue
			}
			isRelatedToRepo = true
			ref.BaseRef = branchPrefix + version
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
	targetDir := path.Join(baseRepoJobsDirPath, forkDir)
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

func hasForkedConfig(file, version string) bool {
	fileBaseName := strings.ReplaceAll(filepath.Base(file), filepath.Ext(file), "")
	outputPath := filepath.Dir(file)
	forkedFolder := path.Join(outputPath, forkDir)
	versionFileRep := strings.ReplaceAll(version, ".", "-")
	return checkFileExists(path.Join(forkedFolder, fileBaseName+"-"+versionFileRep+filepath.Ext(file)))
}

func removeDeprecatedConfigs(repo, baseRepoJobsDirPath string, versions []string) {

	repoString := strings.ReplaceAll(repo, "/", "-")
	log.Printf("repoString: %v\n", repoString)
	forkedDir := baseRepoJobsDirPath + "/" + forkDir
	forkedFiles := getFileNames(forkedDir)
	log.Printf("forkedFiles: %v\n", forkedFiles)

	for _, forkedFile := range forkedFiles {
		if strings.Contains(forkedFile, repoString) {
			log.Printf("%v matched %v\n", forkedFile, repoString)
			// branched File belongs to repo
			matches := false
			for _, version := range versions {
				fileVersion := strings.ReplaceAll(version, ".", "-")

				if strings.Contains(forkedFile,repoString + "-" + fileVersion) {
					// branched File has corresponding branch
					matches = true
					break
				}

			}

			if !matches {
				// File is deprecated and has no corresponding branch to it anymore
				log.Printf("Removing %v, because it's config is deprecated\n", forkedFile)
				os.Remove(forkedFile)
			}
		} else {
			log.Printf("%v didn't match %v\n", forkedFile, repoString)
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
			if contains(repos, repo) {
				continue
			} else {
				repos = append(repos, repo)
			}
		}

		for repo := range j.PostsubmitsStatic {
			if contains(repos, repo) {
				continue
			} else {
				repos = append(repos, repo)
			}
		}

		for _, periodic := range j.Periodics {
			for _, ref := range periodic.ExtraRefs {
				repo := ref.OrgRepoString()

				if contains(repos, repo) {
					continue
				} else {
					repos = append(repos, repo)
				}
			}
		}
	}

	return repos
}