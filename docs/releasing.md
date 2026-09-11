# Release checklist

1. Push the reviewed changes to `main` and confirm CI passes.
2. Create and push a new version tag, such as `v0.2.0`. The release workflow tests the code, builds the Apple Silicon binary, and creates a draft release with the archive, checksums, and Homebrew formula.
3. Review the assets and release notes, then publish the draft. Mark experimental releases as prereleases.
4. Copy `mosyle-aod.rb` from that published release into `Formula/mosyle-aod.rb` in [ptrap/homebrew-tap](https://github.com/ptrap/homebrew-tap), then commit and push. Use the release's generated formula so its checksum matches the published archive; do not substitute a local build.
5. Verify the Homebrew upgrade on an Apple Silicon Mac:

   ```sh
   brew update
   brew upgrade ptrap/tap/mosyle-aod
   brew test ptrap/tap/mosyle-aod
   mosyle-aod version
   ```

For a fresh installation, use `brew install ptrap/tap/mosyle-aod` instead of `brew upgrade`. The formula test checks the version without requesting admin rights.
