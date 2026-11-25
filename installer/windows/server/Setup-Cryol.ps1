#Requires -Version 5.1
[CmdletBinding()]
param(
    [string]$ProjectPath,
    [ValidateRange(1,65535)][int]$DatabasePort = 5432,
    [ValidateRange(1,65535)][int]$AppPort = 8080,
    [switch]$ResetPostgresPassword,
    [switch]$NoBrowser
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Invoke-Native {
    param([string]$File, [string[]]$Arguments)
    & $File @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$([IO.Path]::GetFileName($File)) failed (exit $LASTEXITCODE). See output above." }
}
function ConvertTo-SqlLiteral {
    param([string]$Value)
    return "'" + $Value.Replace("'", "''") + "'"
}
function Read-Secret {
    param([string]$Prompt)
    $secure = Read-Host $Prompt -AsSecureString
    return ([PSCredential]::new('cryol', $secure)).GetNetworkCredential().Password
}
function Protect-LocalPath {
    param([string]$Path)
    $sid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
    $rights = if (Test-Path -LiteralPath $Path -PathType Container) { ':(OI)(CI)F' } else { ':(F)' }
    Invoke-Native 'icacls.exe' @($Path, '/inheritance:r', '/grant:r',
        ('*' + $sid + $rights), ('*S-1-5-18' + $rights), ('*S-1-5-32-544' + $rights)) | Out-Null
}
function Refresh-ToolPath {
    $env:Path = [Environment]::GetEnvironmentVariable('Path', 'Machine') + ';' +
        [Environment]::GetEnvironmentVariable('Path', 'User') +
        ';C:\Program Files\Git\cmd;C:\Program Files\Go\bin;C:\Program Files\nodejs'
}
function Install-Package {
    param([string]$Id, [string[]]$Extra = @(), [switch]$Upgrade)
    if (!(Get-Command winget.exe -ErrorAction SilentlyContinue)) {
        throw 'Install or update Microsoft App Installer (winget), then run setup again.'
    }
    Write-Host "Installing $Id..."
    $verb = if ($Upgrade) { 'upgrade' } else { 'install' }
    Invoke-Native 'winget.exe' (@($verb, '--id', $Id, '--exact', '--source', 'winget',
        '--accept-package-agreements', '--accept-source-agreements', '--disable-interactivity') + $Extra)
    Refresh-ToolPath
}
function Ensure-Tools {
    Refresh-ToolPath
    if (!(Get-Command git.exe -ErrorAction SilentlyContinue)) { Install-Package 'Git.Git' }
    if (!(Get-Command node.exe -ErrorAction SilentlyContinue)) { Install-Package 'OpenJS.NodeJS.LTS' }
    $nodeVersion = [version]((& node.exe --version).TrimStart('v'))
    if ($nodeVersion -lt [version]'22.12.0') {
        Install-Package 'OpenJS.NodeJS.LTS' -Upgrade
        if ([version]((& node.exe --version).TrimStart('v')) -lt [version]'22.12.0') { throw 'Node.js upgrade did not meet the minimum 22.12 version.' }
    }
    if (!(Get-Command go.exe -ErrorAction SilentlyContinue)) { Install-Package 'GoLang.Go' }
    $goVersion = & go.exe version
    if ($LASTEXITCODE -ne 0 -or $goVersion -notmatch 'go(\d+\.\d+(?:\.\d+)?)') { throw 'Cannot determine the Go version.' }
    if ([version]$Matches[1] -lt [version]'1.26.4') {
        Install-Package 'GoLang.Go' -Upgrade
        $goVersion = & go.exe version
        if ($goVersion -notmatch 'go(\d+\.\d+(?:\.\d+)?)' -or [version]$Matches[1] -lt [version]'1.26.4') { throw 'Go upgrade did not meet the minimum 1.26.4 version.' }
    }
    if (!(Get-Command corepack.cmd -ErrorAction SilentlyContinue)) {
        Invoke-Native 'npm.cmd' @('install', '--global', 'corepack@0.34.6')
        Refresh-ToolPath
    }
    Invoke-Native 'go.exe' @('version')
    Invoke-Native 'node.exe' @('--version')
    Invoke-Native 'corepack.cmd' @('--version')
}
function Get-PostgresInstallation {
    param([int]$Port)
    $found = @()
    foreach ($service in @(Get-CimInstance Win32_Service | Where-Object { $_.PathName -match 'pg_ctl\.exe.*runservice' })) {
        if ($service.PathName -notmatch '^\s*"([^"]+pg_ctl\.exe)"') { continue }
        $bin = Split-Path $Matches[1]
        if ($service.PathName -notmatch '(?:^|\s)-D\s+"([^"]+)"') { continue }
        $data = $Matches[1]
        $postgres = Join-Path $bin 'postgres.exe'
        if (!(Test-Path -LiteralPath $postgres)) { continue }
        $configuredPort = & $postgres -C port -D $data
        if ($LASTEXITCODE -ne 0 -or "$configuredPort".Trim() -ne "$Port") { continue }
        $hba = & $postgres -C hba_file -D $data
        if ($LASTEXITCODE -ne 0) { throw 'Cannot locate PostgreSQL authentication configuration.' }
        $hba = "$hba".Trim()
        if (![IO.Path]::IsPathRooted($hba)) { $hba = Join-Path $data $hba }
        $found += [PSCustomObject]@{ Service = $service.Name; Bin = $bin; Data = $data; Hba = $hba }
    }
    if ($found.Count -gt 1) { throw "Multiple PostgreSQL services use port $Port." }
    if ($found.Count -eq 1) { return $found[0] }
    return $null
}
function Ensure-Postgres {
    param([int]$Port, [string]$Password)
    $installation = Get-PostgresInstallation $Port
    if (!$installation) {
        $other = @(Get-CimInstance Win32_Service | Where-Object { $_.PathName -match 'pg_ctl\.exe.*runservice' })
        if ($other.Count -gt 0) {
            throw "PostgreSQL exists but could not be matched to port $Port. Rerun with -DatabasePort ACTUAL_PORT. Existing services were not changed."
        }
        if ($Password -match '[\r\n]') { throw 'Database passwords cannot contain line breaks.' }
        $options = Join-Path $env:TEMP ("cryol-pg-" + [guid]::NewGuid().ToString('N') + '.txt')
        New-Item -ItemType File -Path $options | Out-Null
        Protect-LocalPath $options
        try {
            [IO.File]::WriteAllText($options, "mode=unattended" + [Environment]::NewLine +
                "superpassword=$Password" + [Environment]::NewLine +
                "serverport=$Port" + [Environment]::NewLine +
                "enable-components=server,commandlinetools" + [Environment]::NewLine,
                [Text.UTF8Encoding]::new($false))
            Install-Package 'PostgreSQL.PostgreSQL.17' @('--override', "--optionfile " + [char]34 + $options + [char]34)
        }
        finally { Remove-Item -LiteralPath $options -Force -ErrorAction SilentlyContinue }
        $installation = Get-PostgresInstallation $Port
        if (!$installation) { throw 'PostgreSQL installation did not register the expected service.' }
    }
    $psqlVersion = & (Join-Path $installation.Bin 'psql.exe') --version
    if ($LASTEXITCODE -ne 0 -or $psqlVersion -notmatch 'PostgreSQL\) (\d+)') { throw 'Cannot determine PostgreSQL version.' }
    if ([int]$Matches[1] -lt 17) { throw 'PostgreSQL 17 or newer is required. Existing installation was preserved.' }
    Write-Host $psqlVersion
    Start-Service -Name $installation.Service
    $ready = Join-Path $installation.Bin 'pg_isready.exe'
    $available = $false
    for ($attempt = 0; $attempt -lt 30; $attempt++) {
        & $ready -h 127.0.0.1 -p $Port | Out-Null
        if ($LASTEXITCODE -eq 0) { $available = $true; break }
        Start-Sleep -Seconds 1
    }
    if (!$available) { throw "PostgreSQL did not become ready on port $Port." }
    return $installation
}

function Invoke-DatabaseSql {
    param([string]$Database, [string]$User, [string]$Password, [string]$Sql)
    $previousPassword = $env:PGPASSWORD
    $previousEncoding = $env:PGCLIENTENCODING
    $previousOutputEncoding = $OutputEncoding
    try {
        $env:PGPASSWORD = $Password
        $env:PGCLIENTENCODING = 'UTF8'
        $OutputEncoding = [Text.UTF8Encoding]::new($false)
        $result = @($Sql | & $script:Psql -X --no-password -h 127.0.0.1 -p $script:DatabasePort -U $User -d $Database -v ON_ERROR_STOP=1 -tA -f - 2>&1)
        if ($LASTEXITCODE -ne 0) {
            throw "PostgreSQL command failed for role '$User' and database '$Database'. Check the port, password, and ownership."
        }
        return ($result -join [Environment]::NewLine).Trim()
    }
    finally {
        $env:PGPASSWORD = $previousPassword
        $env:PGCLIENTENCODING = $previousEncoding
        $OutputEncoding = $previousOutputEncoding
    }
}
function Reset-LocalPostgresPassword {
    param([string]$HbaPath, [string]$Password)
    $original = [IO.File]::ReadAllBytes($HbaPath)
    $backup = "$HbaPath.cryol-recovery-$([guid]::NewGuid().ToString('N')).bak"
    [IO.File]::WriteAllBytes($backup, $original)
    $rule = [Text.Encoding]::ASCII.GetBytes("host postgres postgres 127.0.0.1/32 trust" + [Environment]::NewLine)
    try {
        $temporary = $original
        if ($original.Length -ge 3 -and $original[0] -eq 239 -and $original[1] -eq 187 -and $original[2] -eq 191) {
            $temporary = $original[3..($original.Length - 1)]
        }
        [IO.File]::WriteAllBytes($HbaPath, [byte[]]($rule + $temporary))
        $literal = ConvertTo-SqlLiteral $Password
        Invoke-DatabaseSql 'postgres' 'postgres' '' "SET standard_conforming_strings=on; ALTER ROLE postgres WITH PASSWORD $literal;" | Out-Null
    }
    finally {
        [IO.File]::WriteAllBytes($HbaPath, $original)
        Write-Host 'Original PostgreSQL authentication configuration restored.'
    }
    Invoke-DatabaseSql 'postgres' 'postgres' $Password 'SELECT 1;' | Out-Null
}
function Initialize-CryolDatabase {
    param([string]$AdminPassword, [string]$DatabasePassword)
    $owner = Invoke-DatabaseSql 'postgres' 'postgres' $AdminPassword "SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname='cryol';"
    if ($owner -and $owner -ne 'cryol') { throw "Existing database cryol belongs to '$owner'. It was not changed." }
    $role = Invoke-DatabaseSql 'postgres' 'postgres' $AdminPassword "SELECT rolname FROM pg_roles WHERE rolname='cryol';"
    if ($role) {
        Invoke-DatabaseSql 'postgres' 'cryol' $DatabasePassword 'SELECT 1;' | Out-Null
        $privileges = Invoke-DatabaseSql 'postgres' 'postgres' $AdminPassword "SELECT rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls FROM pg_roles WHERE rolname='cryol';"
        if ($privileges -ne 'f') { throw 'Existing cryol role has elevated privileges. Use a dedicated unprivileged login.' }
    }
    else {
        $literal = ConvertTo-SqlLiteral $DatabasePassword
        Invoke-DatabaseSql 'postgres' 'postgres' $AdminPassword "SET standard_conforming_strings=on; CREATE ROLE cryol LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD $literal;" | Out-Null
    }
    if (!$owner) { Invoke-DatabaseSql 'postgres' 'postgres' $AdminPassword 'CREATE DATABASE cryol OWNER cryol;' | Out-Null }
    Invoke-DatabaseSql 'cryol' 'cryol' $DatabasePassword 'SELECT 1;' | Out-Null
}
function Import-CryolEnvironment {
    param([string]$Path)
    foreach ($line in [IO.File]::ReadAllLines($Path)) {
        if ($line.Trim() -eq '' -or $line.TrimStart().StartsWith('#')) { continue }
        if ($line -notmatch '^\s*(CRYOL_[A-Z0-9_]+)\s*=(.*)$') { throw 'Unsupported .env syntax. Use CRYOL_KEY=value entries without shell commands.' }
        $name = $Matches[1]
        $value = $Matches[2].Trim()
        if ($value.Length -ge 2 -and (($value.StartsWith("'") -and $value.EndsWith("'")) -or ($value.StartsWith('"') -and $value.EndsWith('"')))) {
            $value = $value.Substring(1, $value.Length - 2)
        }
        [Environment]::SetEnvironmentVariable($name, $value, 'Process')
    }
}
function Export-CryolEnvironment {
    param([string]$Path, [string]$DatabasePassword, [int]$Port, [int]$HttpPort, [string]$WebDirectory)
    if (Test-Path -LiteralPath $Path) { throw 'Refusing to overwrite existing .env.' }
    $encoded = [Uri]::EscapeDataString($DatabasePassword)
    $url = 'postgres://cryol:' + $encoded + "@127.0.0.1:$Port/cryol?sslmode=disable"
    if ($WebDirectory -match "[\r\n']") { throw 'Project paths containing apostrophes or line breaks are unsupported.' }
    New-Item -ItemType File -Path $Path | Out-Null
    Protect-LocalPath $Path
    [IO.File]::WriteAllText($Path, "CRYOL_DATABASE_URL='$url'" + [Environment]::NewLine +
        "CRYOL_HTTP_ADDR='127.0.0.1:$HttpPort'" + [Environment]::NewLine +
        "CRYOL_WEB_DIR='$WebDirectory'" + [Environment]::NewLine, [Text.UTF8Encoding]::new($false))
}
function Get-CryolDatabasePassword {
    param([string]$Url, [int]$Port)
    $uri = $null
    if (![Uri]::TryCreate($Url, [UriKind]::Absolute, [ref]$uri)) { throw 'CRYOL_DATABASE_URL is invalid.' }
    if ($uri.Scheme -ne 'postgres' -or $uri.Host -ne '127.0.0.1' -or $uri.Port -ne $Port -or $uri.AbsolutePath -ne '/cryol' -or $uri.Query -ne '?sslmode=disable') {
        throw 'This launcher supports only postgres://cryol:...@127.0.0.1:PORT/cryol. Existing .env was preserved.'
    }
    $parts = $uri.UserInfo -split ':', 2
    if ($parts.Count -ne 2 -or $parts[0] -ne 'cryol') { throw 'The local launcher requires the dedicated cryol database role.' }
    return [Uri]::UnescapeDataString($parts[1])
}

function Build-CryolAgentDownloads {
    param([string]$Root, [string]$Directory)
    New-Item -ItemType Directory -Path $Directory -Force | Out-Null
    $previous = @{}
    foreach ($key in @('GOOS', 'GOARCH', 'CGO_ENABLED')) {
        $previous[$key] = [Environment]::GetEnvironmentVariable($key, 'Process')
    }
    $targets = @(
        @{ OS = 'windows'; Arch = 'amd64'; File = 'cryol-agent.exe' },
        @{ OS = 'linux'; Arch = 'amd64'; File = 'cryol-agent' },
        @{ OS = 'linux'; Arch = 'arm64'; File = 'cryol-agent-linux-arm64' },
        @{ OS = 'darwin'; Arch = 'amd64'; File = 'cryol-agent-macos-intel' },
        @{ OS = 'darwin'; Arch = 'arm64'; File = 'cryol-agent-macos-arm64' }
    )
    Push-Location (Join-Path $Root 'apps/server')
    try {
        $env:CGO_ENABLED = '0'
        foreach ($target in $targets) {
            $env:GOOS = $target.OS
            $env:GOARCH = $target.Arch
            Write-Host ("Building agent download: " + $target.File)
            $output = Join-Path $Directory $target.File
            Invoke-Native 'go.exe' @('build', '-buildvcs=false', '-trimpath', '-o', $output, './cmd/cryol-agent')
            if (!(Test-Path -LiteralPath $output -PathType Leaf) -or (Get-Item -LiteralPath $output).Length -eq 0) {
                throw ("Agent build is missing: " + $target.File)
            }
        }
    }
    finally {
        Pop-Location
        foreach ($key in $previous.Keys) {
            [Environment]::SetEnvironmentVariable($key, $previous[$key], 'Process')
        }
    }
}
function Enable-CryolLocalAgentDownloads {
    param([string]$EnvPath, [string]$Directory)
    if (!(Test-Path -LiteralPath $EnvPath -PathType Leaf)) { throw 'Configure the database .env before agent downloads.' }
    if (!$Directory -or $Directory -match "[\r\n']") { throw 'Agent download directory contains unsupported characters.' }
    $content = [IO.File]::ReadAllText($EnvPath)
    $settings = [ordered]@{
        CRYOL_AGENT_DOWNLOAD_DIR = $Directory
        CRYOL_AGENT_LOCAL_HTTP = '1'
    }
    foreach ($key in $settings.Keys) {
        $line = $key + "='" + $settings[$key] + "'"
        $pattern = '(?m)^[ \t]*' + [regex]::Escape($key) + '[ \t]*=[^\r\n]*'
        if ([regex]::IsMatch($content, $pattern)) {
            # A delegate preserves literal dollar signs in Windows directory names.
            $content = [regex]::Replace($content, $pattern, [Text.RegularExpressions.MatchEvaluator]{ param($match) $line })
        }
        else {
            if ($content -and !$content.EndsWith([Environment]::NewLine)) { $content += [Environment]::NewLine }
            $content += $line + [Environment]::NewLine
        }
        [Environment]::SetEnvironmentVariable($key, $settings[$key], 'Process')
    }
    Protect-LocalPath $EnvPath
    [IO.File]::WriteAllText($EnvPath, $content, [Text.UTF8Encoding]::new($false))
}

function Start-CryolSetup {
    $principal = [Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
    if (!$principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        throw 'Run CryolSetup.exe, or launch PowerShell as Administrator to run this script.'
    }
    Write-Host 'Cryol local Windows setup'
    Write-Host 'Private GitHub access, internet access, and Windows administrator rights are required.'
    if (!$ProjectPath) {
        $default = Join-Path ([Environment]::GetFolderPath('MyDocuments')) 'cryol-asset-management'
        $selection = Read-Host "Project folder (existing checkout or empty destination) [$default]"
        if (!$selection) { $selection = $default }
        $ProjectPath = $selection.Trim().Trim('"')
    }
    $ProjectPath = [IO.Path]::GetFullPath($ProjectPath)
    if ($ProjectPath -match "[\r\n']") { throw 'Choose a project path without apostrophes or line breaks.' }
    Ensure-Tools
    if (Test-Path -LiteralPath (Join-Path $ProjectPath '.git')) {
        $remote = & git.exe -c "safe.directory=$ProjectPath" -C $ProjectPath remote get-url origin
        if ($LASTEXITCODE -ne 0 -or "$remote".Trim() -notmatch '^(https://github\.com/|git@github\.com:)iamraghavan/cryol-asset-management(?:\.git)?$') {
            throw 'The selected folder is not the expected Cryol repository.'
        }
        Write-Host 'Using the existing checkout; local changes and branch are preserved.'
        Invoke-Native 'git.exe' @('-c', "safe.directory=$ProjectPath", '-C', $ProjectPath, 'status', '--short', '--branch')
    }
    else {
        if ((Test-Path -LiteralPath $ProjectPath) -and @(Get-ChildItem -LiteralPath $ProjectPath -Force).Count -gt 0) {
            throw 'Destination is not empty. Select an empty folder or a Cryol checkout.'
        }
        New-Item -ItemType Directory -Path (Split-Path $ProjectPath) -Force | Out-Null
        Invoke-Native 'git.exe' @('clone', 'https://github.com/iamraghavan/cryol-asset-management.git', $ProjectPath)
    }
    Set-Location -LiteralPath $ProjectPath
    $envFile = Join-Path $ProjectPath '.env'
    $local = Join-Path $ProjectPath '.cryol-local'
    New-Item -ItemType Directory -Path $local -Force | Out-Null
    Protect-LocalPath $local
    $server = Join-Path $local 'cryol-server.exe'
    if (Test-Path -LiteralPath $envFile) {
        Import-CryolEnvironment $envFile
        Protect-LocalPath $envFile
        if (!$env:CRYOL_DATABASE_URL) { throw 'Existing .env does not define CRYOL_DATABASE_URL.' }
        $password = Get-CryolDatabasePassword $env:CRYOL_DATABASE_URL $DatabasePort
        if ($env:CRYOL_HTTP_ADDR -and $env:CRYOL_HTTP_ADDR -ne "127.0.0.1:$AppPort") {
            throw 'Existing CRYOL_HTTP_ADDR differs. Rerun with the matching -AppPort, or review your existing deployment.'
        }
        Write-Host 'Existing .env preserved. Its database credentials will be verified.'
    }
    else {
        $password = Read-Secret 'Choose the local PostgreSQL password (enter your requested password here)'
        if (!$password) { throw 'Database password cannot be empty.' }
        $confirm = Read-Secret 'Repeat the database password'
        if ($password -cne $confirm) { throw 'Database passwords did not match. Rerun setup.' }
    }
    $installation = Ensure-Postgres $DatabasePort $password
    $script:Psql = Join-Path $installation.Bin 'psql.exe'
    $adminPassword = $password
    $connected = $false
    try { Invoke-DatabaseSql 'postgres' 'postgres' $adminPassword 'SELECT 1;' | Out-Null; $connected = $true } catch {}
    if (!$connected -and !$ResetPostgresPassword) {
        $adminPassword = Read-Secret 'Existing postgres administrator password (Enter if forgotten)'
        if ($adminPassword) {
            try { Invoke-DatabaseSql 'postgres' 'postgres' $adminPassword 'SELECT 1;' | Out-Null; $connected = $true } catch {}
        }
    }
    if (!$connected) {
        if (!$ResetPostgresPassword) {
            $answer = Read-Host 'Type RESET to set the local postgres administrator password to your chosen database password'
            if ($answer -cne 'RESET') { throw 'Administrator access is required. No password was reset.' }
        }
        Reset-LocalPostgresPassword $installation.Hba $password
        $adminPassword = $password
    }
    Initialize-CryolDatabase $adminPassword $password
    $tables = Invoke-DatabaseSql 'cryol' 'cryol' $password "SELECT count(*) FROM information_schema.tables WHERE table_schema='public';"
    if ([int]$tables -gt 0) {
        $isCryol = Invoke-DatabaseSql 'cryol' 'cryol' $password "SELECT to_regclass('public.installation_info') IS NOT NULL;"
        if ($isCryol -ne 't') { throw 'Existing database is not recognized as Cryol. No migrations were applied.' }
    }
    $env:CRYOL_DATABASE_URL = 'postgres://cryol:' + [Uri]::EscapeDataString($password) + "@127.0.0.1:$DatabasePort/cryol?sslmode=disable"
    $env:CRYOL_HTTP_ADDR = "127.0.0.1:$AppPort"
    $env:CRYOL_WEB_DIR = Join-Path $ProjectPath 'apps\web\dist'
    if ($env:CRYOL_TLS_CERT -or $env:CRYOL_TLS_KEY) {
        throw 'TLS configuration detected. Use the HTTPS deployment instructions; this launcher serves loopback HTTP.'
    }
    if (!(Test-Path -LiteralPath $envFile)) {
        Export-CryolEnvironment $envFile $password $DatabasePort $AppPort $env:CRYOL_WEB_DIR
    }
    $listeners = @(Get-NetTCPConnection -LocalPort $AppPort -State Listen -ErrorAction SilentlyContinue)
    if ($listeners.Count -gt 0) {
        $owner = Get-Process -Id $listeners[0].OwningProcess -ErrorAction Stop
        if ($owner.Path -ne $server) {
            throw "Port $AppPort is occupied. Stop that process or choose -AppPort with an unused port."
        }
        Write-Host 'Stopping this checkout server so setup can apply updated environment and binaries.'
        Stop-Process -Id $owner.Id -ErrorAction Stop
        Wait-Process -Id $owner.Id -Timeout 15 -ErrorAction SilentlyContinue
        if (Get-Process -Id $owner.Id -ErrorAction SilentlyContinue) { throw 'The existing Cryol server did not stop.' }
    }
    if ([int]$tables -gt 0) {
        $backups = Join-Path $env:LOCALAPPDATA 'Cryol\backups'
        New-Item -ItemType Directory -Path $backups -Force | Out-Null
        Protect-LocalPath $backups
        $dump = Join-Path $backups ("cryol-" + [guid]::NewGuid().ToString('N') + '.dump')
        $previous = $env:PGPASSWORD
        try {
            $env:PGPASSWORD = $password
            Invoke-Native (Join-Path $installation.Bin 'pg_dump.exe') @('-h', '127.0.0.1', '-p', "$DatabasePort", '-U', 'cryol', '-d', 'cryol', '-w', '-Fc', '-f', $dump)
            Invoke-Native (Join-Path $installation.Bin 'pg_restore.exe') @('--list', $dump) | Out-Null
        }
        finally { $env:PGPASSWORD = $previous }
        Write-Host "Database backup verified: $dump"
    }
    Invoke-Native 'corepack.cmd' @('pnpm', 'install', '--frozen-lockfile')
    Invoke-Native 'corepack.cmd' @('pnpm', 'build')
    if (!(Test-Path -LiteralPath (Join-Path $env:CRYOL_WEB_DIR 'index.html'))) { throw 'Frontend index.html is missing.' }
    $defaultDownloads = Join-Path $local 'agent-binaries'
    $downloadDirectory = $env:CRYOL_AGENT_DOWNLOAD_DIR
    if (!$downloadDirectory -or $downloadDirectory -eq $defaultDownloads) {
        $downloadDirectory = $defaultDownloads
        Build-CryolAgentDownloads $ProjectPath $downloadDirectory
    }
    elseif (!(Test-Path -LiteralPath (Join-Path $downloadDirectory 'cryol-agent.exe') -PathType Leaf)) {
        throw 'The custom CRYOL_AGENT_DOWNLOAD_DIR has no Windows agent. Stage its binaries, or remove that setting to let local setup build them.'
    }
    Enable-CryolLocalAgentDownloads $envFile $downloadDirectory
    Write-Host 'Agent ZIP downloads enabled for testing on this laptop. Separate endpoints require LAN HTTPS.'
    Push-Location 'apps/server'
    try {
        $cli = Join-Path $local 'cryol-cli.exe'
        Invoke-Native 'go.exe' @('build', '-buildvcs=false', '-o', $cli, './cmd/cryol-cli')
        Invoke-Native 'go.exe' @('build', '-buildvcs=false', '-o', $server, './cmd/cryol-server')
        Invoke-Native $cli @('migrate')
        Invoke-Native $cli @('schema-check')
        $users = Invoke-DatabaseSql 'cryol' 'cryol' $password 'SELECT count(*) FROM app_user;'
        if ([int]$users -eq 0) {
            $username = Read-Host 'First Cryol administrator username [admin]'
            if (!$username) { $username = 'admin' }
            Write-Host 'Enter an application password of at least 15 characters twice. It is separate from PostgreSQL.'
            Invoke-Native $cli @('bootstrap-admin', $username)
        }
    }
    finally { Pop-Location }
    $restart = @'
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot
foreach ($line in [IO.File]::ReadAllLines((Join-Path $root '.env'))) {
    if ($line -match '^\s*(CRYOL_[A-Z0-9_]+)\s*=(.*)$') {
        $key = $Matches[1]
        $value = $Matches[2].Trim()
        if ($value.Length -ge 2 -and (($value.StartsWith("'") -and $value.EndsWith("'")) -or ($value.StartsWith('"') -and $value.EndsWith('"')))) {
            $value = $value.Substring(1, $value.Length - 2)
        }
        [Environment]::SetEnvironmentVariable($key, $value, 'Process')
    }
}
$env:CRYOL_WEB_DIR = Join-Path $root 'apps\web\dist'
& (Join-Path $PSScriptRoot 'cryol-server.exe')
exit $LASTEXITCODE
'@
    [IO.File]::WriteAllText((Join-Path $local 'Start-Cryol.ps1'), $restart, [Text.UTF8Encoding]::new($false))
    $outLog = Join-Path $local 'server.out.log'
    $errLog = Join-Path $local 'server.err.log'
    $process = Start-Process -FilePath $server -WorkingDirectory $ProjectPath -WindowStyle Hidden -PassThru -RedirectStandardOutput $outLog -RedirectStandardError $errLog
    $ready = $false
    try {
        for ($attempt = 0; $attempt -lt 60; $attempt++) {
            $process.Refresh()
            if ($process.HasExited) { throw "Server exited. Inspect $errLog" }
            try {
                $response = Invoke-RestMethod "http://127.0.0.1:$AppPort/api/v1/system/readiness" -TimeoutSec 2
                if ($response.status -eq 'ready') { $ready = $true; break }
            } catch {}
            Start-Sleep -Seconds 1
        }
        if (!$ready) { throw "Readiness timed out. Inspect $errLog" }
    }
    finally {
        if (!$ready -and !$process.HasExited) { Stop-Process -Id $process.Id }
    }
    Write-Host "READY: http://127.0.0.1:$AppPort"
    Write-Host "Server PID: $($process.Id). Stop with: Stop-Process -Id $($process.Id)"
    Write-Host ('Restart later: powershell -NoProfile -ExecutionPolicy Bypass -File "' + $local + '\Start-Cryol.ps1"')
    Write-Host 'Database, schema, frontend, agent binaries, and HTTP readiness checks passed.'
    if (!$NoBrowser) { Start-Process "http://127.0.0.1:$AppPort" }
    if (!$NoBrowser) { Read-Host 'Setup complete. Press Enter to close this window' | Out-Null }
}

# Dot-sourcing exposes functions for validation without changing the machine.
if ($MyInvocation.InvocationName -ne '.') {
    try { Start-CryolSetup }
    catch { Write-Error $_; exit 1 }
}
