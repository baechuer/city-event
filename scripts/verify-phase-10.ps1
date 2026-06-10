$ErrorActionPreference = "Stop"

$repoRoot = (Resolve-Path -LiteralPath ".").Path
$env:GOCACHE = Join-Path $repoRoot ".cache\go-build"
New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null

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

function Count-Matches($text, $pattern) {
    return ([regex]::Matches($text, $pattern)).Count
}

function Invoke-Checked($command, $arguments) {
    & $command @arguments | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "$command $($arguments -join ' ') failed with exit code $LASTEXITCODE"
    }
}

Write-Host "== Default Go tests =="
Invoke-Checked "go" @("test", "./...")

Write-Host "== Docker Compose config =="
Invoke-Checked "docker" @("compose", "config", "--quiet")

Write-Host "== Kubernetes manifest static checks =="
foreach ($file in @(
    "Dockerfile",
    ".dockerignore",
    "deploy\kubernetes\README.md",
    "deploy\kubernetes\namespace.yaml",
    "deploy\kubernetes\configmap.yaml",
    "deploy\kubernetes\secret.example.yaml",
    "deploy\kubernetes\deployments.yaml",
    "deploy\kubernetes\services.yaml",
    "deploy\kubernetes\kustomization.yaml"
)) {
    Require-File $file
}

$dockerfile = Get-Content -LiteralPath "Dockerfile" -Raw
Require-Contains $dockerfile "ARG SERVICE=api-gateway" "Dockerfile"
Require-Contains $dockerfile "go build" "Dockerfile"

$deployments = Get-Content -LiteralPath "deploy\kubernetes\deployments.yaml" -Raw
$servicesYaml = Get-Content -LiteralPath "deploy\kubernetes\services.yaml" -Raw
$configMap = Get-Content -LiteralPath "deploy\kubernetes\configmap.yaml" -Raw
$secret = Get-Content -LiteralPath "deploy\kubernetes\secret.example.yaml" -Raw

$httpServices = @(
    "api-gateway",
    "auth-service",
    "event-registration-service",
    "feed-service",
    "notification-service",
    "media-service"
)
$workers = @(
    "feed-worker",
    "notification-worker",
    "media-worker",
    "outbox-relay"
)
$allDeployments = $httpServices + $workers

foreach ($name in $allDeployments) {
    Require-Contains $deployments "name: $name" "deployments.yaml"
    Require-Contains $deployments "image: cityevents/$($name):dev" "deployments.yaml"
}

foreach ($name in $httpServices) {
    Require-Contains $servicesYaml "name: $name" "services.yaml"
}

if ((Count-Matches $deployments "readinessProbe:") -lt $allDeployments.Count) {
    throw "each deployment must define a readinessProbe."
}
if ((Count-Matches $deployments "livenessProbe:") -lt $allDeployments.Count) {
    throw "each deployment must define a livenessProbe."
}
if ((Count-Matches $deployments "resources:") -lt $allDeployments.Count) {
    throw "each deployment must define resource requests and limits."
}
if ((Count-Matches $deployments "requests:") -lt $allDeployments.Count) {
    throw "each deployment must define resource requests."
}
if ((Count-Matches $deployments "limits:") -lt $allDeployments.Count) {
    throw "each deployment must define resource limits."
}
if ((Count-Matches $deployments "/readyz") -lt $httpServices.Count) {
    throw "each HTTP service must use /readyz readiness probe."
}
if ((Count-Matches $deployments "/livez") -lt $httpServices.Count) {
    throw "each HTTP service must use /livez liveness probe."
}

Require-Contains $configMap "CITYEVENTS_ENV" "configmap.yaml"
Require-Contains $configMap "HTTP_ADDR" "configmap.yaml"
Require-Contains $secret "POSTGRES_URL" "secret.example.yaml"
Require-Contains $secret "RABBITMQ_URL" "secret.example.yaml"
Require-Contains $secret "JWT_SECRET" "secret.example.yaml"

$kubectl = Get-Command kubectl -ErrorAction SilentlyContinue
if ($kubectl) {
    Write-Host "== kubectl kustomize =="
    Invoke-Checked "kubectl" @("kustomize", "deploy/kubernetes")

    $kubeConfigPath = $env:KUBECONFIG
    if ([string]::IsNullOrWhiteSpace($kubeConfigPath)) {
        $kubeConfigPath = Join-Path $env:USERPROFILE ".kube\config"
    }
    $canReadKubeConfig = $false
    try {
        if (Test-Path -LiteralPath $kubeConfigPath) {
            $stream = [System.IO.File]::Open($kubeConfigPath, "Open", "Read", "ReadWrite")
            $stream.Close()
            $canReadKubeConfig = $true
        }
    } catch {
        $canReadKubeConfig = $false
    }

    if ($canReadKubeConfig) {
        Write-Host "== kubectl client dry-run =="
        Invoke-Checked "kubectl" @("apply", "--dry-run=client", "--validate=false", "-k", "deploy/kubernetes")
    } else {
        Write-Host "kubectl config is not readable; skipped client dry-run."
    }
} else {
    Write-Host "kubectl not found; skipped client dry-run."
}

Write-Host "Phase 10 verification completed."
