# Releasing

Releases are triggered by pushing a git tag. The release workflow validates the tag format, runs the quality gate, and uses GoReleaser to build and publish.

## Using the CLI

```bash
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

## Using the GitHub UI

1. Go to the [Releases page](../../releases/new)
2. Click "Choose a tag" and type your new version (e.g., `v1.0.0`)
3. Click "Create new tag: v1.0.0 on publish"
4. Add release notes and click "Publish release"

## Tag Format

The tag format must match `vX.Y.Z` or `vX.Y.Z-suffix` (e.g., `v1.0.0`, `v2.1.0-rc1`).

## When a Release Fails

If the release workflow fails after the tag is pushed, **do not delete and recreate the tag**. Instead:

1. **Fix the issue** in a new commit on `main`
2. **Bump the version** and create a new tag (e.g., if `v1.2.0` failed, release `v1.2.1`)
3. If the failed tag created a partial GitHub release, delete the release (not the tag) from the UI

### Why Not Retag?

Retagging is problematic in Go module land:

- Once a version is fetched by anyone, it's recorded in the Go checksum database and module caches
- Changing what `v1.2.3` points to causes checksum mismatches and `SECURITY ERROR` / `sum mismatch` errors for users
- The Go proxy may have already cached the original tag's contents

**Treat tags as immutable.** A patch release is always safer than retagging.

## Go Modules: Mental Model

Understanding how Go handles versions helps avoid release pitfalls:

- **Tags are version labels.** Go tooling doesn't subscribe to tag creation events.
- **Versions are discovered on demand.** When someone resolves a dependency (directly or via proxy), that's when Go sees the version.
- **Versions are append-only.** Once seen by the checksum database, versions are effectively immutable.
- **No tag? Pseudo-versions.** Go can fetch `v0.0.0-YYYYMMDDHHMMSS-<commit>` for untagged commits, but releases should always be tagged.
- **Major version v2+:** Tags alone aren't enough if your module path doesn't include `/v2`. This is a common "why won't Go pick up my v2.0.0 tag?" issue.

## Security Model

Releases are signed using [Sigstore](https://www.sigstore.dev/) for cryptographic proof of artifact origin. This provides supply chain security without managing private keys.

### How It Works

1. **Keyless signing with Fulcio:** During the GitHub Actions release workflow, cosign requests a short-lived certificate from Fulcio. The certificate is bound to the GitHub Actions OIDC identity (the workflow, repository, and commit).

2. **Transparency log with Rekor:** The signature is recorded in Rekor, Sigstore's immutable transparency log. This creates a public, auditable record that the artifact was signed at a specific time by a specific workflow.

3. **Checksum signing:** Rather than signing each artifact individually, we sign `checksums.txt` which contains SHA256 hashes of all release artifacts. This transitively verifies all artifacts through a single signature.

### What Gets Published

Each release includes:
- `triage_<version>_<os>_<arch>.tar.gz` - The binary archive
- `checksums.txt` - SHA256 hashes of all archives
- `checksums.txt.sigstore.json` - Sigstore bundle (contains signature, certificate, and transparency log entry)

### Verifying a Release

Users can verify that artifacts were built by this repository's GitHub Actions workflow:

```bash
# Download the artifact and verification files
cosign verify-blob \
    --bundle checksums.txt.sigstore.json \
    --certificate-identity-regexp "^https://github.com/spiffcs/triage/.*" \
    --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
    checksums.txt

# Then verify the artifact checksum
sha256sum -c checksums.txt --ignore-missing
```
