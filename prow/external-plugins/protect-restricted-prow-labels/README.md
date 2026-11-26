# protect-restricted-prow-labels

`protect-restricted-prow-labels` is a Prow external plugin that enforces that certain labels are
only ever set by Prow's own automation (the GitHub App this plugin runs as, referred to as the
_Prow user_).

## Why

The upstream [`approve`](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/approve)
and [`lgtm`](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/lgtm) plugins own the
`approved` and `lgtm` labels. However, a user with write access can still add these labels
manually via the GitHub UI and thereby bypass the review process:

- The `approve` plugin reacts to a manually added `approved` label only by posting a
  notification that _"Approval requirements bypassed by manually added approval."_. It does not
  remove the label.
- The `lgtm` plugin does nothing at all when the `lgtm` label is added manually.

This plugin closes that gap.

## What it does

On every `pull_request` `labeled` event it checks the added label:

- If the label was added by the Prow user, nothing happens.
- Otherwise, if the label is **protected** and the user is **not allowed** to set it, the plugin
  - removes the label again,
  - deletes the `approve` plugin's manual-approval notification (only for the `approved` label),
  - and posts a comment telling the user that manually setting the label is not allowed
    in this GitHub organization.

To keep the noise down, a previous warning for the same label is deleted before a new one is
posted, so there is at most one warning per label on a pull request.

## Periodic reconciliation

GitHub webhook deliveries are acknowledged before they are processed and are never redelivered.
If enforcing a label fails after the acknowledgement — e.g. because of a transient GitHub error
or a restart while the event was being handled — that label would otherwise stay forever.

To close this gap, the plugin periodically re-scans the open pull requests in the repos where it
is enabled (`--update-period`, default `1m`, `0` disables it). For every protected label still
present it determines, via the issue events, who last added it and enforces the same rule as the
webhook path. The enforcement is idempotent, so the reconciliation and the webhook handler can
run against the same pull request without conflicting. The period is kept short on purpose: Tide
can merge a pull request within a couple of minutes once it is approved, so a slower safety net
could let a manually labeled pull request merge before the label is removed.

To keep the GitHub API usage low, the reconciliation uses one search per scope (all protected
labels OR-ed in a single `label:` query) and only fetches the issue events of pull requests whose
`updated_at` changed since the last successful pass (minus a few minutes, as the GitHub search
index can lag behind). On the first pass after a start it processes all pull requests the search
returns. It also does a full pass whenever the plugin-relevant configuration changes, since a new
label, repo or allowed user can bring pull requests into scope that an earlier pass skipped.

In an HA setup (multiple replicas), the reconciliation runs on a single replica at a time, chosen
via leader election (a `Lease` in the pod's namespace), so the replicas do not reconcile in
parallel. The webhook handler keeps running on all replicas.

## Protected labels

- `approved` and `lgtm` are **always** protected and may be set by the Prow user only.
- Any label configured under the `label` plugin's
  [`restricted_labels`](https://github.com/kubernetes-sigs/prow/blob/main/pkg/plugins/config.go)
  is protected as well. For those labels, the `allowed_users` and `allowed_teams` configured
  there may set the label in addition to the Prow user.

The plugin reads this configuration from the standard Prow `plugins.yaml`, so no separate
configuration file is required. Example:

```yaml
label:
  restricted_labels:
    gardener:
    - label: skip-review
      allowed_users:
      - renovate[bot]

external_plugins:
  gardener:
  - name: protect-restricted-prow-labels
    events:
    - pull_request
```

## Notes

- Deleting the `approve` plugin's manual-approval notification is best-effort: the approve
  plugin reacts to the same label event, so the notification might not exist yet when this
  plugin runs. If the notification appears afterwards, the approve plugin removes it itself once
  the `approved` label is gone.
