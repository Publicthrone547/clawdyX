# Builds clawdyx-camera.exe (the Windows camera helper) with MSVC.
# Usage: powershell -File desktop/helper/windows/build.ps1 [-Out <file>] [-Arch x64|arm64]
# Needs the Visual Studio Build Tools ("Desktop development with C++").
param(
    [string]$Out = "bin/windows/clawdyX/helpers/clawdyx-camera.exe",
    [string]$Arch = "x64"
)
$ErrorActionPreference = "Stop"
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$vswhere = Join-Path ${env:ProgramFiles(x86)} "Microsoft Visual Studio\Installer\vswhere.exe"
$vs = & $vswhere -products * -latest -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
if (-not $vs) { throw "Visual Studio Build Tools with C++ were not found" }
$vcvars = Join-Path $vs "VC\Auxiliary\Build\vcvarsall.bat"
$hostArch = if ($Arch -eq "arm64") { "x64_arm64" } else { "x64" }
$outPath = [IO.Path]::GetFullPath($Out)
New-Item -ItemType Directory -Force (Split-Path -Parent $outPath) | Out-Null
$obj = Join-Path ([IO.Path]::GetTempPath()) ("cxcam-" + [Guid]::NewGuid())
New-Item -ItemType Directory -Force $obj | Out-Null
try {
    # /MT links the C runtime statically: no Visual C++ redistributable is needed.
    $res = Join-Path $obj "camera.res"
    $env:PATH = (Split-Path -Parent $vswhere) + ";" + $env:PATH # vcvarsall looks for vswhere on PATH
    $cmd = "`"$vcvars`" $hostArch >nul && rc /nologo /fo `"$res`" `"$here\camera.rc`" && cl /nologo /std:c++17 /O2 /MT /EHsc /W3 /DUNICODE /D_UNICODE /Fo`"$obj\\`" `"$here\camera.cpp`" `"$res`" /Fe`"$outPath`" /link /SUBSYSTEM:CONSOLE"
    cmd /c $cmd
    if ($LASTEXITCODE -ne 0) { throw "cl failed ($LASTEXITCODE)" }
} finally {
    Remove-Item -Recurse -Force $obj -ErrorAction SilentlyContinue
}
Write-Host "built $outPath"
