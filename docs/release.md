[← back to docs](./README.md)

# Release process

Releases are built by the [Release](../.github/workflows/release.yml) workflow, which runs [GoReleaser](https://goreleaser.com/) with the [.goreleaser.yml](../.goreleaser.yml) configuration whenever a `v*` tag is pushed.

**Process**

1. Point the GitHub Action at the new version. The action installs the release binary of the version set in the `VERSION` variable of [action.yml](../action.yml):

   ```yaml
   VERSION: "v0.9.0"
   ```

   Update the version in the examples in the [README](../README.md), [gh-action.md](./gh-action.md), and [gh-auth.md](./gh-auth.md) as well, and commit the changes to `main`.

2. Tag the commit and push the tag:

   ```bash
   git tag -a v0.9.0 -m "v0.9.0"
   git push origin v0.9.0
   ```

3. Wait for the [Release workflow run](https://github.com/mlarraz/codeowners-validator/actions/workflows/release.yml) to finish. It:
   - builds the binaries for Linux, macOS, and Windows, and uploads the archives with a `checksums.txt` file,
   - pushes a `linux/amd64` and `linux/arm64` image to `ghcr.io/mlarraz/codeowners-validator`, tagged `v0.9.0`, `v0.9`, `v0`, and `stable`,
   - creates a draft GitHub release with a changelog of the commits since the previous tag.

4. Review the draft on the [releases page](https://github.com/mlarraz/codeowners-validator/releases) and publish it. The action can't install the new version until then, because the assets of a draft release can't be downloaded. To list the release on GitHub Marketplace, select **Publish this Action to the GitHub Marketplace** before publishing.
