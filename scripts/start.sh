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
docker compose up -d postgres ollama

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
printf 'Ожидание Ollama...\n'
wait_for_service ollama ollama list

ensure_model() {
  local model="$1"
  local models
  models="$(docker compose exec -T ollama ollama list)"
  if ! printf '%s\n' "$models" | awk -v model="$model" '$1 == model { found=1 } END { exit !found }'; then
    printf 'Загрузка модели %s (первый запуск может занять время)...\n' "$model"
    docker compose exec -T ollama ollama pull "$model"
  fi
}

ensure_model qwen3.5:4b
ensure_model embeddinggemma

printf 'Индексация базы знаний...\n'
docker compose run --build --rm --no-deps ai-service python -m ingest

printf 'Сборка и запуск приложения...\n'
docker compose up -d --build
docker compose ps
printf '\nГотово. Откройте http://localhost:3000\n'
