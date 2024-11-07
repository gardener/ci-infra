# Shared Renovate Config Presets

This directory hosts [shared config presets](https://docs.renovatebot.com/config-presets/) for renovate to reuse common configuration across gardener repositories.
One example is teaching renovate to work with tide when `automerge=true` is set.

Config presets are modular and should be kept as small as possible so that consuming repositories can precisely choose which presets to use.
Typically, config presets are consumed from the default branch of the hosting repository.
Hence, be mindful when changing the presets in this repository.

## Using Config Presets From This Repository

When you want to consume a renovate preset from this repository, pick a preset file name and add a corresponding `extends` item to the renovate config in your repository as follows:

```json5
{
  extends: [
    'github>gardener/ci-infra//config/renovate/automerge-with-tide.json5',
  ]
}
```

When using a [parametrized preset](https://docs.renovatebot.com/config-presets/#preset-parameters), add an `extends` item like this:

```json5
{
  extends: [
    'github>gardener/ci-infra//config/renovate/imagevector.json5(^imagevector\/images.yaml$)',
  ]
}
```


Note that consuming repositories hosted on GitHub could also use a `local>` preset rule.
However, `local>` preset rules are not supported with `--platform=local`, i.e., when executing a [local renovate dry-run](../../README.md#local-dry-run).

## Testing Config Preset Changes

Renovate always fetches config presets remotely, i.e., from GitHub – even if the referenced repository is the repository your working with locally.
Because of this, you have to merge the preset's config directly into the renovate config file, when you want to test your changes to config presets in this repository using a [local dry-run](../../README.md#local-dry-run).
As long as you have a remote reference in the `extends` array, renovate will not pick up your local changes.

Also note that `renovate-config-validator` will not validate the referenced config presets.
This is a good thing, because we only need one pull request to introduce and use presets in this repository.
However, it also means that erroneous reference will only be noticed once renovate runs and adds error messages to the dependency dashboard.
