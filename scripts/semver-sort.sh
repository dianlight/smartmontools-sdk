#!/usr/bin/env bash
# scripts/semver-sort.sh — sort version tags by true SemVer precedence.
#
# git's --sort=-v:refname / -version:refname and the POSIX `sort -V` both
# rank a version's own prerelease AFTER its final release. Verified:
#   printf 'v8.0.0\nv8.0.0-pre.506\n' | sort -V
# puts v8.0.0-pre.506 last (i.e. treats it as newer). SemVer §11.3 says the
# opposite: a prerelease has LOWER precedence than the same version without
# one. This script implements the real ordering (SemVer §11) so the
# version-computation scripts never have to reason about a suffix-naive
# sort again.
#
# Usage:
#   <tags, one per line> | scripts/semver-sort.sh [--prefix PFX]
#     Print the input lines sorted ascending by SemVer precedence (lowest
#     first). Each line must be PFX followed by an optional "v" and a
#     MAJOR.MINOR.PATCH[-prerelease] core (e.g. "bindings/go/v8.0.2-pre.504"
#     with --prefix "bindings/go/"). Lines that don't match are dropped,
#     with a warning on stderr — callers are expected to pre-filter with
#     `git tag -l "<pattern>"` and let this script handle precedence only.
#   ... | scripts/semver-sort.sh --prefix PFX --newest
#     Print only the single highest-precedence line (no output, no error,
#     if stdin was empty after filtering).
set -euo pipefail

PREFIX=""
MODE="all"
while [ $# -gt 0 ]; do
  case "$1" in
    --prefix) PREFIX="$2"; shift 2 ;;
    --newest) MODE="newest"; shift ;;
    *) echo "semver-sort.sh: unknown argument: $1" >&2; exit 2 ;;
  esac
done

# Print "sortkey<TAB>original-line" for one input line, or return 1 if the
# line doesn't parse as PREFIX + a valid version core.
key_and_line() {
  local line="$1" rest core prerelease major minor patch core_key pre_key

  rest="${line#"${PREFIX}"}"
  if [ -n "${PREFIX}" ] && [ "${rest}" = "${line}" ]; then
    return 1 # line does not start with PREFIX
  fi
  rest="${rest#v}"

  case "${rest}" in
    *-*)
      core="${rest%%-*}"; prerelease="${rest#*-}"
      # A trailing hyphen with nothing after it (e.g. "8.0.2-") is not a
      # valid SemVer prerelease — it must have at least one identifier.
      [ -n "${prerelease}" ] || return 1
      ;;
    *)   core="${rest}"; prerelease="" ;;
  esac

  major="${core%%.*}"; core="${core#*.}"
  case "${core}" in
    *.*) minor="${core%%.*}"; patch="${core#*.}" ;;
    *) return 1 ;;
  esac
  # Reject empty/non-numeric components, and leading zeroes on any
  # component with more than one digit ("0" alone is valid; "00"/"01" are
  # not — SemVer §9 forbids leading zeroes on numeric identifiers).
  case "${major}" in '' | *[!0-9]* | 0[0-9]*) return 1 ;; esac
  case "${minor}" in '' | *[!0-9]* | 0[0-9]*) return 1 ;; esac
  case "${patch}" in '' | *[!0-9]* | 0[0-9]*) return 1 ;; esac

  # Zero-pad each numeric core component to a fixed width so plain string
  # comparison reproduces numeric comparison (SemVer §11.2). The `10#0$x`
  # form forces base-10 interpretation so a leading zero in the tag (which
  # SemVer forbids for core components anyway) can't be misread as octal.
  core_key="$(printf '%010d.%010d.%010d' "$((10#0$major))" "$((10#0$minor))" "$((10#0$patch))")"

  if [ -z "${prerelease}" ]; then
    # No prerelease sorts AFTER any prerelease of the same core version
    # (SemVer §11.3) — '~' (0x7E) sorts above every digit/letter identifier
    # this repo's schemes use.
    pre_key="~"
  else
    local id out=""
    for id in ${prerelease//./ }; do
      case "${id}" in
        '')
          return 1 # empty identifier (e.g. from "..")
          ;;
        *[!0-9A-Za-z-]*)
          return 1 # character outside SemVer's allowed identifier set
          ;;
        *[!0-9]*)
          # Alphanumeric identifier: always higher precedence than a
          # numeric one at the same position (SemVer §11.4.3), so prefix
          # with '1' (sorts after the '0' numeric prefix below).
          out="${out}.1${id}"
          ;;
        0[0-9]*)
          return 1 # numeric identifier with a leading zero (SemVer §9)
          ;;
        *)
          # Numeric identifier: compare numerically, so zero-pad it, and
          # prefix with '0' so it sorts before any alphanumeric identifier.
          out="${out}.0$(printf '%010d' "$((10#$id))")"
          ;;
      esac
    done
    # Leading '.' stripped; join separator '.' (0x2E) sorts below both the
    # '0' and '1' prefixes above, so a shorter identifier list — a strict
    # prefix of a longer one — correctly sorts lower (SemVer §11.4.4).
    pre_key="${out#.}"
  fi

  printf '%s-%s\t%s\n' "${core_key}" "${pre_key}" "${line}"
}

pairs=""
while IFS= read -r line; do
  [ -n "${line}" ] || continue
  if p="$(key_and_line "${line}")"; then
    pairs="${pairs}${p}"$'\n'
  else
    echo "semver-sort.sh: skipping unparsable tag: ${line}" >&2
  fi
done

if [ -z "${pairs}" ]; then
  exit 0
fi

sorted="$(printf '%s' "${pairs}" | LC_ALL=C sort -t $'\t' -k1,1 | cut -f2-)"

if [ "${MODE}" = "newest" ]; then
  printf '%s\n' "${sorted}" | tail -n1
else
  printf '%s\n' "${sorted}"
fi
