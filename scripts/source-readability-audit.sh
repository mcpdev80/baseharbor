#!/usr/bin/env bash
set -euo pipefail

max_file_lines=800
max_function_span=200
failed=0

while IFS= read -r file; do
  lines="$(wc -l < "$file")"
  if [ "$lines" -gt "$max_file_lines" ]; then
    echo "source readability: $file has $lines lines (limit: $max_file_lines)" >&2
    failed=1
  fi

  if ! awk -v file="$file" -v limit="$max_function_span" '
    function report(end_line) {
      if (function_line == 0) {
        return
      }
      span = end_line - function_line
      if (span > limit) {
        printf "source readability: %s:%d function %s spans %d lines (limit: %d)\n", file, function_line, function_name, span, limit > "/dev/stderr"
        failed = 1
      }
    }
    /^func[[:space:]]/ {
      report(NR)
      function_line = NR
      function_name = $0
      sub(/^func[[:space:]]+/, "", function_name)
      sub(/[[:space:]]*\(.*/, "", function_name)
      if (function_name == "") {
        function_name = "<method>"
      }
    }
    END {
      report(NR + 1)
      exit failed
    }
  ' "$file"; then
    failed=1
  fi
done < <(find cmd internal spec -type f -name '*.go' ! -name '*_test.go' -print | sort)

if [ "$failed" -ne 0 ]; then
  exit 1
fi

echo "Source readability"
echo
echo "Go file size       PASS (<= ${max_file_lines} lines)"
echo "Function span      PASS (<= ${max_function_span} lines)"
