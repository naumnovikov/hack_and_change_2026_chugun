#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$repo_root"

fail() {
  printf 'Ошибка: %s\n' "$1" >&2
  exit 1
}

command -v docker >/dev/null 2>&1 || fail "Docker CLI не найден. Установите Docker Desktop (macOS) или Docker Engine и Compose plugin (Ubuntu)."

if ! docker info >/dev/null 2>&1; then
  case "$(uname -s)" in
    Darwin)
      open -a Docker || fail "Не удалось запустить Docker Desktop. Установите его и запустите вручную."
      ;;
    Linux)
      if command -v systemctl >/dev/null 2>&1 && systemctl list-unit-files docker.service >/dev/null 2>&1; then
        if systemctl is-active --quiet docker; then
          fail "Docker запущен, но недоступен текущему пользователю. Настройте доступ к Docker; добавление в группу docker даёт права root."
        fi
        if [[ "$EUID" -eq 0 ]]; then
          systemctl start docker || fail "Не удалось запустить службу Docker."
        elif command -v sudo >/dev/null 2>&1; then
          printf 'Для запуска службы Docker может потребоваться пароль sudo.\n'
          sudo systemctl start docker || fail "Не удалось запустить службу Docker."
        else
          fail "Docker Engine не запущен. Запустите его командой sudo systemctl start docker."
        fi
      else
        fail "Docker Engine недоступен. Запустите Docker и убедитесь, что у пользователя есть к нему доступ."
      fi
      ;;
    *)
      fail "Docker Engine недоступен. Запустите Docker Desktop и дождитесь готовности Engine."
      ;;
  esac

  printf 'Ожидание Docker Engine...\n'
  engine_ready=false
  for _ in {1..60}; do
    if docker info >/dev/null 2>&1; then
      engine_ready=true
      break
    fi
    sleep 2
  done
  [[ "$engine_ready" == true ]] || fail "Docker Engine не запустился за 120 секунд. Проверьте Docker Desktop или права доступа."
fi

if [[ ! -f .env ]]; then
  cp .env.example .env
  printf 'Создан .env из .env.example. При необходимости задайте AI_INTERNAL_TOKEN в .env.\n'
fi

docker compose config -q
# Use the AI image's Python to parse Compose JSON; no host Python dependency.
docker compose build ai-service
summary="$(docker compose config --format json | docker compose run --rm -T --no-deps ai-service python -c '
import json,sys
from urllib.parse import urlparse
c=json.load(sys.stdin); e=c["services"]["ai-service"]["environment"]
t=c["services"]["backend"]["environment"].get("AI_INTERNAL_TOKEN", "")
print(len(t)); print(int(e["LLM_PROVIDER"]=="ollama" and urlparse(e["LLM_BASE_URL"]).hostname=="ollama"))
print(e["LLM_MODEL"]); print(e["EMBEDDING_MODEL"])
')"
token_length="$(printf '%s\n' "$summary" | sed -n '1p')"
local_ollama="$(printf '%s\n' "$summary" | sed -n '2p')"
llm_model="$(printf '%s\n' "$summary" | sed -n '3p')"
embedding_model="$(printf '%s\n' "$summary" | sed -n '4p')"
if [[ "$token_length" == 0 ]]; then
  AI_INTERNAL_TOKEN="$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')"
  export AI_INTERNAL_TOKEN
  printf 'Для этого запуска создан общий внутренний токен Go/AI; .env не изменён.\n'
elif [[ "$token_length" -lt 32 ]]; then
  fail 'AI_INTERNAL_TOKEN должен содержать минимум 32 символа.'
fi
if [[ "$local_ollama" == 1 ]]; then docker compose up -d postgres ollama
else docker compose up -d postgres; fi

wait_for_service() {
  local service="$1"
  shift
  for _ in {1..60}; do
    if docker compose exec -T "$service" "$@" >/dev/null 2>&1; then
      return
    fi
    sleep 2
  done
  fail "Сервис $service не стал готов за 120 секунд. Проверьте: docker compose logs $service"
}

printf 'Ожидание PostgreSQL...\n'
wait_for_service postgres pg_isready
if [[ "$local_ollama" == 1 ]]; then
  printf 'Ожидание Ollama...\n'
  wait_for_service ollama ollama list
fi

ensure_model() {
  local model="$1"
  local models
  models="$(docker compose exec -T ollama ollama list)"
  local canonical="$model"
  [[ "$canonical" == *:* ]] || canonical="${canonical}:latest"
  if ! printf '%s\n' "$models" | awk -v model="$canonical" '$1 == model { found=1 } END { exit !found }'; then
    printf 'Загрузка модели %s (первый запуск может занять время)...\n' "$model"
    docker compose exec -T ollama ollama pull "$model"
  fi
}

if [[ "$local_ollama" == 1 ]]; then
  ensure_model "$llm_model"
  ensure_model "$embedding_model"
fi

printf 'Индексация базы знаний...\n'
docker compose run --build --rm --no-deps ai-service python -m ingest

printf 'Сборка и запуск приложения...\n'
docker compose up -d --build --no-deps backend ai-service frontend
docker compose ps
printf '\nГотово. Откройте http://localhost:3000\n'
