# Installs the whisk CLI from its GitHub releases (CLI.md §2). Windows, amd64 and arm64:
#   irm https://github.com/Whisk-Hosting/cli/releases/latest/download/install.ps1 | iex
# WHISK_INSTALL_DIR sets where it goes (default %LOCALAPPDATA%\Programs\whisk).
$ErrorActionPreference = "Stop"
# Windows PowerShell 5 draws a progress bar per byte, which makes a download many times slower.
$ProgressPreference = "SilentlyContinue"
$releases = "https://github.com/Whisk-Hosting/cli/releases"
# GitHub serves release files as application/octet-stream, which Invoke-WebRequest answers as bytes.
function Get-Text($uri) {
  $content = (Invoke-WebRequest -UseBasicParsing -TimeoutSec 60 -Uri $uri).Content
  if ($content -is [byte[]]) { $content = [System.Text.Encoding]::UTF8.GetString($content) }
  return $content
}
# One release's files throughout: the version the latest release names, then that release.
$version = (Get-Text "$releases/latest/download/VERSION").Trim()
if ($version -notmatch '^[A-Za-z0-9._-]+$') { throw "whisk: could not read the latest version from $releases" }
$base = "$releases/download/$version"
$arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq "Arm64") { "arm64" } else { "amd64" }
$name = "whisk_windows_$arch.exe"
$dir = if ($env:WHISK_INSTALL_DIR) { $env:WHISK_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "Programs\whisk" }
New-Item -ItemType Directory -Force -Path $dir | Out-Null
$tmp = Join-Path $env:TEMP ("whisk-" + [System.Guid]::NewGuid().ToString() + ".exe")
Invoke-WebRequest -UseBasicParsing -TimeoutSec 300 -Uri "$base/$name" -OutFile $tmp
$sums = Get-Text "$base/SHA256SUMS"
$want = ($sums -split "\n" | Where-Object { $_ -match " $name$" } | ForEach-Object { ($_ -split " ")[0] }) | Select-Object -First 1
if (-not $want) { Remove-Item $tmp; throw "whisk: SHA256SUMS has no entry for $name; not installing" }
$got = (Get-FileHash -Algorithm SHA256 $tmp).Hash.ToLower()
if ($got -ne $want.ToLower()) { Remove-Item $tmp; throw "whisk: checksum mismatch for $name" }
Move-Item -Force $tmp (Join-Path $dir "whisk.exe")
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (($userPath -split ";") -notcontains $dir) {
  [Environment]::SetEnvironmentVariable("Path", "$userPath;$dir", "User")
  $env:Path = "$env:Path;$dir"
  Write-Host "added $dir to your PATH (open a new terminal for other windows to see it)"
}
Write-Host "whisk $version installed to $dir\whisk.exe"
Write-Host "next: whisk login"
