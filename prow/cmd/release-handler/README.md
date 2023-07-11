# Release-Handler
## Functionality
The `release-handler` handles release specific tasks of a repository. It runs as a `postsubmit` job in prow and identifies release activities by checking commited changes of a configurable `VERSION` file. The `VERSION` file must include a semver compatible version in its first line.

Currently, it identifies four cases:
- `newRelease`: when the version increases from a dev to a final version like `v1.75.0-dev -> v1.75.0`.
- `nextDevCycle`: when the version increases from a final to a higher dev version like `v1.75.0 -> v1.75.1-dev`
- `prepareNewPatchVersion`: when the version increases from one dev version to another with a higher patch version like `v1.75.0-dev -> v1.75.1-dev`
- `prepareNewMajorMinorVersion`: when the version increases from one dev version to another with a higher major or minor version like `v1.75.0-dev -> v1.76.0-dev`

When it identifies the `prepareNewMajorMinorVersion` case it creates a new release branch using a configurable pattern at the previous commit. The development could continue working on the main branch while the release verification activities are using the release branch.
An example, when the version of the main branch increases from `v1.75.0-dev` to `v1.76.0-dev` release-handler creates a release branch for `v1.75`.
