# Linux CI integration test against the disposable PostgreSQL service.
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'Setup-Cryol.ps1')
$script:Psql = (Get-Command psql).Source
$script:DatabasePort = 5432
$adminPassword = $env:CRYOL_CI_POSTGRES_PASSWORD
if (!$adminPassword) { throw 'CI PostgreSQL password is required.' }
$testPassword = "test-only'@\:/%#password"
Initialize-CryolDatabase $adminPassword $testPassword
Initialize-CryolDatabase $adminPassword $testPassword
$identity = Invoke-DatabaseSql 'cryol' 'cryol' $testPassword 'SELECT current_user;'
if ($identity -ne 'cryol') { throw 'Dedicated database identity was not used.' }
try {
    Initialize-CryolDatabase $adminPassword 'wrong-password'
    throw 'Expected credential verification to fail.'
}
catch {
    if ($_.Exception.Message -eq 'Expected credential verification to fail.') { throw }
}
Invoke-DatabaseSql 'cryol' 'cryol' $testPassword 'SELECT 1;' | Out-Null
$env:CRYOL_DATABASE_URL = 'postgres://cryol:' + [Uri]::EscapeDataString($testPassword) + '@127.0.0.1:5432/cryol?sslmode=disable'
$env:CRYOL_HTTP_ADDR = '127.0.0.1:18080'
$env:CRYOL_WEB_DIR = ''
Push-Location (Join-Path $PSScriptRoot '../../../apps/server')
try {
    Invoke-Native 'go' @('run', './cmd/cryol-cli', 'migrate')
    Invoke-Native 'go' @('run', './cmd/cryol-cli', 'migrate')
    Invoke-Native 'go' @('run', './cmd/cryol-cli', 'schema-check')
    $server = Join-Path ([IO.Path]::GetTempPath()) ('cryol-ci-' + [guid]::NewGuid().ToString('N'))
    Invoke-Native 'go' @('build', '-o', $server, './cmd/cryol-server')
    $process = Start-Process -FilePath $server -PassThru
    try {
        $ready = $false
        for ($attempt = 0; $attempt -lt 30; $attempt++) {
            $process.Refresh()
            if ($process.HasExited) { throw 'CI server exited before readiness.' }
            try {
                $status = Invoke-RestMethod 'http://127.0.0.1:18080/api/v1/system/readiness' -TimeoutSec 2
                if ($status.status -eq 'ready') { $ready = $true; break }
            } catch {}
            Start-Sleep -Seconds 1
        }
        if (!$ready) { throw 'CI server did not become ready.' }
    }
    finally { if (!$process.HasExited) { Stop-Process -Id $process.Id }; Remove-Item -LiteralPath $server -Force }
}
finally { Pop-Location }
Write-Host 'Database creation, rerun, credential verification, migrations, schema, and server readiness passed.'
