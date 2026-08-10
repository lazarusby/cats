#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 4 ]]; then
  echo "usage: $0 VERSION SOURCE_COMMIT ARTIFACT_DIR OUTPUT" >&2
  exit 2
fi
version=$1
source_commit=$2
artifact_dir=$3
output=$4
[[ $version =~ ^[0-9A-Za-z][0-9A-Za-z._+-]{0,63}$ ]] || { echo "invalid release version" >&2; exit 1; }
[[ $source_commit =~ ^[0-9a-f]{40}$ ]] || { echo "invalid source commit" >&2; exit 1; }

python3 - "$version" "$source_commit" "$artifact_dir" "$output" <<'PY'
import hashlib
import json
import pathlib
import sys

version, source_commit, artifact_dir, output = sys.argv[1:]
root = pathlib.Path(artifact_dir)
files = sorted(p for p in root.iterdir() if p.is_file() and p.name not in {"SHA256SUMS", "release-manifest.json"})
if not files:
    raise SystemExit("release artifact directory is empty")

artifacts = []
windows = None
payload = None
for path in files:
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    item = {"file": path.name, "sha256": digest, "size": path.stat().st_size}
    artifacts.append(item)
    if path.name == f"cats_{version}_windows_amd64_wsl_ubuntu-24.04_amd64.zip":
        windows = item
    if path.name == f"cats-wsl-payload_{version}_ubuntu-24.04_linux_amd64.tar.gz":
        payload = item
if windows is None or payload is None:
    raise SystemExit("release is missing the versioned Windows/WSL pair")

manifest = {
    "schema": 1,
    "product": "cats-release",
    "release_id": version,
    "source_commit": source_commit,
    "artifacts": artifacts,
    "windows_wsl_pair": {
        "launcher_package": windows["file"],
        "payload": payload["file"],
        "windows_architecture": "amd64",
        "wsl_architecture": "amd64",
        "distribution_floor": "ubuntu-24.04",
    },
    "provenance": {"provider": "github-artifact-attestation", "subject": "each release artifact"},
}
pathlib.Path(output).write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
PY

(
  cd "$artifact_dir"
  sha256sum -- * | grep -vE '  (SHA256SUMS|release-manifest\.json)$' > SHA256SUMS
)
echo "==> $output"
