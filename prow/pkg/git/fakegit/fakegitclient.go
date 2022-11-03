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

package fakegit

import "k8s.io/test-infra/prow/git/v2"

// FakeGitClientFactory fakes the prow git client factory
type FakeGitClientFactory struct {
	DirectoryString string
}

// FakeRepoClient fakes the prow git RepoClient
type FakeRepoClient struct {
	FakePublisher
	FakeInteractor
}

// FakePublisher fakes the prow git Publisher
type FakePublisher struct{}

// FakeInteractor fakes the prow git Publisher
type FakeInteractor struct{}

// FakeCommitClient fakes the gardener/ci-infra githubinteractor CommitClient
type FakeCommitClient struct{}

// Commit is a fake for Commit
func (gc *FakeCommitClient) Commit(repoClient git.RepoClient, message string) error {
	return nil
}

// ClientFromDir is a fake for ClientFromDir
func (fgcf *FakeGitClientFactory) ClientFromDir(org, repo, dir string) (git.RepoClient, error) {
	repoClient := new(FakeRepoClient)
	return repoClient, nil
}

// ClientFor is a fake for ClientFor
func (fgcf *FakeGitClientFactory) ClientFor(org, repo string) (git.RepoClient, error) {
	repoClient := new(FakeRepoClient)
	return repoClient, nil
}

// Clean is a fake for Clean
func (fgcf *FakeGitClientFactory) Clean() error {
	return nil
}

//--------------------------------------------------------------------------------------//

// Commit is a fake for Commit
func (fp *FakePublisher) Commit(title, body string) error {
	return nil
}

// PushToFork is a fake for PushToFork
func (fp *FakePublisher) PushToFork(branch string, force bool) error {
	return nil
}

// PushToNamedFork is a fake for PushToNamedFork
func (fp *FakePublisher) PushToNamedFork(forkName, branch string, force bool) error {
	return nil
}

// PushToCentral is a fake for PushToCentral
func (fp *FakePublisher) PushToCentral(branch string, force bool) error {
	return nil
}

//--------------------------------------------------------------------------------------//

// Directory is a fake for Directory
func (fi *FakeInteractor) Directory() string {
	return ""
}

// Clean is a fake for Clean
func (fi *FakeInteractor) Clean() error {
	return nil
}

// CommitExists is a fake for CommitExists
func (fi *FakeInteractor) CommitExists(sha string) (bool, error) {
	return true, nil
}

// ResetHard is a fake for ResetHard
func (fi *FakeInteractor) ResetHard(commitlike string) error {
	return nil
}

// IsDirty is a fake for IsDirty
func (fi *FakeInteractor) IsDirty() (bool, error) {
	return false, nil
}

// Checkout is a fake for Checkout
func (fi *FakeInteractor) Checkout(commitlike string) error {
	return nil
}

// RevParse is a fake for RevParse
func (fi *FakeInteractor) RevParse(commitlike string) (string, error) {
	return "", nil
}

// BranchExists is a fake for BranchExists
func (fi *FakeInteractor) BranchExists(branch string) bool {
	return true
}

// CheckoutNewBranch is a fake for CheckoutNewBranch
func (fi *FakeInteractor) CheckoutNewBranch(branch string) error {
	return nil
}

// Merge is a fake for Merge
func (fi *FakeInteractor) Merge(commitlike string) (bool, error) {
	return true, nil
}

// MergeWithStrategy is a fake for MergeWithStrategy
func (fi *FakeInteractor) MergeWithStrategy(commitlike, mergeStrategy string, opts ...git.MergeOpt) (bool, error) {
	return true, nil
}

// MergeAndCheckout is a fake for MergeAndCheckout
func (fi *FakeInteractor) MergeAndCheckout(baseSHA string, mergeStrategy string, headSHAs ...string) error {
	return nil
}

// Am is a fake for Am
func (fi *FakeInteractor) Am(path string) error {
	return nil
}

// Fetch is a fake for Fetch
func (fi *FakeInteractor) Fetch(arg ...string) error {
	return nil
}

// FetchRef is a fake for FetchRef
func (fi *FakeInteractor) FetchRef(refspec string) error {
	return nil
}

// FetchFromRemote is a fake for FetchFromRemote
func (fi *FakeInteractor) FetchFromRemote(remote git.RemoteResolver, branch string) error {
	return nil
}

// CheckoutPullRequest is a fake for CheckoutPullRequest
func (fi *FakeInteractor) CheckoutPullRequest(number int) error {
	return nil
}

// Config is a fake for Config
func (fi *FakeInteractor) Config(args ...string) error {
	return nil
}

// Diff is a fake for Diff
func (fi *FakeInteractor) Diff(head, sha string) (changes []string, err error) {
	return []string{}, nil
}

// MergeCommitsExistBetween is a fake for MergeCommitsExistBetween
func (fi *FakeInteractor) MergeCommitsExistBetween(target, head string) (bool, error) {
	return true, nil
}

// ShowRef is a fake for ShowRef
func (fi *FakeInteractor) ShowRef(commitlike string) (string, error) {
	return "", nil
}
