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
func (gc *FakeCommitClient) Commit(_, _, _, _ string, _ bool) error {
	return nil
}

// ClientFromDir is a fake for ClientFromDir
func (fgcf *FakeGitClientFactory) ClientFromDir(_, _, _ string) (git.RepoClient, error) {
	repoClient := new(FakeRepoClient)
	return repoClient, nil
}

// ClientFor is a fake for ClientFor
func (fgcf *FakeGitClientFactory) ClientFor(_, _ string) (git.RepoClient, error) {
	repoClient := new(FakeRepoClient)
	return repoClient, nil
}

// ClientForWithRepoOpts is a fake for ClientForWithRepoOpts
func (fgcf *FakeGitClientFactory) ClientForWithRepoOpts(_, _ string, _ git.RepoOpts) (git.RepoClient, error) {
	repoClient := new(FakeRepoClient)
	return repoClient, nil
}

// Clean is a fake for Clean
func (fgcf *FakeGitClientFactory) Clean() error {
	return nil
}

//--------------------------------------------------------------------------------------//

// Commit is a fake for Commit
func (fp *FakePublisher) Commit(_, _ string) error {
	return nil
}

// PushToFork is a fake for PushToFork
func (fp *FakePublisher) PushToFork(_ string, _ bool) error {
	return nil
}

// PushToNamedFork is a fake for PushToNamedFork
func (fp *FakePublisher) PushToNamedFork(_, _ string, _ bool) error {
	return nil
}

// PushToCentral is a fake for PushToCentral
func (fp *FakePublisher) PushToCentral(_ string, _ bool) error {
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
func (fi *FakeInteractor) CommitExists(_ string) (bool, error) {
	return true, nil
}

// ResetHard is a fake for ResetHard
func (fi *FakeInteractor) ResetHard(_ string) error {
	return nil
}

// IsDirty is a fake for IsDirty
func (fi *FakeInteractor) IsDirty() (bool, error) {
	return false, nil
}

// Checkout is a fake for Checkout
func (fi *FakeInteractor) Checkout(_ string) error {
	return nil
}

// RevParse is a fake for RevParse
func (fi *FakeInteractor) RevParse(_ string) (string, error) {
	return "", nil
}

// RevParseN is a fake for RevParseN
func (fi *FakeInteractor) RevParseN(_ []string) (map[string]string, error) {
	return nil, nil
}

// BranchExists is a fake for BranchExists
func (fi *FakeInteractor) BranchExists(_ string) bool {
	return true
}

// ObjectExists is a fake for ObjectExists
func (fi *FakeInteractor) ObjectExists(_ string) (bool, error) {
	return true, nil
}

// CheckoutNewBranch is a fake for CheckoutNewBranch
func (fi *FakeInteractor) CheckoutNewBranch(_ string) error {
	return nil
}

// Merge is a fake for Merge
func (fi *FakeInteractor) Merge(_ string) (bool, error) {
	return true, nil
}

// MergeWithStrategy is a fake for MergeWithStrategy
func (fi *FakeInteractor) MergeWithStrategy(_, _ string, _ ...git.MergeOpt) (bool, error) {
	return true, nil
}

// MergeAndCheckout is a fake for MergeAndCheckout
func (fi *FakeInteractor) MergeAndCheckout(_, _ string, _ ...string) error {
	return nil
}

// Am is a fake for Am
func (fi *FakeInteractor) Am(_ string) error {
	return nil
}

// Fetch is a fake for Fetch
func (fi *FakeInteractor) Fetch(_ ...string) error {
	return nil
}

// FetchRef is a fake for FetchRef
func (fi *FakeInteractor) FetchRef(_ string) error {
	return nil
}

// FetchFromRemote is a fake for FetchFromRemote
func (fi *FakeInteractor) FetchFromRemote(_ git.RemoteResolver, _ string) error {
	return nil
}

// CheckoutPullRequest is a fake for CheckoutPullRequest
func (fi *FakeInteractor) CheckoutPullRequest(_ int) error {
	return nil
}

// Config is a fake for Config
func (fi *FakeInteractor) Config(_ ...string) error {
	return nil
}

// Diff is a fake for Diff
func (fi *FakeInteractor) Diff(_, _ string) (changes []string, err error) {
	return []string{}, nil
}

// MergeCommitsExistBetween is a fake for MergeCommitsExistBetween
func (fi *FakeInteractor) MergeCommitsExistBetween(_, _ string) (bool, error) {
	return true, nil
}

// ShowRef is a fake for ShowRef
func (fi *FakeInteractor) ShowRef(_ string) (string, error) {
	return "", nil
}
