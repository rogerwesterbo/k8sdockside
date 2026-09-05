# Security Policy

## Reporting a vulnerability

**Please do not open a public issue for a security vulnerability.**

Use GitHub's private reporting instead: **Security → Report a vulnerability** on
[this repository](https://github.com/rogerwesterbo/k8sdockside/security/advisories/new).
That opens a private advisory only the maintainers can see.

Include what you found, how to reproduce it, what an attacker gets out of it,
and a suggested fix if you have one.

You can expect an acknowledgement within a few days, and to be credited in the
advisory unless you would rather not be.

## Supported versions

Fixes go onto the latest release. This project has not reached 1.0, so there is
no back-porting to older tags.

| Version | Supported |
| ------- | --------- |
| Latest release | ✅ |
| Anything older | ❌ |

## What this app does with your credentials

K8s Dockside is a desktop application. It has no server component, no telemetry
and no account.

- **Kubeconfigs are read, never written.** Context aliases and colours are
  stored in the app's own settings file; your kubeconfig files are not modified.
- **Credentials stay in `clientcmd`.** Client certificates, bearer tokens and
  `exec` credential plugins are handled by the standard Kubernetes client
  libraries. The app does not copy secrets out of them or persist them.
- **Nothing connects at launch.** Contexts are listed from disk. A connection is
  opened when you open a view, and closed when the last tab using it closes.
  Port forwards remembered from a previous session are stored as *requests*, not
  as live connections.
- **Secrets are redacted before caching.** An informer holds a whole collection
  in memory, so Secret values are stripped on the way in and the tables show key
  counts only. The YAML editor is the deliberate exception: it reads the object
  live, because an editor opened on a redacted copy would write the redaction
  back over the secret.
- **Themes and plugins are data, not code.** Both formats are JSON — a theme is
  colours, a plugin names resource kinds and PromQL the app already knows how to
  render. Neither can ship CSS or execute anything.

The app is only as privileged as the kubeconfig you point it at. Treat a
context's RBAC as the boundary: **Shell** on a node, for example, creates the
same privileged debug pod `kubectl debug` would, and will only work if you are
already allowed to do that.

## Build and release security

- **Every push is scanned.** CI runs `gosec` and `govulncheck` over the Go code
  and `npm audit` over the frontend tree, and fails on findings.
- **Weekly deep scan.** A scheduled workflow re-runs those in reporting mode,
  adds a Trivy filesystem scan for committed secrets and misconfiguration, and
  files results under **Security → Code scanning**.
- **Dependencies are updated automatically** by Dependabot across the Go, npm
  and GitHub Actions trees.
- **Releases are built only from a tag**, entirely in GitHub Actions, and ship
  an SPDX SBOM and a `checksums.txt` of every asset.
- **Binaries are `-trimpath` and stripped**, and the desktop build uses the
  production build tags.
- **`checksums.txt` is signed with cosign, keylessly.** There is no private key
  to steal: the release job exchanges its short-lived GitHub OIDC token for a
  Sigstore certificate that names the workflow, repository and tag that did the
  signing, and the signature is logged in Rekor.
- **Every asset carries SLSA build provenance**, recorded in GitHub's
  attestation store, tying each file to the workflow run and commit that
  produced it.

## Verifying a download

Checksums:

```sh
sha256sum -c --ignore-missing checksums.txt
```

The signature on that checksum file — one signature covering every asset,
because `checksums.txt` names them all:

```sh
cosign verify-blob \
  --certificate checksums.txt.pem \
  --signature checksums.txt.sig \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github\.com/rogerwesterbo/k8sdockside/\.github/workflows/release\.yml@refs/tags/' \
  checksums.txt
```

Build provenance for an individual file:

```sh
gh attestation verify <file> --repo rogerwesterbo/k8sdockside
```

### What this does not cover

Sigstore signatures and provenance attestations prove **where a file came
from**. They are not operating-system code signing, and neither macOS nor
Windows consults them:

- macOS builds are ad-hoc signed, not notarised — Gatekeeper will warn.
- Windows installers are unsigned — SmartScreen will warn.
- Linux packages are unsigned.

Fixing those needs an Apple Developer ID with notarisation and an Authenticode
certificate, which are paid and identity-bound. The Wails scaffold already has
the `darwin:sign:notarize`, `windows:sign:installer` and `linux:sign:packages`
tasks waiting for credentials.
