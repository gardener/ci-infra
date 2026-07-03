# peribolos-checkconfig

`peribolos-checkconfig` is a static linter for peribolos org
configuration files. It validates a peribolos `org.yaml` against a set of
consistency rules **without needing a GitHub token** — making it a natural fit
for a presubmit prow job on the repo that stores your org config.

## What it checks

Given a peribolos `org.FullConfig`, for each org it verifies:

- **YAML is well-formed** and unmarshals cleanly into the peribolos schema.
- **Minimum admins** — each org has at least `--min-admins` admins (default `5`,
  minimum `2`).
- **Required admins present** — every user passed via `--required-admins`
  appears in the org's `admins` list.
- **No dual-role users** — no user is listed as both an `admin` and a `member`
  of the same org. Comparison is done after `github.NormLogin` so casing
  differences (e.g. `Alice` vs `alice`) are caught.
- **No dual-role team users** — no user is listed as both `member` and
  `maintainer` of the same team. Peribolos itself only enforces this at apply
  time when `--fix-team-members` is set, so bad configs can otherwise
  accumulate silently. Normalized the same way as the org check.
- **Team members are org members** — every user listed as a team `member` or
  `maintainer` (recursively across `children`) is also listed at the org level
  as `admin` or `member`.
- **Unique team names** — team `name` + all entries in `previously` are unique
  within an org.
- **Nested teams are `closed`** — any team that has a parent or has children
  must have `privacy: closed`. Teams with unset `privacy` are allowed (peribolos
  fills in `closed` at apply time).
- **Unique repo names** — repo `name` + all entries in `previously` are unique
  within an org, compared case-insensitively (GitHub repo names are
  case-insensitive).
- **No `archived: false`** — repos configured with `archived: false` are
  rejected, because the GitHub API cannot unarchive a repository.

Checks that require live GitHub state (`--maximum-removal-delta`, `require-self`,
current-org reconciliation, invitation flow) are intentionally out of scope —
they belong to peribolos itself.

## Usage

```
peribolos-checkconfig \
  --config-path=./org.yaml \
  --min-admins=5 \
  --required-admins=alice --required-admins=bob
```

### Flags

| Flag                | Default      | Description                                                         |
| ------------------- | ------------ | ------------------------------------------------------------------- |
| `--config-path`     | _(required)_ | Path to peribolos org config YAML.                                  |
| `--min-admins`      | `5`          | Fail if any org has fewer admins than this. Must be ≥ 2.            |
| `--required-admins` | _(none)_     | Repeatable; usernames that must appear as admins in every org.      |
| `--log-level`       | `info`       | One of `panic`, `fatal`, `error`, `warn`, `info`, `debug`, `trace`. |

### Exit codes

- **0** — all orgs pass every check.
- **non-zero** — one or more checks failed; the failing checks are aggregated
  and logged before exit.


## Development

```bash
# Build
go build ./prow/cmd/peribolos-checkconfig

# Run tests
go test ./prow/cmd/peribolos-checkconfig
```

See [main_test.go](main_test.go) for the check-by-check test matrix.
