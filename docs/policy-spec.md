# Sluice policy file: v1alpha1

Status: draft, 2026-11-12. License: Apache-2.0.

The policy file tells the gate what an agent-authored change must prove before
it merges. It lives at `.sluice/policy.yaml` and is always read from the
repo's default branch, never from the PR under evaluation.

## Example

```yaml
version: v1alpha1

# Branches the gate merges into. Anything else is ignored.
branches: [main]

# Check runs count as evidence only from these apps.
trusted_checks:
  - app: github-actions

tiers:
  auto:
    require:
      checks: [build, test]
      max_changed_lines: 200
  review:
    require:
      checks: [build, test]
      review_app: healthy
      independent_review: pass
      max_changed_lines: 800
  human:
    require:
      checks: [build, test]
      approval: passkey

# First match per file; the change gets the strictest tier among its files.
rules:
  - paths: ["docs/**", "**/*.md"]
    tier: auto
  - paths: ["migrations/**", "deploy/**", "**/*secret*"]
    tier: human
  - paths: ["**"]
    tier: review

# Paths the courier will not push without human approval of the diff.
# `.github/**` and `.sluice/**` are always protected, listed or not.
protected_paths:
  - "Dockerfile"

tainted_runs:
  max_tier: human        # a tainted run is never merged below this tier

freeze:
  - when: slo_burn
    slo: availability
    above: 2.0           # burn-rate multiple
  - when: alert_firing
    alert: DeployInProgress

watch:
  window: 30m
  regress_on: [slo_burn, alert_firing]
  on_regress: revert_pr

budgets:
  per_run: { cost_usd: 5, wall_time: 45m }
  per_day: { cost_usd: 40, runs: 30 }
```

## Semantics

**Default deny.** A repo with no policy file, an unparseable file, or an
unknown `version` is treated as `human` for every change. A file that matches
no rule is `human`.

**Tier order.** `auto` < `review` < `human`. A change takes the strictest tier
among its changed files. Deleted and renamed files count under both old and
new paths.

**Built-in rules.** These cannot be overridden:

- A change touching `.sluice/**` is `human`.
- A change touching `.github/**` is `human` and its paths are protected.
- A tainted run is at least `tainted_runs.max_tier` (default `human`).
- While any `freeze` condition holds, only `human` merges proceed.

**Requirements.** Every item under a tier's `require` must hold at the PR's
current head SHA:

| Key | Satisfied when |
|---|---|
| `checks` | Each named check run concluded `success` at the head SHA and was created by a `trusted_checks` app |
| `review_app` | The deploy adapter reports the PR's review environment healthy at the head SHA |
| `independent_review` | A reviewer run with a different identity from the author reported no blocking findings on the head SHA |
| `max_changed_lines` | Added plus removed lines, as reported by the forge, do not exceed the value; exceeding it raises the change one tier rather than failing it |
| `approval` | A passkey approval for the head SHA is recorded |

**Staleness.** Any new commit on the PR discards all evidence and approvals.

**Decision record.** Each evaluation stores the policy file's commit SHA, the
head SHA, the tier, each requirement with its observed value, and the verdict.
The same inputs must always produce the same verdict.

**Freeze and watch** read from the metrics adapter. If the adapter is
unreachable, freeze conditions are treated as holding (fail closed) and the
watch window is extended until it answers.

**Budgets** are enforced by the runner. Exceeding one stops the run and marks
the work item for a human; it never raises a limit on its own.

## Not in v1alpha1

- Per-actor rules (different tiers for different agents).
- Gating human-authored PRs.
- Custom requirement plugins. The requirement set is closed so that a policy
  can be read and audited without reading code.
