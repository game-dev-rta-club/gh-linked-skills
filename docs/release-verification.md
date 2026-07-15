# Verifying release artifacts

Skill Linker releases from v0.6.0 onward are immutable and include SHA-256
checksums and signed GitHub build provenance. Use these steps when you need to
verify a downloaded binary independently of `gh extension install`. Earlier
releases use the former extension and asset names.

Set the release you want to verify and download it into an empty directory:

```sh
version=v0.6.0
gh release download "$version" --repo game-dev-rta-club/gh-skill-linker
```

On Linux, verify every checksum with:

```sh
sha256sum -c SHA256SUMS
```

On macOS, use:

```sh
shasum -a 256 -c SHA256SUMS
```

Then verify that every asset was produced by this repository's release
workflow from the requested tag on a GitHub-hosted runner:

```sh
for asset in SHA256SUMS gh-skill-linker_"$version"_*; do
  gh attestation verify "$asset" \
    --repo game-dev-rta-club/gh-skill-linker \
    --signer-workflow game-dev-rta-club/gh-skill-linker/.github/workflows/release.yml \
    --source-ref "refs/tags/$version" \
    --deny-self-hosted-runners
done
```

All checksum and attestation commands must succeed before you trust the
downloaded artifacts. Report a failure or unexpected provenance privately as
described in the [security policy](../SECURITY.md).
