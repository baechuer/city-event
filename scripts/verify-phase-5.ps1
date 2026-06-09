param(
    [switch]$SkipRuntimeSmoke
)

$ErrorActionPreference = "Stop"

$repoRoot = (Resolve-Path -LiteralPath ".").Path
$env:GOCACHE = Join-Path $repoRoot ".cache\go-build"
New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null

$postgresUrl = "postgres://cityevents:cityevents@localhost:5432/cityevents?sslmode=disable"
$rabbitUrl = "amqp://cityevents:cityevents@localhost:5672/"
$redisUrl = "redis://localhost:6379/0"
if (-not $env:PHASE5_TEST_DATABASE_URL) {
    $env:PHASE5_TEST_DATABASE_URL = $postgresUrl
}
if (-not $env:PHASE5_TEST_REDIS_URL) {
    $env:PHASE5_TEST_REDIS_URL = $redisUrl
}
if (-not $env:PHASE4_TEST_DATABASE_URL) {
    $env:PHASE4_TEST_DATABASE_URL = $postgresUrl
}
if (-not $env:PHASE4_TEST_RABBITMQ_URL) {
    $env:PHASE4_TEST_RABBITMQ_URL = $rabbitUrl
}

Write-Host "== Default Go tests =="
go test ./...

Write-Host "== Ensure Postgres, RabbitMQ, and Redis are running =="
docker compose up -d postgres rabbitmq redis

foreach ($service in @("postgres", "rabbitmq", "redis")) {
    $deadline = (Get-Date).AddMinutes(2)
    do {
        $state = docker compose ps --format json $service | ConvertFrom-Json
        if ($state.Health -eq "healthy") {
            break
        }
        Start-Sleep -Seconds 3
    } while ((Get-Date) -lt $deadline)

    $state = docker compose ps --format json $service | ConvertFrom-Json
    if ($state.Health -ne "healthy") {
        docker compose ps $service
        throw "$service did not become healthy."
    }
}

Write-Host "== Integration tests =="
go test -count=1 -tags=integration ./...

if ($SkipRuntimeSmoke) {
    Write-Host "Runtime smoke skipped."
    exit 0
}

Write-Host "== Feed service runtime smoke =="
New-Item -ItemType Directory -Force -Path tmp | Out-Null
go build -o tmp\feed-service.exe .\cmd\feed-service

$port = 18085
$job = Start-Job -ScriptBlock {
    param($dir, $port, $postgresUrl, $redisUrl)
    Set-Location -LiteralPath $dir
    $env:FEED_SERVICE_HTTP_ADDR = "127.0.0.1:$port"
    $env:POSTGRES_URL = $postgresUrl
    $env:REDIS_URL = $redisUrl
    & .\tmp\feed-service.exe
} -ArgumentList $repoRoot, $port, $postgresUrl, $redisUrl

try {
    $deadline = (Get-Date).AddSeconds(20)
    $ready = $null
    do {
        try {
            $ready = Invoke-RestMethod -Uri "http://127.0.0.1:$port/readyz" -Method Get -TimeoutSec 2
            break
        }
        catch {
            Start-Sleep -Milliseconds 300
        }
    } while ((Get-Date) -lt $deadline)

    if (-not $ready) {
        Receive-Job $job -ErrorAction SilentlyContinue | Out-Host
        throw "feed-service did not become ready."
    }

    $list = Invoke-RestMethod -Uri "http://127.0.0.1:$port/v1/feed/events?limit=5" -Method Get
    if ($null -eq $list.events) {
        throw "feed list response did not contain events array."
    }
}
finally {
    Stop-Job $job -ErrorAction SilentlyContinue | Out-Null
    Remove-Job $job -Force -ErrorAction SilentlyContinue | Out-Null
}

Write-Host "Phase 5 verification completed."
