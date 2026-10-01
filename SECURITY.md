# Security Policy

## Supported Versions

Only the latest minor release of the current major version receives security
fixes. Upgrade to the newest `vX.Y.Z` tag before reporting an issue.

## Reporting a Vulnerability

Please do not open a public issue for security problems. Use GitHub's private
vulnerability reporting instead: **Security → Report a vulnerability** on
<https://github.com/dntosas/astrolavos/security/advisories>. You should get an
acknowledgement within a week. Fixes are released as a patch version and the
advisory is published once the fix is available.

## Release Integrity

Every release after `v1.0.0` produced by the tagged
[`go-release.yml`](.github/workflows/go-release.yml) workflow ships with:

- **Keyless Sigstore signatures** (cosign) on every container image and
  multi-arch manifest in `ghcr.io/dntosas/astrolavos`, and on the
  `checksums.txt` file of the GitHub release. There is no long-lived signing
  key; the certificate is bound to the workflow identity and recorded in the
  public Rekor transparency log.
- **SPDX SBOMs** (`*.sbom.json`) for each release archive, generated with syft.
- **GitHub build provenance attestations** (SLSA) for the release archives,
  SBOMs, checksums and the image manifest.

The signing identity is
`https://github.com/dntosas/astrolavos/.github/workflows/go-release.yml@refs/tags/vX.Y.Z`
issued by `https://token.actions.githubusercontent.com`.

### Verify a container image

```sh
cosign verify ghcr.io/dntosas/astrolavos:vX.Y.Z \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github.com/dntosas/astrolavos/\.github/workflows/go-release\.yml@refs/tags/v'

# or, provenance via the GitHub CLI
gh attestation verify oci://ghcr.io/dntosas/astrolavos:vX.Y.Z --owner dntosas
```

To enforce this in a cluster, use an admission policy such as Kyverno
`verifyImages` with the issuer and identity above.

### Verify a release archive

Download `checksums.txt`, `checksums.txt.sig` and `checksums.txt.pem` from the
release, then:

```sh
cosign verify-blob \
  --certificate checksums.txt.pem --signature checksums.txt.sig \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github.com/dntosas/astrolavos/\.github/workflows/go-release\.yml@refs/tags/v' \
  checksums.txt
sha256sum --ignore-missing -c checksums.txt

# or
gh attestation verify astrolavos_Linux_x86_64.tar.gz --owner dntosas
```

## Runtime Hardening

The published image is `gcr.io/distroless/static:nonroot` with a static binary
and no shell. The Helm chart defaults to `runAsNonRoot`, UID 65532, a read-only
root filesystem, all capabilities dropped, the `RuntimeDefault` seccomp profile
and no mounted service account token.

Note that `skipTLS: true` on an endpoint disables TLS certificate verification
for that probe only. It is opt-in and intended for endpoints with self-signed
certificates; leave it unset otherwise.
