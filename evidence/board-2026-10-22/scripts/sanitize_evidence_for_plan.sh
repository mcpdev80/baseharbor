#!/usr/bin/env bash
# sanitize_evidence_for_plan.sh — Maskiere Markdown-Checkboxen vor dem Kopieren nach baseharbor-plan
#
# Ziel: CardBoard-Regel erzwingen — im Plan-Repo keine "- [ ]"/"- [x]" Checkboxen.
# Ersetzt durch Unicode-Pendants; sonst unveraendert.
#
# usage: bash sanitize_evidence_for_plan.sh [datei]
#        oder: echo "- [ ] text" | bash sanitize_evidence_for_plan.sh
#
# Exit 0 = ok, 1 = syntax usage error

set -euo pipefail

# Unicode-Boxen:
#   [ ]  -> [\u2610]  BALLOT BOX
#   [x]  -> [\u2714]  CHECK MARK
#   [X]  -> [\u2714]  CHECK MARK
#   [done] -> [\u2714]  CHECK MARK
#   [-]  -> [\u25CB]  CIRCLE
#   [>]  -> [\u25CB]  CIRCLE

sanitize_line() {
    local line="$1"

    # Check for - [x], - [X], - [done] patterns
    if echo "$line" | grep -qE '^\s*-\s+\[(x|X|done)\]'; then
        # Replace checked patterns with CHECK MARK
        line=$(echo "$line" | sed -E 's/^\s*-\s+\[(x|X|done)\]/- [✔]/')
    # Check for - [ ] pattern
    elif echo "$line" | grep -qE '^\s*-\s+\[ \]'; then
        # Replace empty checkbox with BALLOT BOX
        line=$(echo "$line" | sed -E 's/^\s*-\s+\[ \]/- [☐]/')
    # Check for - [-], - [>] patterns
    elif echo "$line" | grep -qE '^\s*-\s+\[(\-|>)]'; then
        # Replace with CIRCLE
        line=$(echo "$line" | sed -E 's/^\s*-\s+\[(\-|>)]/- [○]/')
    fi

    echo "$line"
}

main() {
    if [[ $# -gt 1 ]]; then
        echo "Usage: sanitize_evidence_for_plan.sh [datei]" >&2
        echo "  oder: echo '- [ ] text' | sanitize_evidence_for_plan.sh" >&2
        exit 1
    fi

    local input=""
    if [[ $# -eq 1 ]]; then
        if [[ ! -f "$1" ]]; then
            echo "File not found: $1" >&2
            exit 1
        fi
        input=$(cat "$1")
    else
        input=$(cat)
    fi

    # Process line by line
    while IFS= read -r line; do
        sanitize_line "$line"
    done <<< "$input"
}

main "$@"
