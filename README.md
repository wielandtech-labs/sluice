# Sluice

A self-hosted control plane for autonomous software agents. Agents write the
code; Sluice decides what is allowed through.

- **Gate**: the only identity that merges agent-written changes, on evidence
  it collected itself, against a policy stored in your repo.
- **Board**: one page per project, from spec to production health.
- **Runner**: sandboxes agents so they hold no forge write credentials.

Status: design stage. Start with the docs:

- [Product requirements](docs/prd.md)
- [Threat model](docs/threat-model.md)
- [Policy file spec](docs/policy-spec.md)

## License

Server code (`cmd/`, `internal/`) is AGPL-3.0-only; see [LICENSE](LICENSE).
The policy spec, evidence schema, adapter interfaces and client SDK (`spec/`,
`pkg/`, `docs/policy-spec.md`) are Apache-2.0; see
[LICENSES/Apache-2.0.txt](LICENSES/Apache-2.0.txt).

Contributions are accepted under the
[Developer Certificate of Origin](https://developercertificate.org/): sign off
your commits with `git commit -s`.
