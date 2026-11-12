# Sluice: threat model

Status: draft, 2026-11-12

## What we protect

1. The default branch of every gated repo (in GitOps, this is production).
2. Secrets: CI secrets, cluster credentials, the forge App key, model API keys.
3. The integrity of the audit log.
4. The operator's money (model spend) and attention.

## Who we defend against

| Adversary | Assumed capability |
|---|---|
| **Misbehaving agent** | Follows a wrong plan with full use of everything in its sandbox. Not malicious, just wrong. |
| **Injected agent** | An attacker controls the agent's instructions through content it read (issue text, web page, dependency, file in the repo). Treated as arbitrary code execution inside the sandbox. |
| **Malicious contributor** | Can open issues, comment, or open PRs from a fork. |
| **Network attacker** | Can send requests to any exposed endpoint, including forged webhooks. |

Out of scope: a malicious operator, a compromised forge, a compromised cluster
control plane, and a compromised model provider.

## Trust boundaries

```
 untrusted                    | trusted
------------------------------+------------------------------------------
 agent sandbox (Job)          | courier, gate, Postgres
 issue/PR/comment text        | policy file on the default branch
 PR branch contents           | forge API responses fetched by the gate
 anything an agent reports    | passkey approvals on the gate's page
```

The rule that follows: **the gate believes only what it fetched itself.**
Nothing an agent says about its own work counts as evidence.

## Threats and controls

### T1. Agent merges its own change

- Agents hold no token with merge permission. Only the gate's App identity
  merges.
- Where the forge supports it (public repos, paid plans), a ruleset restricts
  merges on gated branches to the Sluice App. Where it does not (private repos
  on GitHub Free), this is not enforceable server-side; drift detection flags
  any merge that lacks a decision record. The docs state this limit plainly.

### T2. Agent pushes directly to a protected branch

- GitHub installation tokens can be narrowed to a repo and to permissions, but
  not to a branch. A write token in the sandbox would therefore allow a push
  to the default branch.
- Control: the sandbox gets a read-only token. The agent commits locally; the
  courier, outside the sandbox, pushes only to `sluice/run-<id>`.

### T3. Agent edits CI to steal secrets

- A pushed branch that changes a workflow file runs that workflow with the
  repo's secrets before anyone reviews it.
- Control: the courier checks the diff before pushing. Changes to protected
  paths (`.github/**`, the policy file, anything the policy lists) are held
  for a human approval of that exact diff, before the push.

### T4. Agent weakens the policy

- Policy is read only from the default branch at decision time, never from the
  PR. A PR that edits the policy is always `human` tier, and this rule is
  built in, not configurable.

### T5. Approval is replayed or the code changes after approval

- Every verdict and approval is bound to the head SHA. The merge call pins
  that SHA and fails if the branch moved. A new push discards prior approvals
  and evidence.

### T6. Agent forges evidence

- The gate reads check runs, review-app health and diff stats from the forge
  and adapters directly. Check runs count only when created by an App listed
  in the policy, so an agent cannot post a passing check under its own name.

### T7. Author approves itself

- The reviewer run has a separate identity, a fresh context, and sees the diff
  and the issue but not the author's transcript. The gate rejects a review
  from the run that authored the change.
- Residual risk: both runs may share the same blind spots. Policy can require
  a different model; high-risk paths should stay `human`.

### T8. Prompt injection through untrusted content

- Assume it succeeds. T1–T6 limit what a hijacked agent can do to: a diff on
  its own branch, inside its budget.
- Taint: a run that fetched content from outside the repo, or was triggered by
  an issue from a non-collaborator, is marked tainted and capped at the tier
  the policy sets for tainted runs (default `human`).
- Text from issues and PRs is displayed escaped in the board and is never
  interpolated into gate decisions.

### T9. Secret exfiltration from the sandbox

- The sandbox holds two secrets: a read-only token for one repo and a model
  API key. Egress is default-deny with an allowlist (forge, model API, package
  registries). No service account token is mounted.
- Residual risk: the model key can be spent or leaked to an allowed host. Use
  a per-run or low-limit key where the provider supports it; budgets cap the
  damage.

### T10. Forged webhooks and exposed endpoints

- Webhooks are verified by HMAC and treated as hints: the gate re-fetches
  state from the forge API before acting.
- The approval page is the only human-facing write endpoint. It requires a
  passkey (WebAuthn), shows the diff summary and evidence, and signs the SHA.
- Notifications carry a link only, never an approve action or a bearer token.

### T11. Gate compromise

- The gate holds the App key, so it is the highest-value target. Keep it
  small: no UI framework, no agent code, no outbound calls except forge,
  adapters and notifier. The board runs as a separate process with a read-only
  database role and no forge write access.
- App permissions are the minimum: contents write, pull requests write,
  checks write, metadata read. No admin, no secrets, no workflows permission.
- Releases and images are signed; the Helm chart ships a NetworkPolicy.

### T12. Audit log tampering

- Entries are append-only and hash-chained; the database role used by the gate
  cannot update or delete them. The chain head can be exported on a schedule
  to an external location so truncation is detectable.

### T13. Runaway cost or loops

- Budgets per run, project and day for tokens, cost and wall time, enforced by
  the runner outside the sandbox. Retry limits per work item. The kill switch
  stops new runs and merges at once.

### T14. A bad change passes every check

- Gates reduce this, they do not remove it. Controls: freeze on SLO burn,
  a post-merge watch window with an automatic revert PR, and per-tier revert
  rates on the board so the operator can see when a tier is too permissive.

## Known limits

- On forges or plans without server-side merge restrictions, anyone with
  write access can bypass Sluice. Sluice detects this; it cannot prevent it.
- v1 without the runner gives merge-side guarantees only (T1, T4–T6). T2, T3
  and T9 need the runner.
- A human who approves without reading defeats the `human` tier. The approval
  page is designed to make the risky part of the diff the first thing shown.
