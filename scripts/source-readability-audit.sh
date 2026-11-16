#!/usr/bin/env bash
set -euo pipefail

max_file_lines=800
max_function_span=200
failed=0

baseline_file_limit() {
  case "$1" in
    "internal/application/runtime_contract.go") echo 825 ;;
    "internal/application/runtime_postgres.go") echo 953 ;;
    "internal/providers/runtime/podman/lifecycle.go") echo 878 ;;
    *) echo 0 ;;
  esac
}

baseline_function_limit() {
  case "$1|$2" in
    "cmd/baha/mcp_tools.go|registerMCPLifecycleTools") echo 266 ;;
    "internal/application/runtime_contract.go|EnsureRuntimeContract") echo 223 ;;
    *) echo 0 ;;
  esac
}

while IFS= read -r file; do
  lines="$(wc -l < "$file")"
  if [ "$lines" -gt "$max_file_lines" ]; then
    baseline="$(baseline_file_limit "$file")"
    if [ "$baseline" -gt 0 ] && [ "$lines" -le "$baseline" ]; then
      echo "source readability baseline: $file has $lines lines (target: <= $max_file_lines, ratchet: <= $baseline)"
    else
      echo "source readability: $file has $lines lines (limit: $max_file_lines; baseline ratchet: $baseline)" >&2
      failed=1
    fi
  fi

  if ! awk -v file="$file" -v limit="$max_function_span" '
    function emit(end_line) {
      if (function_line == 0) {
        return
      }
      span = end_line - function_line
      if (span > limit) {
        printf "%s\t%s\t%d\n", file, function_name, span
      }
    }
    /^func[[:space:]]/ {
      emit(NR)
      function_line = NR
      function_name = $0
      sub(/^func[[:space:]]+/, "", function_name)
      sub(/[[:space:]]*\(.*/, "", function_name)
      if (function_name == "") {
        function_name = "<method>"
      }
    }
    END {
      emit(NR + 1)
    }
  ' "$file" | while IFS=$'\t' read -r violation_file function_name span; do
    baseline="$(baseline_function_limit "$violation_file" "$function_name")"
    if [ "$baseline" -gt 0 ] && [ "$span" -le "$baseline" ]; then
      echo "source readability baseline: $violation_file function $function_name spans $span lines (target: <= $max_function_span, ratchet: <= $baseline)"
    else
      echo "source readability: $violation_file function $function_name spans $span lines (limit: $max_function_span; baseline ratchet: $baseline)" >&2
      exit 42
    fi
  done; then
    failed=1
  fi
done < <(find cmd internal spec conformance -type f -name '*.go' ! -name '*_test.go' -print | sort)

if [ "$failed" -ne 0 ]; then
  exit 1
fi

echo "Source readability"
echo
echo "Go file size       PASS (<= $max_file_lines lines or no worse than explicit baseline)"
echo "Function span      PASS (<= $max_function_span lines or no worse than explicit baseline)"
echo "Regression ratchet PASS (new/worsened violations fail closed)"
