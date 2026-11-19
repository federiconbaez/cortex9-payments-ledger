#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VER="v1.7.4"
SHA="936a262061c914694dfd669a543be24573c45d5aa0ff20a8b96b23d01e050e88"
JAR="${TLA_JAR:-$ROOT/tools/tla2tools.jar}"
URL="https://github.com/tlaplus/tlaplus/releases/download/${VER}/tla2tools.jar"

if [[ ! -f "$JAR" ]]; then
  mkdir -p "$(dirname "$JAR")"
  echo "descargando ${URL}"
  if command -v gh >/dev/null 2>&1; then
    tmp="$(mktemp -d)"
    gh release download "$VER" --repo tlaplus/tlaplus --pattern 'tla2tools.jar' --dir "$tmp"
    mv "$tmp/tla2tools.jar" "$JAR"
    rm -rf "$tmp"
  else
    curl -sSfL -o "$JAR" "$URL"
  fi
fi
got="$(sha256sum "$JAR" | awk '{print $1}')"
if [[ "$got" != "$SHA" ]]; then
  echo "checksum invalido de tla2tools.jar: $got (esperado $SHA)" >&2
  exit 1
fi
if ! command -v java >/dev/null 2>&1; then
  echo "hace falta java para TLC" >&2
  exit 1
fi
# -workers 1 y -deadlock para un chequeo de seguridad acotado y reproducible.
run_one() {
  local cfg="$1"
  echo "==== TLC $cfg ===="
  java -XX:+UseParallelGC -jar "$JAR" -config "$cfg" -workers 1 -deadlock "$ROOT/spec/Ledger.tla"
}
run_one "$ROOT/spec/Ledger.cfg"
run_one "$ROOT/spec/Ledger-large.cfg"
