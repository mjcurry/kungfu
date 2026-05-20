# Releasing kungfu

This document walks through cutting a new release and the one-time
Homebrew-tap setup that makes `brew install mjcurry/kungfu/kungfu` work.

## Cutting a release

Releases are tag-driven. To ship `v1.2.3`:

```sh
# Make sure main is green, then:
git checkout main && git pull
git tag -a v1.2.3 -m "v1.2.3"
git push origin v1.2.3
```

The push of a `v*.*.*` tag triggers `.github/workflows/release.yml`. It
runs [goreleaser](https://goreleaser.com) with the configuration in
`.goreleaser.yaml` and:

1. Cross-compiles the binary for `linux/amd64`, `linux/arm64`,
   `darwin/amd64`, `darwin/arm64`, `windows/amd64`, `windows/arm64`.
2. Produces a `.tar.gz` per unix target and a `.zip` per windows target.
3. Computes a sha256 checksum file (`kungfu_<version>_checksums.txt`).
4. Builds a release changelog from `feat:` / `fix:` commits since the
   previous tag; `chore:` / `docs:` / `test:` / `style:` are excluded.
5. Creates a GitHub Release at the tag, attaches every archive and the
   checksums file.
6. *Optionally:* updates the Homebrew tap (see below).

## Smoke-testing before tagging

You can run the same goreleaser flow locally without publishing:

```sh
goreleaser release --snapshot --clean --skip=publish
ls dist/
```

The artifacts under `dist/` should include six archives plus a checksums
file. Install one with `tar -xzf dist/kungfu_*_<os>_<arch>.tar.gz` and
run `./kungfu version` to confirm the build is healthy.

## Homebrew tap setup

The release step also writes a Homebrew formula into a tap repo. The
first time you set this up:

1. **Create the tap repository.** Make a new public repo named exactly
   `homebrew-kungfu` under your GitHub account. (Homebrew requires the
   `homebrew-` prefix.) The repo can start empty — goreleaser will push
   the formula on the next release.

2. **Mint a fine-grained Personal Access Token.** Visit
   <https://github.com/settings/personal-access-tokens/new>:
   - *Repository access:* "Only select repositories" → `homebrew-kungfu`.
   - *Permissions:* `Contents: Read and write`.
   - Save the token.

3. **Store the token as a repository secret on `kungfu`.** Settings →
   Secrets and variables → Actions → New repository secret. Name it
   `HOMEBREW_TAP_GITHUB_TOKEN` and paste the token.

That's it. The next tag push will land a formula on the tap, and end
users can install via:

```sh
brew install mjcurry/kungfu/kungfu
```

If `HOMEBREW_TAP_GITHUB_TOKEN` is missing or unset, goreleaser's
`brews.skip_upload` conditional turns the brew step off rather than
failing the release. The binaries on the GitHub Release are produced
regardless.

## Hosting the curl|sh installer

`install.sh` is served straight from the repo:

```sh
curl -fsSL https://raw.githubusercontent.com/mjcurry/kungfu/main/install.sh | sh
```

A short, branded URL (e.g. `kungfu.sh`) is nice-to-have for v2 — set up
once the tool sees enough traction to warrant the domain. The raw GitHub
URL is fine in the meantime and is what the README links to.

## Troubleshooting

- **Release workflow fails at the goreleaser step.** Inspect the action
  logs. The most common cause is a non-fast-forward tag or a missing
  `fetch-depth: 0` checkout (the workflow already sets this, but it's
  worth reverifying if it ever regresses).
- **Homebrew formula is not updated.** Confirm
  `HOMEBREW_TAP_GITHUB_TOKEN` is set as a repo secret and that the PAT
  still has write access to `homebrew-kungfu`. The release logs will
  show `skip_upload=true` when the secret is missing.
- **Install script returns a checksum mismatch.** Almost always means
  the user fetched the script during a release in flight (binaries
  uploaded before checksums or vice versa). Retry in a minute, or set
  `KUNGFU_VERSION=v<previous>` to pin.
