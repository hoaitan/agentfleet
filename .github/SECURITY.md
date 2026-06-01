# Security Policy

## Supported Versions

agentfleet is distributed as a Go module. Security fixes are applied to the
latest released minor version on the `main` branch. Older versions are not
maintained — please track the latest tag.

| Version        | Supported          |
| -------------- | ------------------ |
| latest `main`  | :white_check_mark: |
| older releases | :x:                |

## Reporting a Vulnerability

**Please do not open a public issue for security vulnerabilities.**

Report vulnerabilities privately through GitHub's
[private vulnerability reporting](https://github.com/hoaitan/agentfleet/security/advisories/new).
This creates a private advisory that only maintainers can see.

When reporting, please include:

- A description of the vulnerability and its impact
- Steps to reproduce (proof-of-concept if possible)
- Affected version(s) or commit
- Any suggested remediation

### What to expect

- **Acknowledgement:** within 3 business days.
- **Assessment & triage:** within 10 business days we will confirm the issue
  and outline next steps.
- **Fix & disclosure:** we will coordinate a fix and a disclosure timeline with
  you. We aim to release a patch within 90 days, and will credit reporters who
  wish to be acknowledged.

## Scope & Security Considerations

agentfleet spawns and supervises **interactive CLI sessions in PTYs** and can
expose them over **Unix domain sockets** (e.g. `/tmp/agentfleet-<task>.sock`).
Operators should be aware that:

- Anyone with filesystem access to a session socket can attach to and control
  that agent's terminal. Restrict socket permissions and the directory they
  live in.
- Tasks loaded from external sources (HTTP endpoints, generated tasks, files)
  execute arbitrary commands. Only load task definitions from trusted sources.
- API keys (e.g. `ANTHROPIC_API_KEY`) are read from the environment. Never
  commit credentials; use a secrets manager or environment injection.

Reports about these areas — socket permissioning, untrusted task sources, and
secret handling — are especially welcome.
