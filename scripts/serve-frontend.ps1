param(
    [int]$Port = 18088
)

$ErrorActionPreference = "Stop"

$node = "C:\Users\Administrator\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe"
if (-not (Test-Path -LiteralPath $node)) {
    $node = "node"
}

& $node .\frontend\server.mjs $Port
