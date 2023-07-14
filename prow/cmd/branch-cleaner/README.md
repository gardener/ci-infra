# Branch-Cleaner
## Functionality
The `branch-cleaner` scans GitHub repositories for branches of a specific pattern and deletes them.

When the `--keep-branches=N` is set, it keeps the last `N` matching branches sorted in alphabetical order. If `--ignore-open-prs=false` it checks the branches for open PRs before deletion. If there are any, the branch won't be deleted.
