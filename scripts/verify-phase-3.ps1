param(
    [switch]$SkipRuntimeSmoke
)

$ErrorActionPreference = "Stop"

$repoRoot = (Resolve-Path -LiteralPath ".").Path
$env:GOCACHE = Join-Path $repoRoot ".cache\go-build"
New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null

$postgresUrl = "postgres://cityevents:cityevents@localhost:5432/cityevents?sslmode=disable"
if (-not $env:EVENT_REGISTRATION_TEST_DATABASE_URL) {
    $env:EVENT_REGISTRATION_TEST_DATABASE_URL = $postgresUrl
}

Write-Host "== Default Go tests =="
go test ./...

Write-Host "== Ensure Postgres is running =="
docker compose up -d postgres

$deadline = (Get-Date).AddMinutes(2)
do {
    $postgres = docker compose ps --format json postgres | ConvertFrom-Json
    if ($postgres.Health -eq "healthy") {
        break
    }
    Start-Sleep -Seconds 3
} while ((Get-Date) -lt $deadline)

$postgres = docker compose ps --format json postgres | ConvertFrom-Json
if ($postgres.Health -ne "healthy") {
    docker compose ps postgres
    throw "Postgres did not become healthy."
}

Write-Host "== Integration tests =="
go test -count=1 -tags=integration ./...

if ($SkipRuntimeSmoke) {
    Write-Host "Runtime smoke skipped."
    exit 0
}

Write-Host "== Event-registration service runtime smoke =="
New-Item -ItemType Directory -Force -Path tmp | Out-Null
go build -o tmp\event-registration-service.exe .\cmd\event-registration-service

$port = 18083
$job = Start-Job -ScriptBlock {
    param($dir, $port, $postgresUrl)
    Set-Location -LiteralPath $dir
    $env:EVENT_REGISTRATION_SERVICE_HTTP_ADDR = "127.0.0.1:$port"
    $env:POSTGRES_URL = $postgresUrl
    & .\tmp\event-registration-service.exe
} -ArgumentList $repoRoot, $port, $postgresUrl

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
        throw "event-registration-service did not become ready."
    }

    $headersOrganizer = @{ "X-User-ID" = "phase3-organizer" }
    $startsAt = (Get-Date).ToUniversalTime().AddDays(1).ToString("o")
    $createBody = @{
        title = "Phase 3 Smoke Event"
        description = "Runtime smoke event"
        city = "Sydney"
        venue = "Town Hall"
        startsAt = $startsAt
        capacity = 1
    } | ConvertTo-Json

    $created = Invoke-RestMethod -Uri "http://127.0.0.1:$port/v1/events" -Method Post -Headers $headersOrganizer -Body $createBody -ContentType "application/json"
    if (-not $created.event.id) {
        throw "create event did not return an event ID."
    }
    $eventId = $created.event.id

    $joinOne = Invoke-RestMethod -Uri "http://127.0.0.1:$port/v1/events/$eventId/join" -Method Post -Headers @{ "X-User-ID" = "phase3-user-1" }
    if ($joinOne.status -ne "CONFIRMED") {
        throw "first join returned $($joinOne.status), expected CONFIRMED."
    }

    $joinTwo = Invoke-RestMethod -Uri "http://127.0.0.1:$port/v1/events/$eventId/join" -Method Post -Headers @{ "X-User-ID" = "phase3-user-2" }
    if ($joinTwo.status -ne "WAITLISTED") {
        throw "second join returned $($joinTwo.status), expected WAITLISTED."
    }

    $cancelOne = Invoke-RestMethod -Uri "http://127.0.0.1:$port/v1/events/$eventId/join" -Method Delete -Headers @{ "X-User-ID" = "phase3-user-1" }
    if (-not $cancelOne.promoted -or $cancelOne.promoted.userId -ne "phase3-user-2") {
        throw "cancel did not promote phase3-user-2."
    }

    $statusTwo = Invoke-RestMethod -Uri "http://127.0.0.1:$port/v1/events/$eventId/join" -Method Get -Headers @{ "X-User-ID" = "phase3-user-2" }
    if ($statusTwo.status -ne "CONFIRMED") {
        throw "promoted user status returned $($statusTwo.status), expected CONFIRMED."
    }

    $detail = Invoke-RestMethod -Uri "http://127.0.0.1:$port/v1/events/$eventId" -Method Get -Headers @{ "X-User-ID" = "phase3-user-2" }
    if ($detail.confirmedCount -ne 1 -or $detail.viewerJoinStatus -ne "CONFIRMED") {
        throw "event detail did not return confirmed count and viewer status."
    }
}
finally {
    Stop-Job $job -ErrorAction SilentlyContinue | Out-Null
    Remove-Job $job -Force -ErrorAction SilentlyContinue | Out-Null
}

Write-Host "Phase 3 verification completed."
