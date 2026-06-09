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

Write-Host "== Media service and worker builds =="
New-Item -ItemType Directory -Force -Path tmp | Out-Null
go build -o tmp\media-service.exe .\cmd\media-service
go build -o tmp\media-worker.exe .\cmd\media-worker

if ($SkipRuntimeSmoke) {
    Write-Host "Runtime smoke skipped."
    exit 0
}

Write-Host "== Media service runtime smoke =="
$port = 18087
$job = Start-Job -ScriptBlock {
    param($dir, $port, $postgresUrl, $minioEndpoint, $minioAccessKey, $minioSecretKey, $minioBucket)
    Set-Location -LiteralPath $dir
    $env:MEDIA_SERVICE_HTTP_ADDR = "127.0.0.1:$port"
    $env:POSTGRES_URL = $postgresUrl
    $env:MINIO_ENDPOINT = $minioEndpoint
    $env:MINIO_ACCESS_KEY = $minioAccessKey
    $env:MINIO_SECRET_KEY = $minioSecretKey
    $env:MINIO_BUCKET = $minioBucket
    & .\tmp\media-service.exe
} -ArgumentList $repoRoot, $port, $postgresUrl, $minioEndpoint, $minioAccessKey, $minioSecretKey, $minioBucket

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
        throw "media-service did not become ready."
    }

    $body = @{
        eventId = "phase7-event"
        filename = "banner.jpg"
        contentType = "image/jpeg"
        sizeBytes = 10
    } | ConvertTo-Json
    $created = Invoke-RestMethod -Uri "http://127.0.0.1:$port/v1/media/uploads" -Method Post -Headers @{ "X-User-ID" = "phase7-user" } -Body $body -ContentType "application/json"
    if (-not $created.asset.id -or -not $created.uploadUrl) {
        throw "create upload did not return asset id and upload URL."
    }
    $detail = Invoke-RestMethod -Uri "http://127.0.0.1:$port/v1/media/$($created.asset.id)" -Method Get
    if ($detail.asset.status -ne "UPLOADING") {
        throw "media detail status was $($detail.asset.status), expected UPLOADING."
    }
}
finally {
    Stop-Job $job -ErrorAction SilentlyContinue | Out-Null
    Remove-Job $job -Force -ErrorAction SilentlyContinue | Out-Null
}

Write-Host "Phase 7 verification completed."
