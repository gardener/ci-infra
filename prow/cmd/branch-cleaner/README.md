# Branch-Cleaner
## Functionality
The `branch-cleaner` scans GitHub repositories for branches of a specific pattern and deletes them.

When the `--keep-branches=N` is set, it keeps the last `N` matching branches sorted in alphabetical order. 
When `--ignore-open-prs=false` it checks the branches for open PRs before deletion. If there are any, the branch won't be deleted. 
When `--release-branch-mode=true` it checks release branch names for semver versions and searches for an existing Github release tag (`v{major}.{minor}.0`) for this version. Release branches which do not have a version yet are neither counted nor considered for deletion, because they are not released yet. If the branch names to not include a semver version `branch-cleaner` fails in this mode.
