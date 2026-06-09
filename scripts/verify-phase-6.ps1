param(
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

$repoRoot = (Resolve-Path -LiteralPath ".").Path
$env:GOCACHE = Join-Path $repoRoot ".cache\go-build"
New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null

$postgresUrl = "postgres://cityevents:cityevents@localhost:5432/cityevents?sslmode=disable"
$rabbitUrl = "amqp://cityevents:cityevents@localhost:5672/"
$redisUrl = "redis://localhost:6379/0"
$smtpAddr = "localhost:1025"

foreach ($pair in @(
    @("PHASE4_TEST_DATABASE_URL", $postgresUrl),
    @("PHASE4_TEST_RABBITMQ_URL", $rabbitUrl),
    @("PHASE5_TEST_DATABASE_URL", $postgresUrl),
    @("PHASE5_TEST_REDIS_URL", $redisUrl),
    @("PHASE6_TEST_DATABASE_URL", $postgresUrl),
    @("PHASE6_TEST_RABBITMQ_URL", $rabbitUrl),
    @("PHASE6_TEST_SMTP_ADDR", $smtpAddr)
)) {
    if (-not (Get-Item "Env:\$($pair[0])" -ErrorAction SilentlyContinue)) {
        Set-Item "Env:\$($pair[0])" $pair[1]
    }
}

Write-Host "== Default Go tests =="
go test ./...

Write-Host "== Ensure Postgres, RabbitMQ, Redis, and Mailpit are running =="
docker compose up -d postgres rabbitmq redis mailpit

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

$deadline = (Get-Date).AddMinutes(1)
do {
    try {
        $client = [System.Net.Sockets.TcpClient]::new()
        $client.Connect("127.0.0.1", 1025)
        $client.Close()
        $mailpitReady = $true
        break
    }
    catch {
        Start-Sleep -Seconds 2
    }
} while ((Get-Date) -lt $deadline)

if (-not $mailpitReady) {
    docker compose ps mailpit
    throw "Mailpit SMTP did not become reachable."
}

Write-Host "== Integration tests =="
go test -count=1 -tags=integration ./...

if ($SkipBuild) {
    Write-Host "Worker build skipped."
    exit 0
}

Write-Host "== Notification worker build =="
New-Item -ItemType Directory -Force -Path tmp | Out-Null
go build -o tmp\notification-worker.exe .\cmd\notification-worker
go build -o tmp\notification-service.exe .\cmd\notification-service

Write-Host "Phase 6 verification completed."
