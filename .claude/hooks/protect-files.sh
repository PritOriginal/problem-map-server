#!/usr/bin/env bash
# PreToolUse: запрет правок сгенерированных и секретных файлов
set -u
file=$(python3 -c 'import json,sys; print(json.load(sys.stdin).get("tool_input",{}).get("file_path",""))' 2>/dev/null)
[[ -z "$file" ]] && exit 0
case "$file" in
  */docs/swagger.json|*/docs/swagger.yaml|*/docs/docs.go|*mocks*.go|*trm_mocks.go)
    echo "Файл генерируется автоматически ($file). Используй /regen (make swag / mockery)." >&2; exit 2;;
  */configs/config.yaml|*/configs/config-docker.yaml|*/configs/config-tests.yaml|*/.env*)
    echo "Локальный конфиг/секреты ($file) редактировать запрещено; правь configs/config.yaml.example." >&2; exit 2;;
esac
exit 0
