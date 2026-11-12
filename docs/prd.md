# Sluice: product requirements

Status: draft, 2026-11-12

## Problem

Coding agents can now take an issue to a pull request without help. What stops
people from letting them run unattended is not capability but trust: an agent
that can merge, deploy or read secrets can do damage, and today's protection is
mostly prompt instructions and hope.

At the same time, the state of a project is scattered across the forge, CI, the
GitOps repo, the cluster and the metrics stack. Answering "what changed in prod,
who or what changed it, and on what evidence?" takes five tabs.

## What Sluice is

A self-hosted control plane for autonomous software agents, with three parts:

1. **Gate**: the only identity allowed to merge agent-written changes. It
   merges when a policy stored in the repo is satisfied by evidence it collected
   itself.
2. **Board**: a read-only view of every project: specs, issues, agent runs,
   PRs, checks, deployments, SLOs, and one timeline per work item.
3. **Runner**: runs agents in sandboxes that hold no forge write credentials.

The gate is the product. The board makes it usable; the runner makes it safe by
construction rather than by convention.

## Goals

- An agent-authored change reaches production only through a gate decision
  that is recorded, reproducible and tied to an exact commit SHA.
- A compromised or prompt-injected agent cannot merge, cannot push outside its
  run branch, cannot change CI or policy, and cannot read secrets.
- One page per project answers: what is in flight, what is blocked on a human,
  what is deployed where, and is it healthy.
- Any production change can be traced back to the issue, the run, the
  transcript and the approvals that produced it.
- Install to a useful read-only board in five minutes with one Helm chart.

## Non-goals

- Not a CI system, a deploy tool or a metrics store. Sluice reads from those.
- Not an agent framework. It runs existing agents behind an adapter.
- Not a general developer portal (no scaffolding, no service scorecards).
- No multi-tenant SaaS in the open-source scope. One install serves one org.
- Human-authored PRs are displayed but not gated in v1.

## Users

- **Solo operator / homelab** (first user): many repos, one human, wants agents
  working overnight and a phone tap for the risky ones.
- **Small team on GitOps**: wants agent autonomy with an audit trail, cannot
  hand agents a write token.

## Core model

```
Project ── Spec (PRD, docs)
   └── WorkItem (issue)
          └── Run (one agent attempt: transcript, cost, taint, diff)
                 └── Change (PR at a head SHA)
                        ├── Evidence (checks, review app, review, diff stats)
                        ├── Decision (policy version, tier, verdict, approvals)
                        └── Rollout (environment, version, health after)
```

One primitive covers merge and promotion: **a gated merge**. In a GitOps setup
a promotion is a PR against the GitOps repo, so gating merges there gates
deployments. Sluice never talks to the cluster with write access.

## Components

| Component | Role | Credentials |
|---|---|---|
| `sluice-gate` | Evaluates policy, collects evidence, serves the approval page, merges, mints read tokens, writes the audit log | Forge App private key |
| `sluice-board` | Web UI and read API over the database and read-only adapters | Read-only DB role, read-only metrics/cluster access |
| `sluice-runner` | Launches sandboxed agent Jobs, then pushes the result to the run branch through the courier | Kubernetes Job creation in its namespace |
| Postgres | State and append-only audit log | n/a |

All three are Go binaries from one module. The board UI is server-rendered
(Go templates plus htmx) so there is no Node toolchain and one small supply
chain to audit.

Adapters sit behind interfaces, with one implementation each to start:
forge (GitHub App), deploy state (Flux), metrics (Prometheus), notify (ntfy),
agent runtime (Claude Code).

## Requirements by milestone

### v0: read-only board

- GitHub App with read permissions and webhooks; repos opt in by installing it.
- Project page: open issues, PRs with check status, latest test results,
  review apps, version per environment (from Flux), firing alerts and SLO
  burn (from Prometheus).
- Work-item timeline assembled from webhook events.
- "Needs human" inbox: PRs awaiting review, failed checks, issues labelled
  for a human.
- Specs and docs rendered from the repo (`docs/`, configurable).

Done when: the board shows every opted-in repo's live state within 30 seconds
of a change, with no write permission anywhere.

### v1: gate

- Policy file per repo, read from the default branch only
  (see [policy-spec.md](policy-spec.md)).
- Gate evaluates each agent-authored PR, publishes its verdict as a check run,
  and merges with the head SHA pinned when the policy is satisfied.
- Three tiers: `auto`, `review` (independent reviewer run), `human`
  (passkey approval on the gate's own page; ntfy carries only a link).
- Freeze: no autonomous merges for a project while its SLO is burning or a
  named alert fires.
- Post-merge watch: if health regresses inside the watch window, open a revert
  PR and notify.
- Hash-chained audit log, exportable.
- Global and per-project kill switch.
- Drift detection: any merge to a gated branch that did not come from the gate
  is flagged.

Done when: an agent with only PR-level access cannot get a change into the
default branch except through a recorded decision, and the Sluice repo itself
is merged this way.

Limit in v1: agents not started by the runner still hold their own forge
credentials, so the push-side guarantees in the threat model do not apply to
them yet. The gate still controls the merge.

### v2: runner

- Queue fed by labelled issues (`agent-ready` by default).
- Each run is a Kubernetes Job: non-root, read-only root filesystem, default
  deny egress with an allowlist, a read-only forge token for one repo, the
  model API key, and nothing else.
- The agent commits locally. The **courier** (trusted, outside the sandbox)
  inspects the diff and pushes it to `sluice/run-<id>`; diffs touching
  protected paths wait for a human before the push.
- Taint: a run that reads untrusted input is marked and capped at a lower
  tier.
- Budgets per run, per project and per day (tokens, cost, wall time).
- Transcript, cost and diff stored and linked from the timeline.

Done when: a prompt-injected run, given full control of its sandbox, cannot
push to any branch itself, cannot trigger CI on a modified workflow, and
cannot exceed its budget.

## Licensing

- Server (`cmd/`, `internal/`): AGPL-3.0-only.
- Policy spec, evidence schema, adapter interfaces and client SDK
  (`spec/`, `pkg/`): Apache-2.0.
- Contributions under DCO sign-off.

## Success measures

- Share of agent PRs merged with no human touch, per tier.
- Zero merges to gated branches without a decision record.
- Reverts within the watch window, per tier (the cost of autonomy).
- Median time an item waits in the "needs human" inbox.

## Open questions

- Reviewer independence: is a second run of the same model with a clean
  context enough, or should policy be able to require a different model?
- Should human PRs be gateable later, so a team can use one merge path?
- Evidence from review apps: which smoke checks are generic enough to ship?
- GitLab/Forgejo and Argo CD adapters: wait for contributors or build one
  early to prove the interfaces?
