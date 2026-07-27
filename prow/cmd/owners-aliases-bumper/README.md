# Owners-Aliases-Bumper

## Functionality
The `owners-aliases-bumper` keeps the `OWNERS_ALIASES` files of your repositories in sync with the GitHub team definitions in a [Peribolos](https://docs.prow.k8s.io/docs/components/cli-tools/peribolos/) config.

For every alias that shares its name with a Peribolos team, the tool rebuilds the alias membership from that team's `members` and `maintainers` (recursively including child teams). It then compares this against the alias membership currently committed in each repository's `OWNERS_ALIASES` file and, where they differ, pushes a branch and opens a pull request against the repo.

A few things to be aware of:

- **Only repos defined in the Peribolos config are checked.** The tool iterates the orgs and repos in `--peribolos-conf`; anything not listed there is never looked at.
- **Only existing aliases are reconciled.** Aliases are never added or removed — the tool only updates the membership of aliases that already exist in a repo's `OWNERS_ALIASES`.
- **Repos without an `OWNERS_ALIASES` file are skipped.**

If more than one open PR from the bumper branch is found on a repo, the newest is kept and the rest are closed automatically (they are assumed to have been opened manually or by a previous run).

## Modes

### Dry-run (default)
Without `--confirm`, the tool only reports what *would* change. No commits or PRs are created. Use this to preview the impact before applying.

### Apply
Pass `--confirm` to actually push the branch and open PRs. In this mode the tool requires an authenticated GitHub client capable of resolving the bot user identity used for the git commit author.

## Flags

| Flag | Required | Default | Description |
| --- | --- | --- | --- |
| `--peribolos-conf` | yes | | Path to the Peribolos config. Only repos defined here are managed, and aliases whose name matches a GitHub team are populated from that team. |
| `--confirm` | no | `false` | Apply changes (commit, push, open PRs). Without it, the tool runs in dry-run mode and only prints what would change. |
| `--skip-repos` | no | | Comma-separated list of repos to exclude, in `<org>/<repo>` format. By default every repo in the Peribolos config is managed. |
| `--log-level` | no | `info` | Logging level (one of the standard `logrus` levels, e.g. `debug`, `info`, `warn`, `error`). |

The commit and pull request text can be overridden. Titles and the branch name are inline strings; the commit and PR bodies can be given either inline or read from a file (the inline and `-file` variants are mutually exclusive). When none is set, a built-in default is used.

| Flag | Required | Default | Description |
| --- | --- | --- | --- |
| `--pr-branch` | no | `owners-aliases-bumper` | Name of the branch pushed to the repo and used as the PR head. |
| `--pr-title` | no | built-in | Title of the opened pull request. |
| `--commit-title` | no | built-in | Title (subject line) of the commit. |
| `--commit-body` | no | built-in | Commit body, inline. Mutually exclusive with `--commit-body-file`. |
| `--commit-body-file` | no | | Path to a file whose contents are used as the commit body. Mutually exclusive with `--commit-body`. |
| `--pr-body` | no | built-in | PR body, inline. Mutually exclusive with `--pr-body-file`. |
| `--pr-body-file` | no | | Path to a file whose contents are used as the PR body. Mutually exclusive with `--pr-body`. |

In addition, the tool accepts the standard Prow [`flagutil.GitHubOptions`](https://pkg.go.dev/sigs.k8s.io/prow/pkg/flagutil#GitHubOptions) flags for authenticating the GitHub and git clients (e.g. `--github-token-path` or GitHub App credentials, `--github-endpoint`).

## Examples

Preview changes for all repos in a Peribolos config:

```sh
owners-aliases-bumper \
  --peribolos-conf=/path/to/peribolos.yaml
```

Apply changes, skipping a couple of repos, using a token:

```sh
owners-aliases-bumper \
  --peribolos-conf=/path/to/peribolos.yaml \
  --github-token-path=/etc/github/token \
  --skip-repos=my-org/repo-a,my-org/repo-b \
  --confirm
```

Run with verbose logging to see per-alias diffs:

```sh
owners-aliases-bumper \
  --peribolos-conf=/path/to/peribolos.yaml \
  --log-level=debug
```
