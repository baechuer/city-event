$ErrorActionPreference = "Stop"

$repoRoot = (Resolve-Path -LiteralPath ".").Path
$env:GOCACHE = Join-Path $repoRoot ".cache\go-build"
New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null

$node = "C:\Users\Administrator\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe"
if (-not (Test-Path -LiteralPath $node)) {
    $node = "node"
}

Write-Host "== Backend default Go tests =="
go test ./...

Write-Host "== Frontend JavaScript tests =="
& $node --test frontend/src/*.test.mjs

Write-Host "== Static frontend file checks =="
foreach ($path in @(
    "frontend/index.html",
    "frontend/server.mjs",
    "frontend/src/app.js",
    "frontend/src/api.js",
    "frontend/src/state.js",
    "frontend/src/styles.css"
)) {
    if (-not (Test-Path -LiteralPath $path)) {
        throw "Missing frontend file: $path"
    }
}

Write-Host "Phase 8 verification completed."
