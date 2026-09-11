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

## Native authentication validation

Build on macOS with Apple Command Line Tools or Xcode and `CGO_ENABLED=1`.
The release script links Foundation and LocalAuthentication into the CLI; no
separate helper or language runtime is shipped.

Test the real authentication dialog without reading Mosyle cookies or requesting
administrator rights (ordinary test runs skip this interactive test):

```sh
MOSYLE_TEST_LOCAL_AUTH=success go test ./internal/localauth -run '^TestInteractiveConfirmation$' -count=1 -v
MOSYLE_TEST_LOCAL_AUTH=cancel go test ./internal/localauth -run '^TestInteractiveConfirmation$' -count=1 -v
```

Authenticate in the first test and click Cancel in the second. Run the success
test again and select password authentication to verify the fallback separately.

Before publishing, manually validate the native dialog on a managed test Mac:

- Successful Touch ID and Mac login-password fallback each permit one request.
- Canceling the dialog or pressing Ctrl-C prevents submission and closes the prompt.
- A Mac without enrolled Touch ID offers password authentication.
- Leaving the prompt open for two minutes stops the request.
- `status`, `wait`, and an already-admin `request` do not prompt.

Successful confirmation in a real `request` can grant temporary administrator
rights. Unit tests use fake authentication and never submit live requests.
