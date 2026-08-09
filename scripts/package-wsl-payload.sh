#!/usr/bin/env bash
set -euo pipefail

repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo"

hash=$(git rev-parse --short HEAD)
case "$hash" in
  *[!0-9a-f]*|'') echo "invalid git hash: $hash" >&2; exit 1 ;;
esac
arch=$(go env GOARCH)
if [[ $arch != amd64 ]]; then
  echo "WSL payload release is qualified only for amd64, got $arch" >&2
  exit 1
fi

for name in catway cathost catctl cats-wsl-host; do
  test -x "bin/$name" || { echo "missing executable bin/$name" >&2; exit 1; }
  file "bin/$name" | grep -q 'ELF 64-bit.*x86-64' || {
    echo "bin/$name is not a Linux amd64 ELF executable" >&2
    exit 1
  }
done

glibc_floor=$(
  for name in catway cathost catctl cats-wsl-host; do
    readelf -W --version-info "bin/$name" 2>/dev/null |
      sed -n 's/.*Name: GLIBC_\([0-9][0-9.]*\).*/\1/p'
  done | sort -Vu | tail -n 1
)
test -n "$glibc_floor" || { echo "could not determine GLIBC symbol floor" >&2; exit 1; }

epoch=${SOURCE_DATE_EPOCH:-$(git show -s --format=%ct HEAD)}
built_at=$(date -u -d "@$epoch" '+%Y-%m-%dT%H:%M:%SZ')
root="cats-wsl-payload_${hash}_linux_${arch}"
stage="dist/${root}"
output="dist/${root}.tar.gz"
rm -rf -- "$stage"
mkdir -p "$stage/licenses"
cp bin/catway bin/cathost bin/catctl bin/cats-wsl-host "$stage/"
cp config.example.yaml NOTICE "$stage/"
cp third_party/libghostty-vt/LICENSE "$stage/licenses/libghostty-vt.txt"

printf '%s\n' \
  '{' \
  '  "schema": 1,' \
  '  "product": "cats-wsl-payload",' \
  "  \"release_id\": \"$hash\"," \
  "  \"compatibility_version\": \"$hash\"," \
  '  "os": "linux",' \
  "  \"architecture\": \"$arch\"," \
  "  \"glibc_floor\": \"$glibc_floor\"," \
  "  \"built_at\": \"$built_at\"" \
  '}' > "$stage/release.json"

(
  cd "$stage"
  sha256sum catway cathost catctl cats-wsl-host config.example.yaml NOTICE \
    licenses/libghostty-vt.txt release.json > SHA256SUMS
)
chmod 0755 "$stage/catway" "$stage/cathost" "$stage/catctl" "$stage/cats-wsl-host"
chmod 0644 "$stage/config.example.yaml" "$stage/NOTICE" "$stage/release.json" \
  "$stage/SHA256SUMS" "$stage/licenses/libghostty-vt.txt"

rm -f -- "$output"
tar --sort=name --owner=0 --group=0 --numeric-owner --mtime="@$epoch" \
  -czf "$output" -C dist "$(basename "$stage")"
rm -rf -- "$stage"
printf '==> %s (glibc >= %s)\n' "$output" "$glibc_floor"
