#!/bin/bash
set -euo pipefail

# Build Apple Silicon artifacts and a formula containing the actual archive hash.
version=${1:?Usage: scripts/release.sh vX.Y.Z [owner/repository]}
repository=${2:-ptrap/mosyle-aod-cli}
if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Expected a version such as v0.1.0" >&2
  exit 1
fi
if [[ ! "$repository" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]]; then
  echo "Expected owner/repository" >&2
  exit 1
fi
cd "$(dirname "$0")/.."
release_version=${version#v}
stage="dist/stage-$release_version"
mkdir -p "$stage" dist/Formula
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -buildvcs=false \
  -ldflags "-s -w -X main.version=$release_version" \
  -o "$stage/mosyle-aod" ./cmd/mosyle-aod
cp README.md LICENSE "$stage/"
archive="mosyle-aod_${release_version}_darwin_arm64.tar.gz"
COPYFILE_DISABLE=1 tar -czf "dist/$archive" -C "$stage" mosyle-aod README.md LICENSE
checksum=$(shasum -a 256 "dist/$archive" | cut -d ' ' -f 1)
printf '%s  %s\n' "$checksum" "$archive" > dist/checksums.txt
cat > dist/Formula/mosyle-aod.rb <<FORMULA
class MosyleAod < Formula
  desc "Request temporary admin rights through Mosyle Self-Service"
  homepage "https://github.com/$repository"
  url "https://github.com/$repository/releases/download/$version/$archive"
  version "$release_version"
  sha256 "$checksum"
  license "MIT"

  depends_on :macos
  depends_on arch: :arm64

  def install
    bin.install "mosyle-aod"
  end

  test do
    assert_match "mosyle-aod #{version}", shell_output("#{bin}/mosyle-aod version")
  end
end
FORMULA
printf 'Built dist/%s and dist/Formula/mosyle-aod.rb\n' "$archive"
