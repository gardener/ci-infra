# Cherrypicker

**Note: This is a fork of [cherrypicker from kubernetes/test-infra (commit 78d91d6)](https://github.com/kubernetes/test-infra/tree/78d91d66742286ba1c8d003fa3b328184869867d/prow/external-plugins/cherrypicker) repository.
It changes the release note regexp to fit the gardener release note format**

Cherrypicker is an external prow plugin that can also run as a standalone bot.
It automates cherry-picking merged PRs into different branches. Cherrypicks are
triggered from either comments or labels in GitHub PRs that need to be cherrypicked.

For comments:

```
/cherrypick release-1.10
```

The above comment will result in opening a new PR against the `release-1.10` branch
once the PR where the comment was made gets merged or is already merged.

To use label, you need to apply labels that contain the name of the branch in the form:

```
cherrypick/XXX
```

where XXX is the name of the branch.

The bot uses its own fork to push patches that need to be cherry-picked and opens
PRs out of those patches. The fork is created automatically by the bot so there is
no need to set it up manually. 

Required scopes for the oauth token that need to be used are `read:org` and `repo`.
