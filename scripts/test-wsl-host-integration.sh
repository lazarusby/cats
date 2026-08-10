#!/usr/bin/env bash
set -euo pipefail

repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
payload=${1:-"$repo/bin"}
for name in catway cathost catctl cats-wsl-host; do
  test -x "$payload/$name" || {
    echo "missing executable $payload/$name" >&2
    exit 1
  }
done

work=$(mktemp -d)
control="$work/control.fifo"
protocol="$work/protocol.ndjson"
host_log="$work/host.log"
host_pid=
cleanup() {
  exec 3>&- 2>/dev/null || true
  if [[ -n ${host_pid:-} ]] && kill -0 "$host_pid" 2>/dev/null; then
    kill "$host_pid" 2>/dev/null || true
    wait "$host_pid" 2>/dev/null || true
  fi
  rm -rf -- "$work"
}
trap cleanup EXIT

port=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')
mkfifo "$control"
env XDG_CONFIG_HOME="$work/config" XDG_STATE_HOME="$work/state" \
  "$payload/cats-wsl-host" --allow-non-wsl --port "$port" \
  --launch-id phase7-ci --start-dir "$work" <"$control" >"$protocol" 2>"$host_log" &
host_pid=$!
exec 3>"$control"

deadline=$((SECONDS + 30))
until grep -q '"type":"ready"' "$protocol" 2>/dev/null; do
  if ! kill -0 "$host_pid" 2>/dev/null; then
    cat "$protocol" >&2 || true
    cat "$host_log" >&2 || true
    wait "$host_pid" || true
    echo "cats-wsl-host exited before readiness" >&2
    exit 1
  fi
  if (( SECONDS >= deadline )); then
    cat "$protocol" >&2 || true
    cat "$host_log" >&2 || true
    echo "timed out waiting for cats-wsl-host readiness" >&2
    exit 1
  fi
  sleep 0.1
done

"$payload/catctl" probe --url "ws://127.0.0.1:$port/ws" --timeout 12s \
  --script 'wait:500;type:printf CATS_PHASE7_REAL\n;expect:1:CATS_PHASE7_REAL;split:1:v;panes:2;typeat:2:printf CATS_PHASE7_SECOND\n;expect:2:CATS_PHASE7_SECOND;close:2;panes:1'

printf '{"v":1,"type":"stop"}\n' >&3
exec 3>&-
if ! wait "$host_pid"; then
  cat "$protocol" >&2 || true
  cat "$host_log" >&2 || true
  echo "cats-wsl-host did not stop cleanly" >&2
  exit 1
fi
host_pid=
grep -q '"type":"starting"' "$protocol"
grep -q '"type":"ready"' "$protocol"
grep -q '"type":"stopped","reason":"requested"' "$protocol"
echo 'PASS: real cats-wsl-host, cathost, catway, and catctl probe integration'
