#!/usr/bin/env bash
# PostToolUse: gofmt + go vet для пакета отредактированного .go-файла
set -u
file=$(python3 -c 'import json,sys; print(json.load(sys.stdin).get("tool_input",{}).get("file_path",""))' 2>/dev/null)
[[ -z "$file" || "$file" != *.go ]] && exit 0
[[ "$file" == *mocks* || "$file" == */docs/docs.go ]] && exit 0
cd "$CLAUDE_PROJECT_DIR" || exit 0
gofmt -l -w "$file" >/dev/null 2>&1
pkg=$(dirname "$file")
out=$(go vet "./${pkg#"$CLAUDE_PROJECT_DIR"/}" 2>&1) || { echo "go vet: $out" >&2; exit 2; }
exit 0
