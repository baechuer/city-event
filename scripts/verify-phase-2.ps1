param(
    [switch]$SkipRuntimeSmoke
)

$ErrorActionPreference = "Stop"

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

Write-Host "== Auth Postgres integration tests =="
go test -tags=integration ./internal/services/auth

if ($SkipRuntimeSmoke) {
    Write-Host "Runtime smoke skipped."
    exit 0
}

Write-Host "== Auth service runtime smoke =="
New-Item -ItemType Directory -Force -Path tmp | Out-Null
go build -o tmp\auth-service.exe .\cmd\auth-service

$port = 18082
$cwd = (Resolve-Path -LiteralPath ".").Path
$job = Start-Job -ScriptBlock {
    param($dir, $port)
    Set-Location -LiteralPath $dir
    $env:AUTH_SERVICE_HTTP_ADDR = "127.0.0.1:$port"
    $env:POSTGRES_URL = "postgres://cityevents:cityevents@localhost:5432/cityevents?sslmode=disable"
    $env:JWT_SECRET = "phase-2-smoke-secret"
    & .\tmp\auth-service.exe
} -ArgumentList $cwd, $port

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
        throw "auth-service did not become ready."
    }

    $email = "phase2-" + [guid]::NewGuid().ToString("N") + "@example.com"
    $registerBody = @{
        email = $email
        password = "StrongerPass123"
        displayName = "Phase Two"
    } | ConvertTo-Json

    $registered = Invoke-RestMethod -Uri "http://127.0.0.1:$port/v1/auth/register" -Method Post -Body $registerBody -ContentType "application/json"
    if (-not $registered.accessToken) {
        throw "register did not return an access token."
    }

    $loginBody = @{
        email = $email
        password = "StrongerPass123"
    } | ConvertTo-Json

    $loggedIn = Invoke-RestMethod -Uri "http://127.0.0.1:$port/v1/auth/login" -Method Post -Body $loginBody -ContentType "application/json"
    if (-not $loggedIn.accessToken) {
        throw "login did not return an access token."
    }

    $headers = @{ Authorization = "Bearer " + $loggedIn.accessToken }
    $me = Invoke-RestMethod -Uri "http://127.0.0.1:$port/v1/auth/me" -Method Get -Headers $headers
    if ($me.user.email -ne $email) {
        throw "me returned wrong user."
    }

    Add-Type -AssemblyName System.Net.Http
    $client = [System.Net.Http.HttpClient]::new()
    $client.DefaultRequestHeaders.Authorization = [System.Net.Http.Headers.AuthenticationHeaderValue]::new("Bearer", $loggedIn.accessToken)

    $logout = $client.PostAsync("http://127.0.0.1:$port/v1/auth/logout", $null).GetAwaiter().GetResult()
    if ([int]$logout.StatusCode -ne 204) {
        throw "logout returned $([int]$logout.StatusCode)."
    }

    $afterLogout = $client.GetAsync("http://127.0.0.1:$port/v1/auth/me").GetAwaiter().GetResult()
    if ([int]$afterLogout.StatusCode -ne 401) {
        throw "revoked token returned $([int]$afterLogout.StatusCode)."
    }

    $client.Dispose()
}
finally {
    Stop-Job $job -ErrorAction SilentlyContinue | Out-Null
    Remove-Job $job -Force -ErrorAction SilentlyContinue | Out-Null
}

Write-Host "Phase 2 verification completed."
