$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

function Invoke-Docker {
    param(
        [Parameter(Mandatory = $true, ValueFromRemainingArguments = $true)]
        [string[]]$DockerArgs
    )

    & docker @DockerArgs
    if ($LASTEXITCODE -ne 0) {
        throw "Команда 'docker $($DockerArgs -join ' ')' завершилась с кодом $LASTEXITCODE."
    }
}

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw 'Docker CLI не найден. Установите Docker Desktop for Windows.'
}

try {
    Invoke-Docker info *> $null
}
catch {
    $dockerDesktop = Join-Path $env:ProgramFiles 'Docker\Docker\Docker Desktop.exe'
    if (-not (Test-Path $dockerDesktop)) {
        throw 'Docker Engine недоступен, а Docker Desktop не найден. Установите или запустите Docker Desktop.'
    }

    Start-Process -FilePath $dockerDesktop
    Write-Host 'Ожидание Docker Engine...'
    $engineReady = $false
    for ($attempt = 0; $attempt -lt 60; $attempt++) {
        try {
            Invoke-Docker info *> $null
            $engineReady = $true
            break
        }
        catch {
            Start-Sleep -Seconds 2
        }
    }
    if (-not $engineReady) {
        throw 'Docker Engine не запустился за 120 секунд. Проверьте Docker Desktop и доступ к Docker.'
    }
}

if (-not (Test-Path '.env')) {
    Copy-Item '.env.example' '.env'
    Write-Host 'Создан .env из .env.example. При необходимости задайте AI_INTERNAL_TOKEN в .env.'
}

Invoke-Docker compose config -q
Invoke-Docker compose up -d postgres ollama

function Wait-ForService {
    param(
        [string]$Service,
        [string[]]$Command
    )

    for ($attempt = 0; $attempt -lt 60; $attempt++) {
        try {
            Invoke-Docker compose exec -T $Service @Command *> $null
            return
        }
        catch {
            Start-Sleep -Seconds 2
        }
    }

    throw "Сервис $Service не стал готов за 120 секунд. Проверьте: docker compose logs $Service"
}

Write-Host 'Ожидание PostgreSQL...'
Wait-ForService -Service 'postgres' -Command @('pg_isready')
Write-Host 'Ожидание Ollama...'
Wait-ForService -Service 'ollama' -Command @('ollama', 'list')

function Ensure-OllamaModel {
    param([string]$Model)

    $models = Invoke-Docker compose exec -T ollama ollama list
    $pattern = '^{0}(\s|$)' -f [regex]::Escape($Model)
    if (-not ($models -match $pattern)) {
        Write-Host "Загрузка модели $Model (первый запуск может занять время)..."
        Invoke-Docker compose exec -T ollama ollama pull $Model
    }
}

Ensure-OllamaModel -Model 'qwen3.5:4b'
Ensure-OllamaModel -Model 'embeddinggemma'

Write-Host 'Индексация базы знаний...'
Invoke-Docker compose run --build --rm --no-deps ai-service python -m ingest

Write-Host 'Сборка и запуск приложения...'
Invoke-Docker compose up -d --build
Invoke-Docker compose ps
Write-Host "`nГотово. Откройте http://localhost:3000"
