$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

function Invoke-Docker {
    $DockerArgs = $args

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

    Start-Process -FilePath $dockerDesktop -WindowStyle Hidden
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
$config = (Invoke-Docker compose config --format json | Out-String) | ConvertFrom-Json
$ai = $config.services.'ai-service'.environment
$token = $config.services.backend.environment.AI_INTERNAL_TOKEN
if ([string]::IsNullOrEmpty($token)) {
    $bytes = New-Object byte[] 32
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($bytes) } finally { $rng.Dispose() }
    $env:AI_INTERNAL_TOKEN = -join ($bytes | ForEach-Object { $_.ToString('x2') })
    Write-Host 'Для этого запуска создан общий внутренний токен Go/AI; .env не изменён.'
} elseif ($token.Length -lt 32) {
    throw 'AI_INTERNAL_TOKEN должен содержать минимум 32 символа.'
}
$localOllama = $ai.LLM_PROVIDER -eq 'ollama' -and ([uri]$ai.LLM_BASE_URL).Host -eq 'ollama'
if ($localOllama) { Invoke-Docker compose up -d postgres ollama }
else { Invoke-Docker compose up -d postgres }

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
if ($localOllama) {
    Write-Host 'Ожидание Ollama...'
    Wait-ForService -Service 'ollama' -Command @('ollama', 'list')
}

function Ensure-OllamaModel {
    param([string]$Model)

    $models = Invoke-Docker compose exec -T ollama ollama list
    $canonical = if ($Model.Contains(':')) { $Model } else { "${Model}:latest" }
    $pattern = '^{0}(\s|$)' -f [regex]::Escape($canonical)
    if (-not ($models -match $pattern)) {
        Write-Host "Загрузка модели $Model (первый запуск может занять время)..."
        Invoke-Docker compose exec -T ollama ollama pull $Model
    }
}

if ($localOllama) {
    Ensure-OllamaModel -Model $ai.LLM_MODEL
    Ensure-OllamaModel -Model $ai.EMBEDDING_MODEL
}

Write-Host 'Индексация базы знаний...'
Invoke-Docker compose run --build --rm --no-deps ai-service python -m ingest

Write-Host 'Сборка и запуск приложения...'
Invoke-Docker compose up -d --build --no-deps backend ai-service frontend
Invoke-Docker compose ps
Write-Host "`nГотово. Откройте http://localhost:3000"
