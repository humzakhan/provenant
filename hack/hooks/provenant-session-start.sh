#!/usr/bin/env bash
set -euo pipefail

tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/provenant-session-start.XXXXXX")"
trap 'rm -rf "$tmpdir"' EXIT

input_path="$tmpdir/input.json"
cat >"$input_path"

parsed_path="$tmpdir/parsed.bin"
perl -MJSON::PP -e '
  use strict;
  use warnings;
  local $/;
  my $raw = <STDIN>;
  my $doc = JSON::PP->new->decode($raw);
  binmode STDOUT;
  print(($doc->{cwd} // ""), "\0", ($doc->{session_id} // ""));
' <"$input_path" >"$parsed_path"

exec 3<"$parsed_path"
IFS= read -r -d '' cwd <&3 || true
IFS= read -r -d '' session_id <&3 || true
exec 3<&-

if [ -z "$cwd" ] || [ -z "$session_id" ]; then
  echo "provenant-session-start: cwd and session_id are required" >&2
  exit 1
fi

bundle_path="$cwd/.provenant/claude-code/$session_id/current.bundle.json"
mkdir -p "$(dirname "$bundle_path")"

if [ -n "${CLAUDE_ENV_FILE:-}" ]; then
  printf 'PROVENANT_BUNDLE_PATH=%q\n' "$bundle_path" >>"$CLAUDE_ENV_FILE"
  printf 'export PROVENANT_BUNDLE_PATH\n' >>"$CLAUDE_ENV_FILE"
fi
