param(
    [switch]$RunFullIntegration
)

$ErrorActionPreference = "Stop"

function Require-File($path) {
    if (-not (Test-Path -LiteralPath $path)) {
        throw "required file is missing: $path"
    }
}

function Require-Contains($text, $needle, $label) {
    if (-not $text.Contains($needle)) {
        throw "$label is missing required content: $needle"
    }
}

function Require-NotContains($text, $needle, $label) {
    if ($text.Contains($needle)) {
        throw "$label contains forbidden content: $needle"
    }
}

function Invoke-Checked($command, $arguments) {
    & $command @arguments | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "$command $($arguments -join ' ') failed with exit code $LASTEXITCODE"
    }
}

Write-Host "== Phase 11 baseline verification =="
& ".\scripts\verify-phase-11.ps1"
if ($LASTEXITCODE -ne 0) {
    throw "scripts\verify-phase-11.ps1 failed with exit code $LASTEXITCODE"
}

if ($RunFullIntegration) {
    Write-Host "== Full integration verification =="
    & ".\scripts\verify-phase-9.ps1"
    if ($LASTEXITCODE -ne 0) {
        throw "scripts\verify-phase-9.ps1 failed with exit code $LASTEXITCODE"
    }
}

Write-Host "== Final evidence document checks =="
Require-File "docs\architecture\final-evidence-audit.md"
Require-File "docs\architecture\high-availability-decision.md"
Require-File "docs\resume\resume-claims.md"
Require-File "README.md"

$audit = Get-Content -LiteralPath "docs\architecture\final-evidence-audit.md" -Raw
$resume = Get-Content -LiteralPath "docs\resume\resume-claims.md" -Raw
$readme = Get-Content -LiteralPath "README.md" -Raw

Require-Contains $audit "Evidence Matrix" "final-evidence-audit.md"
Require-Contains $audit "Exactly-once consumption" "final-evidence-audit.md"
Require-Contains $audit "Not supported" "final-evidence-audit.md"
Require-Contains $audit "at-least-once messaging" "final-evidence-audit.md"
Require-Contains $audit "idempotent business effects" "final-evidence-audit.md"
Require-Contains $audit "The project is not yet a production HA system" "final-evidence-audit.md"

Require-Contains $resume "Recommended Resume Bullets" "resume-claims.md"
Require-Contains $resume "exactly-once RabbitMQ consumption" "resume-claims.md"
Require-Contains $resume "Kubernetes-ready" "resume-claims.md"
Require-Contains $resume "Current Limitation Statement" "resume-claims.md"

Require-Contains $readme "Phase 12" "README.md"
Require-Contains $readme "Verify Final Evidence Audit" "README.md"
Require-Contains $readme "Phase 12 Claim Boundary" "README.md"
Require-Contains $readme "Exactly-once RabbitMQ consumption" "README.md"
Require-Contains $readme "guaranteed no message loss" "README.md"

Write-Host "== Safe bullet sanity checks =="
$recommended = $resume.Substring($resume.IndexOf("## Recommended Resume Bullets"))
$recommended = $recommended.Substring(0, $recommended.IndexOf("## Short Version"))
Require-NotContains $recommended "exactly-once" "Recommended Resume Bullets"
Require-NotContains $recommended "guaranteed" "Recommended Resume Bullets"
Require-NotContains $recommended "highly available" "Recommended Resume Bullets"
Require-NotContains $recommended "production deployed" "Recommended Resume Bullets"

Write-Host "== Native smoke =="
Invoke-Checked "git" @("diff", "--check")

Write-Host "Phase 12 verification completed."
