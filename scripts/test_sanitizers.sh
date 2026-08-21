#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
binary="${TMPDIR:-/tmp}/jitjson_native_sanitizer"

gcc \
  -O1 \
  -g \
  -std=c11 \
  -Wall \
  -Wextra \
  -D_GNU_SOURCE \
  -fsanitize=address,undefined \
  -fno-omit-frame-pointer \
  -I"${root_dir}/internal/native" \
  "${root_dir}/tests/native_sanitizer.c" \
  "${root_dir}/internal/native/bridge_linux_amd64.c" \
  "${root_dir}/internal/native/writer_linux_amd64.c" \
  "${root_dir}/internal/native/simd_linux_amd64.c" \
  "${root_dir}/internal/native/jit_linux_amd64.c" \
  -o "${binary}"

ASAN_OPTIONS=detect_leaks=1 UBSAN_OPTIONS=print_stacktrace=1 "${binary}"
