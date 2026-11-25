#Requires -Version 5.1
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'Setup-Cryol.ps1')
$root = (Resolve-Path (Join-Path $PSScriptRoot '../../..')).Path
$directory = Join-Path $env:TEMP ('cryol-agent-build-test-' + [guid]::NewGuid().ToString('N'))
$previousOS = $env:GOOS
$previousArch = $env:GOARCH
$previousCGO = $env:CGO_ENABLED
try {
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    $env:CGO_ENABLED = '1'
    Build-CryolAgentDownloads $root $directory
    if ($env:GOOS -ne 'windows' -or $env:GOARCH -ne 'amd64' -or $env:CGO_ENABLED -ne '1') {
        throw 'Cross-compilation changed the subsequent server build environment.'
    }
    foreach ($name in @('cryol-agent.exe', 'cryol-agent', 'cryol-agent-linux-arm64', 'cryol-agent-macos-intel', 'cryol-agent-macos-arm64')) {
        $file = Get-Item -LiteralPath (Join-Path $directory $name)
        if ($file.Length -le 0 -or $file.Length -gt (64 * 1024 * 1024)) { throw "Invalid agent artifact: $name" }
    }
    $windowsBinary = [IO.File]::ReadAllBytes((Join-Path $directory 'cryol-agent.exe'))
    if ($windowsBinary[0] -ne 77 -or $windowsBinary[1] -ne 90) { throw 'Windows agent is not a PE executable.' }
    # An interrupted cross-build must also restore the server build environment.
    $native = (Get-Command Invoke-Native).ScriptBlock
    function Invoke-Native { throw 'Simulated cross-build failure' }
    $failed = $false
    try { Build-CryolAgentDownloads $root $directory } catch { $failed = $true }
    if (!$failed) { throw 'Expected cross-build failure was not propagated.' }
    if ($env:GOOS -ne 'windows' -or $env:GOARCH -ne 'amd64' -or $env:CGO_ENABLED -ne '1') {
        throw 'Failed cross-build leaked environment changes.'
    }
    Set-Item Function:Invoke-Native $native
    Write-Host 'All five agent targets compiled; successful and failed builds preserve the server toolchain environment.'
}
finally {
    $env:GOOS = $previousOS
    $env:GOARCH = $previousArch
    $env:CGO_ENABLED = $previousCGO
    # Only this test invocation owns this generated temporary directory.
    if (Test-Path -LiteralPath $directory) { Remove-Item -LiteralPath $directory -Recurse -Force }
}
$global:LASTEXITCODE = 0
