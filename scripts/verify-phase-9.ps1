$ErrorActionPreference = "Stop"

$repoRoot = (Resolve-Path -LiteralPath ".").Path
$env:GOCACHE = Join-Path $repoRoot ".cache\go-build"
New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null

$postgresUrl = "postgres://cityevents:cityevents@localhost:5432/cityevents?sslmode=disable"
$rabbitUrl = "amqp://cityevents:cityevents@localhost:5672/"
$redisUrl = "redis://localhost:6379/0"
$smtpAddr = "localhost:1025"
$minioEndpoint = "http://localhost:9000"
$minioAccessKey = "cityevents"
$minioSecretKey = "cityevents-password"
$minioBucket = "cityevents-media-test"

foreach ($pair in @(
    @("PHASE4_TEST_DATABASE_URL", $postgresUrl),
    @("PHASE4_TEST_RABBITMQ_URL", $rabbitUrl),
    @("PHASE5_TEST_DATABASE_URL", $postgresUrl),
    @("PHASE5_TEST_REDIS_URL", $redisUrl),
    @("PHASE6_TEST_DATABASE_URL", $postgresUrl),
    @("PHASE6_TEST_RABBITMQ_URL", $rabbitUrl),
    @("PHASE6_TEST_SMTP_ADDR", $smtpAddr),
    @("PHASE7_TEST_DATABASE_URL", $postgresUrl),
    @("PHASE7_TEST_MINIO_ENDPOINT", $minioEndpoint),
    @("PHASE7_TEST_MINIO_ACCESS_KEY", $minioAccessKey),
    @("PHASE7_TEST_MINIO_SECRET_KEY", $minioSecretKey),
    @("PHASE7_TEST_MINIO_BUCKET", $minioBucket)
)) {
    if (-not (Get-Item "Env:\$($pair[0])" -ErrorAction SilentlyContinue)) {
        Set-Item "Env:\$($pair[0])" $pair[1]
    }
}

Write-Host "== Default Go tests =="
go test ./...

Write-Host "== Ensure local dependencies are running =="
docker compose up -d postgres rabbitmq redis minio mailpit

foreach ($service in @("postgres", "rabbitmq", "redis", "minio")) {
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

Write-Host "== Debug walkthrough check =="
if (-not (Test-Path -LiteralPath "project-center\20-audits\debugging-walkthrough.md")) {
    throw "debugging walkthrough is missing."
}
$walkthrough = Get-Content -LiteralPath "project-center\20-audits\debugging-walkthrough.md" -Raw
foreach ($term in @("X-Correlation-ID", "outbox_messages", "feed_events", "notifications")) {
    if (-not $walkthrough.Contains($term)) {
        throw "debugging walkthrough is missing required term: $term"
    }
}

Write-Host "Phase 9 verification completed."
