param(
    [switch]$StartInfrastructure,
    [switch]$RequireDockerDaemon
)

$ErrorActionPreference = "Stop"

$services = @(
    "api-gateway",
    "auth-service",
    "event-registration-service",
    "feed-service",
    "notification-service",
    "media-service",
    "media-worker"
)

Write-Host "== Go tests =="
go test ./...

Write-Host "== Docker Compose config =="
docker compose config --quiet

Write-Host "== Service startup checks =="
$previousStartupCheck = $env:CITYEVENTS_STARTUP_CHECK_ONLY
$env:CITYEVENTS_STARTUP_CHECK_ONLY = "true"
try {
    foreach ($service in $services) {
        Write-Host "checking $service"
        go run "./cmd/$service"
    }
}
finally {
    if ($null -eq $previousStartupCheck) {
        Remove-Item Env:\CITYEVENTS_STARTUP_CHECK_ONLY -ErrorAction SilentlyContinue
    }
    else {
        $env:CITYEVENTS_STARTUP_CHECK_ONLY = $previousStartupCheck
    }
}

Write-Host "== Docker daemon check =="
$dockerOutput = docker info --format "{{.ServerVersion}}" 2>&1
if ($LASTEXITCODE -eq 0) {
    $dockerOutput | Out-Host
    $dockerAvailable = $true
}
else {
    $dockerAvailable = $false
    if ($RequireDockerDaemon) {
        throw ($dockerOutput | Out-String)
    }
    Write-Warning "Docker daemon is unavailable; skipping live infrastructure start."
}

if ($StartInfrastructure) {
    if (-not $dockerAvailable) {
        throw "Cannot start infrastructure because Docker daemon is unavailable."
    }

    Write-Host "== Starting local infrastructure =="
    docker compose up -d
    docker compose ps
}

Write-Host "Phase 1 verification completed."
