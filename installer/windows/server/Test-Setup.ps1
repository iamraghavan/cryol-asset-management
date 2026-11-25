#Requires -Version 5.1
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$scriptPath = Join-Path $PSScriptRoot 'Setup-Cryol.ps1'
$tokens = $null
$parseErrors = $null
[Management.Automation.Language.Parser]::ParseFile($scriptPath, [ref]$tokens, [ref]$parseErrors) | Out-Null
if ($parseErrors.Count) { throw ($parseErrors | Out-String) }
. $scriptPath
function Assert-Equal($Actual, $Expected, $Message) {
    if ($Actual -cne $Expected) { throw $Message }
}
function Assert-Throws([scriptblock]$Action, $Message) {
    $failed = $false
    try { & $Action } catch { $failed = $true }
    if (!$failed) { throw $Message }
}
$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('cryol-tests-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot | Out-Null
try {
    Assert-Equal (ConvertTo-SqlLiteral "a'b\c") "'a''b\c'" 'SQL quoting failed.'
    $password = "example'@:/\%#-secret"
    $url = 'postgres://cryol:' + [Uri]::EscapeDataString($password) + '@127.0.0.1:5432/cryol?sslmode=disable'
    Assert-Equal (Get-CryolDatabasePassword $url 5432) $password 'URI password round-trip failed.'
    Assert-Throws { Get-CryolDatabasePassword ($url.Replace('127.0.0.1', 'example.com')) 5432 } 'Remote database must be rejected.'
    Assert-Throws { Get-CryolDatabasePassword ($url.Replace('/cryol?', '/other?')) 5432 } 'Unexpected database must be rejected.'
    $oldProtect = (Get-Command Protect-LocalPath).ScriptBlock
    function Protect-LocalPath { param($Path) }
    $envPath = Join-Path $testRoot '.env'
    Export-CryolEnvironment $envPath $password 5432 8080 'C:\Project With Spaces\apps\web\dist'
    $bytes = [IO.File]::ReadAllBytes($envPath)
    if ($bytes[0] -eq 239 -and $bytes[1] -eq 187) { throw 'Unexpected UTF-8 BOM.' }
    Import-CryolEnvironment $envPath
    Assert-Equal $env:CRYOL_DATABASE_URL $url '.env did not round-trip.'
    Assert-Equal $env:CRYOL_WEB_DIR 'C:\Project With Spaces\apps\web\dist' 'Space-containing path did not round-trip.'
    $before = [IO.File]::ReadAllText($envPath)
    Assert-Throws { Export-CryolEnvironment $envPath 'other' 5432 8080 'C:\other' } 'Existing .env must not be overwritten.'
    Assert-Equal ([IO.File]::ReadAllText($envPath)) $before 'Existing .env was changed.'
    Set-Item Function:Protect-LocalPath $oldProtect
    # Exercise cleanup even when authentication recovery fails.
    $hba = Join-Path $testRoot 'pg_hba.conf'
    $original = [byte[]](239,187,191,35,32,111,114,105,103,105,110,97,108,13,10)
    [IO.File]::WriteAllBytes($hba, $original)
    $oldSql = (Get-Command Invoke-DatabaseSql).ScriptBlock
    function Invoke-DatabaseSql { throw 'Simulated failed password reset' }
    Assert-Throws { Reset-LocalPostgresPassword $hba 'test-only-secret' } 'Recovery failure was not propagated.'
    Assert-Equal ([Convert]::ToBase64String([IO.File]::ReadAllBytes($hba))) ([Convert]::ToBase64String($original)) 'Authentication file was not restored byte-for-byte.'
    Set-Item Function:Invoke-DatabaseSql $oldSql
    Assert-Throws { Invoke-Native 'powershell.exe' @('-NoProfile', '-Command', 'exit 7') } 'Native nonzero exit was ignored.'
    Assert-Throws { Invoke-Native 'definitely-missing-cryol-test-command' @() } 'Native command failure was ignored.'
    Write-Host 'Setup parsing, credential encoding, env preservation, and recovery rollback checks passed.'
}
finally {
    # Only this test invocation's uniquely named directory is deleted.
    Remove-Item -LiteralPath $testRoot -Recurse -Force
}
