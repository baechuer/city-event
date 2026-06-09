param(
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

$repoRoot = (Resolve-Path -LiteralPath ".").Path
$env:GOCACHE = Join-Path $repoRoot ".cache\go-build"
New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null

$postgresUrl = "postgres://cityevents:cityevents@localhost:5432/cityevents?sslmode=disable"
$rabbitUrl = "amqp://cityevents:cityevents@localhost:5672/"
if (-not $env:PHASE4_TEST_DATABASE_URL) {
    $env:PHASE4_TEST_DATABASE_URL = $postgresUrl
}
if (-not $env:PHASE4_TEST_RABBITMQ_URL) {
    $env:PHASE4_TEST_RABBITMQ_URL = $rabbitUrl
}

Write-Host "== Default Go tests =="
go test ./...

Write-Host "== Ensure Postgres and RabbitMQ are running =="
docker compose up -d postgres rabbitmq

foreach ($service in @("postgres", "rabbitmq")) {
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

if ($SkipBuild) {
    Write-Host "Worker build skipped."
    exit 0
}

Write-Host "== Worker builds =="
New-Item -ItemType Directory -Force -Path tmp | Out-Null
go build -o tmp\outbox-relay.exe .\cmd\outbox-relay
go build -o tmp\feed-worker.exe .\cmd\feed-worker

Write-Host "Phase 4 verification completed."
