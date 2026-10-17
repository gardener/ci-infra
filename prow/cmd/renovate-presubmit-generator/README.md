# renovate-presubmit-generator

## Functionality

The `renovate-presubmit-generator` generates the Prow presubmits that run
`renovate-config-validator` against changes to Renovate configuration files. It creates one
`pull-<repo>-check-renovate-config` job for every **public, non-archived, non-fork**
repository of the configured GitHub org (GitHub meta-repositories whose name starts with a
dot, such as `.github`, are skipped) and writes them to
`config/jobs/common/renovate-presubmits.yaml`.

Prow has no org-wide presubmit, so a job has to exist per repository. This tool keeps that
list complete and in sync with the org as repositories are added or removed.

The Renovate image used by the jobs is managed by Renovate itself (docker datasource). To
avoid reverting those bumps, the generator reuses the image already pinned in the existing
output file and only falls back to `--renovate-image` when the file does not exist yet. If
neither source provides an image, the generator exits with an error (there is no hardcoded
default, as it would go stale quickly).

## Usage

The primary use case is the `ci-renovate-presubmit-generator` periodic, which runs the
generator in apply mode: it clones `gardener/ci-infra`, regenerates the file and opens (or
updates) a pull request when the repository set changed.

```sh
renovate-presubmit-generator \
  --confirm \
  --github-token-path=/etc/github-token/token
```

Without `--confirm` the file is only written locally (dry-run), which is useful to preview
changes. Listing the org repositories needs a token with read access to the org. The
`hack/generate-renovate-presubmits.sh` wrapper runs exactly this:

```sh
GITHUB_TOKEN_PATH=/path/to/token hack/generate-renovate-presubmits.sh
# equivalent to:
renovate-presubmit-generator --github-token-path=/path/to/token
```

Authentication also works as a GitHub App via `--github-app-id` and
`--github-app-private-key-path`.

Use `--skip-repos=<org>/<repo>` (repeatable) to exclude individual repositories, `--org` to
target a different GitHub org, and `--repo` to change the repository the config lives in and
the pull request is opened against (defaults to `ci-infra`).
