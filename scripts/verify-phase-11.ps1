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

function Count-Matches($text, $pattern) {
    return ([regex]::Matches($text, $pattern)).Count
}

function Invoke-Checked($command, $arguments) {
    & $command @arguments | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "$command $($arguments -join ' ') failed with exit code $LASTEXITCODE"
    }
}

Write-Host "== Phase 10 baseline verification =="
& ".\scripts\verify-phase-10.ps1"
if ($LASTEXITCODE -ne 0) {
    throw "scripts\verify-phase-10.ps1 failed with exit code $LASTEXITCODE"
}

Write-Host "== Phase 11 HA decision checks =="
Require-File "docs\architecture\high-availability-decision.md"
Require-File "README.md"
Require-File "deploy\kubernetes\README.md"
Require-File "deploy\kubernetes\deployments.yaml"

$decision = Get-Content -LiteralPath "docs\architecture\high-availability-decision.md" -Raw
$readme = Get-Content -LiteralPath "README.md" -Raw
$deployReadme = Get-Content -LiteralPath "deploy\kubernetes\README.md" -Raw
$deployments = Get-Content -LiteralPath "deploy\kubernetes\deployments.yaml" -Raw

Require-Contains $decision "not claiming high availability" "high-availability-decision.md"
Require-Contains $decision "Kubernetes-ready microservices" "high-availability-decision.md"
Require-Contains $decision "RabbitMQ quorum queues" "high-availability-decision.md"
Require-Contains $decision "managed or replicated Postgres" "high-availability-decision.md"
Require-Contains $decision "Required Failure Tests Before Claiming HA" "high-availability-decision.md"
Require-Contains $decision "Do not use yet" "high-availability-decision.md"

Require-Contains $readme "Phase 11" "README.md"
Require-Contains $readme "documented HA deferral" "README.md"
Require-Contains $readme "Verify High Availability Decision" "README.md"
Require-Contains $readme "Not yet allowed" "README.md"
Require-Contains $readme "highly available Kubernetes deployment" "README.md"

Require-Contains $deployReadme "They do not prove high availability" "deploy/kubernetes/README.md"
Require-Contains $deployReadme "high-availability-decision.md" "deploy/kubernetes/README.md"

if ((Count-Matches $deployments "replicas: 1") -ne 10) {
    throw "Phase 11 deferral expects ten single-replica readiness Deployments."
}
Require-NotContains $deployments "kind: HorizontalPodAutoscaler" "deployments.yaml"
Require-NotContains $deployments "kind: PodDisruptionBudget" "deployments.yaml"

Write-Host "== Native smoke =="
Invoke-Checked "git" @("diff", "--check")

Write-Host "Phase 11 verification completed."
