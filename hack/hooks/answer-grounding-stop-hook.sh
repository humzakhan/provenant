#!/usr/bin/env bash
set -uo pipefail

json_escape() {
  local value="${1-}"
  value=${value//\\/\\\\}
  value=${value//\"/\\\"}
  value=${value//$'\n'/\\n}
  value=${value//$'\r'/\\r}
  value=${value//$'\t'/\\t}
  printf '%s' "$value"
}

emit_block_json() {
  local reason
  reason="$(json_escape "${1-}")"
  printf '{"decision":"block","reason":"%s"}\n' "$reason"
}

if ! tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/provenant-stop-hook.XXXXXX")"; then
  emit_block_json "provenant stop hook could not create a temporary directory"
  exit 0
fi
trap 'rm -rf "$tmpdir"' EXIT

input_path="$tmpdir/input.json"
if ! cat >"$input_path"; then
  emit_block_json "provenant stop hook could not read hook input"
  exit 0
fi

if ! command -v perl >/dev/null 2>&1; then
  emit_block_json "provenant stop hook requires perl"
  exit 0
fi

if ! perl -MJSON::PP -e '1' >/dev/null 2>&1; then
  emit_block_json "provenant stop hook requires perl JSON::PP"
  exit 0
fi

parsed_path="$tmpdir/parsed.bin"
if ! perl -MJSON::PP -e '
  use strict;
  use warnings;
  local $/;
  my $raw = <STDIN>;
  my $doc = JSON::PP->new->decode($raw);
  my $stop_hook_active = "";
  if (exists $doc->{stop_hook_active}) {
    my $value = $doc->{stop_hook_active};
    die "stop_hook_active must be a JSON boolean\n" if !JSON::PP::is_bool($value);
    $stop_hook_active = $value ? "1" : "";
  }
  binmode STDOUT;
  print(
    ($doc->{cwd} // ""),
    "\0",
    ($doc->{session_id} // ""),
    "\0",
    ($doc->{last_assistant_message} // ""),
    "\0",
    ($doc->{provenant_bundle_path} // ""),
    "\0",
    ($doc->{bundle_path} // ""),
    "\0",
    $stop_hook_active
  );
' <"$input_path" >"$parsed_path" 2>"$tmpdir/parse.stderr"; then
  parse_error="$(tr '\n' ' ' <"$tmpdir/parse.stderr" | sed 's/[[:space:]]\+/ /g; s/^ //; s/ $//')"
  if [ -z "$parse_error" ]; then
    parse_error="invalid hook input"
  fi
  emit_block_json "provenant stop hook could not parse input: $parse_error"
  exit 0
fi

exec 3<"$parsed_path"
IFS= read -r -d '' cwd <&3 || true
IFS= read -r -d '' session_id <&3 || true
IFS= read -r -d '' answer <&3 || true
IFS= read -r -d '' fixture_provenant_bundle_path <&3 || true
IFS= read -r -d '' fixture_bundle_path <&3 || true
IFS= read -r -d '' stop_hook_active <&3 || true
exec 3<&-

# Claude can invoke the Stop hook again while a prior block is still active.
# Re-entering enforcement just loops; the first invocation already made the gate decision.
if [ "$stop_hook_active" = "1" ]; then
  exit 0
fi

bundle_path=""
if [ -n "${PROVENANT_BUNDLE_PATH:-}" ] && [ -f "${PROVENANT_BUNDLE_PATH:-}" ]; then
  bundle_path="${PROVENANT_BUNDLE_PATH}"
elif [ -n "$cwd" ] && [ -n "$session_id" ] && [ -f "$cwd/.provenant/claude-code/$session_id/current.bundle.json" ]; then
  bundle_path="$cwd/.provenant/claude-code/$session_id/current.bundle.json"
elif [ -n "$fixture_provenant_bundle_path" ] && [ -f "$fixture_provenant_bundle_path" ]; then
  bundle_path="$fixture_provenant_bundle_path"
elif [ -n "$fixture_bundle_path" ] && [ -f "$fixture_bundle_path" ]; then
  bundle_path="$fixture_bundle_path"
fi

if [ -z "$bundle_path" ]; then
  if printf '%s' "$answer" | LC_ALL=C grep -q '[0-9]'; then
    emit_block_json "answer contains digits but no Provenant bundle was found"
  fi
  exit 0
fi

answer_path="$tmpdir/answer.txt"
printf '%s' "$answer" >"$answer_path"

request_path="$tmpdir/request.json"
if ! BUNDLE_PATH="$bundle_path" ANSWER_PATH="$answer_path" perl -MJSON::PP -e '
  use strict;
  use warnings;
  print JSON::PP->new->ascii->canonical->encode({
    bundle_path => $ENV{BUNDLE_PATH},
    answer_path => $ENV{ANSWER_PATH},
  });
' >"$request_path" 2>"$tmpdir/request.stderr"; then
  request_error="$(tr '\n' ' ' <"$tmpdir/request.stderr" | sed 's/[[:space:]]\+/ /g; s/^ //; s/ $//')"
  if [ -z "$request_error" ]; then
    request_error="request encoding failed"
  fi
  emit_block_json "provenant stop hook could not encode request: $request_error"
  exit 0
fi

verdict_path="$tmpdir/verdict.json"
stderr_path="$tmpdir/check-answer.stderr"
provenant_bin="${PROVENANT_BIN:-provenant}"
if "$provenant_bin" check-answer --request "$request_path" >"$verdict_path" 2>"$stderr_path"; then
  exit 0
else
  status=$?
fi
if [ "$status" -eq 1 ]; then
  if reason="$(
    perl -MJSON::PP -e '
      use strict;
      use warnings;
      local $/;
      my $raw = <STDIN>;
      my $doc = JSON::PP->new->decode($raw);
      my $claims = $doc->{ungrounded_claims};
      if (ref($claims) ne "ARRAY" || !@$claims) {
        exit 1;
      }
      my @parts;
      for my $claim (@$claims) {
        my $value = defined $claim->{value} ? $claim->{value} : "";
        my $raw_value = defined $claim->{raw} ? $claim->{raw} : "";
        my $span = "";
        if (defined $claim->{start} && defined $claim->{end}) {
          $span = $claim->{start} . ":" . $claim->{end};
        }
        push @parts, "ungrounded value=$value raw=$raw_value span=$span";
      }
      print join("; ", @parts);
    ' <"$verdict_path"
  )"; then
    emit_block_json "$reason"
    exit 0
  fi
fi

error_reason="provenant check-answer failed"
if [ -s "$stderr_path" ]; then
  stderr_text="$(tr '\n' ' ' <"$stderr_path" | sed 's/[[:space:]]\+/ /g; s/^ //; s/ $//')"
  if [ -n "$stderr_text" ]; then
    error_reason="$error_reason: $stderr_text"
  fi
fi
emit_block_json "$error_reason"
