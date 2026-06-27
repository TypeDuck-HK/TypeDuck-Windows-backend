#Requires -Version 5.1
<#
.SYNOPSIS
  Build the TypeDuck runtime package.

.PARAMETER RepoRoot
  Root of the backend checkout (defaults to the parent directory of this script).

.PARAMETER BuildRoot
  Build output directory (default: scripts\build).

.PARAMETER PackageDir
  Packaged runtime directory (default: scripts\build\TypeDuckRuntime).

.PARAMETER RimeDataSource
  Rime shared data directory to package. By default this prefers the TypeDuck-Web schema
  checkout at I:\GitHub\TypeDuck-Web\schema. Release workflows pass the checked-out
  TypeDuck-HK schema explicitly.
#>
param(
    [string] $RepoRoot = "",
    [string] $BuildRoot = "",
    [string] $PackageDir = "",
    [string] $RimeDataSource = ""
)

$ErrorActionPreference = "Stop"

function Write-Step {
    param([string] $Title)

    Write-Host ""
    Write-Host "============================================"
    Write-Host $Title
    Write-Host "============================================"
    Write-Host ""
}

function Ensure-Directory {
    param([string] $Path)

    New-Item -ItemType Directory -Path $Path -Force | Out-Null
}

function Remove-IfExists {
    param([string] $Path)

    if (Test-Path -LiteralPath $Path) {
        Remove-Item -LiteralPath $Path -Recurse -Force
    }
}

function Invoke-External {
    param(
        [string] $FilePath,
        [string[]] $ArgumentList,
        [switch] $IgnoreExitCode
    )

    Write-Host ">> $FilePath $($ArgumentList -join ' ')"
    & $FilePath @ArgumentList
    $exitCode = $LASTEXITCODE
    if (-not $IgnoreExitCode -and $exitCode -ne 0) {
        throw "Command failed with exit code ${exitCode}: $FilePath"
    }
    return $exitCode
}

function Get-GoToolExecutablePath {
    param([string] $ToolName)

    $command = Get-Command $ToolName -ErrorAction SilentlyContinue
    if ($command) {
        return $command.Source
    }

    $goBin = (& go env GOBIN).Trim()
    if (-not $goBin) {
        $goPath = (& go env GOPATH).Trim()
        if (-not $goPath) {
            throw "Unable to resolve GOPATH for Go tool installation."
        }

        $firstGoPath = $goPath.Split([System.IO.Path]::PathSeparator)[0]
        $goBin = Join-Path $firstGoPath "bin"
    }

    return (Join-Path $goBin ($ToolName + ".exe"))
}

function Get-GoTool {
    param(
        [string] $ToolName,
        [string] $ModuleAtVersion
    )

    $toolPath = Get-GoToolExecutablePath -ToolName $ToolName
    if (Test-Path -LiteralPath $toolPath) {
        return $toolPath
    }

    Write-Host "[INFO] Installing Go tool: $ModuleAtVersion"
    $null = Invoke-External -FilePath "go" -ArgumentList @("install", $ModuleAtVersion)

    $toolPath = Get-GoToolExecutablePath -ToolName $ToolName
    if (-not (Test-Path -LiteralPath $toolPath)) {
        throw "Installed Go tool was not found: $toolPath"
    }

    return $toolPath
}

function Copy-DirectoryContents {
    param(
        [string] $Source,
        [string] $Destination
    )

    Ensure-Directory -Path $Destination
    Copy-Item -Path (Join-Path $Source "*") -Destination $Destination -Recurse -Force
}

function Prepare-RimeData {
    param(
        [string] $RimeDataDir,
        [string] $PackageRimeDataDir
    )

    Remove-IfExists -Path $PackageRimeDataDir
    Ensure-Directory -Path $PackageRimeDataDir

    Write-Host "[INFO] Copying shared data from `"$RimeDataDir`" ..."
    Copy-DirectoryContents -Source $RimeDataDir -Destination $PackageRimeDataDir

    Remove-IfExists -Path (Join-Path $PackageRimeDataDir ".github")
    Remove-IfExists -Path (Join-Path $PackageRimeDataDir ".git")

    foreach ($name in @("README.md", "LICENSE")) {
        $path = Join-Path $PackageRimeDataDir $name
        if (Test-Path -LiteralPath $path) {
            Remove-Item -LiteralPath $path -Force
        }
    }

    Write-Host "[INFO] Packaged Rime shared data prepared at `"$PackageRimeDataDir`""
}

function Remove-PackagePath {
    param(
        [string] $Path,
        [string] $Label
    )

    if (Test-Path -LiteralPath $Path) {
        Remove-Item -LiteralPath $Path -Recurse -Force
        Write-Host "[INFO] Removed packaged $Label"
    }
}

function Write-ServerVersionInfo {
    param(
        [string] $VersionInfoPath,
        [string] $IconPath
    )

    $fileDescription = "TypeDuck Runtime Engine"
    $productName = "TypeDuck Windows IME"

    $versionInfo = [ordered]@{
        FixedFileInfo  = [ordered]@{
            FileVersion    = [ordered]@{
                Major = 1
                Minor = 0
                Patch = 0
                Build = 0
            }
            ProductVersion = [ordered]@{
                Major = 1
                Minor = 0
                Patch = 0
                Build = 0
            }
            FileFlagsMask  = "3f"
            FileFlags      = "00"
            FileOS         = "040004"
            FileType       = "01"
            FileSubType    = "00"
        }
        StringFileInfo = [ordered]@{
            Comments         = ""
            CompanyName      = ""
            FileDescription  = $fileDescription
            FileVersion      = "1.0.0.0"
            InternalName     = "server.exe"
            LegalCopyright   = ""
            LegalTrademarks  = ""
            OriginalFilename = "server.exe"
            PrivateBuild     = ""
            ProductName      = $productName
            ProductVersion   = "1.0.0.0"
            SpecialBuild     = ""
        }
        VarFileInfo    = [ordered]@{
            Translation = [ordered]@{
                LangID    = "0804"
                CharsetID = "04B0"
            }
        }
        IconPath       = $IconPath
        ManifestPath   = ""
    }

    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText(
        $VersionInfoPath,
        ($versionInfo | ConvertTo-Json -Depth 6),
        $utf8NoBom
    )
}

$scriptRepoRoot = Join-Path $PSScriptRoot ".."
if (-not $RepoRoot) { $RepoRoot = $scriptRepoRoot }
$RepoRoot = [System.IO.Path]::GetFullPath($RepoRoot)

if (-not $BuildRoot) { $BuildRoot = Join-Path $PSScriptRoot "build" }
if (-not $PackageDir) { $PackageDir = Join-Path $BuildRoot "TypeDuckRuntime" }

$BuildRoot = [System.IO.Path]::GetFullPath($BuildRoot)
$PackageDir = [System.IO.Path]::GetFullPath($PackageDir)
$ServerExe = Join-Path $PackageDir "server.exe"
$InputMethodsDir = Join-Path $RepoRoot "input_methods"
$IconsDir = Join-Path $RepoRoot "icons"
$RimeDir = Join-Path $InputMethodsDir "rime"
$canonicalAppearanceThemeRelativePath = "input_methods\rime\appearance_themes.json"
if (-not $RimeDataSource) {
    $typeDuckSchema = "I:\GitHub\TypeDuck-Web\schema"
    if (Test-Path -LiteralPath (Join-Path $typeDuckSchema "jyut6ping3.schema.yaml")) {
        $RimeDataSource = $typeDuckSchema
    }
}
if (-not $RimeDataSource) {
    throw "RimeDataSource is required when I:\GitHub\TypeDuck-Web\schema is unavailable. Use the TypeDuck-HK schema checkout on aap2-alpha."
}
$RimeDataDir = [System.IO.Path]::GetFullPath($RimeDataSource)
$PackageRimeDir = Join-Path $PackageDir "input_methods\rime"
$PackageRimeDataDir = Join-Path $PackageRimeDir "data"
$ServerIcon = Join-Path $IconsDir "TypeDuck_Transparent.ico"
$ServerVersionInfo = Join-Path $BuildRoot "server.versioninfo.json"
$ServerResource = Join-Path $RepoRoot "resource_windows_amd64.syso"

Write-Host "============================================"
Write-Host " TypeDuck Runtime Build Script"
Write-Host "============================================"

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Go was not found in PATH. Install Go from https://golang.org/dl/"
}

$goVersion = & go version
if ($LASTEXITCODE -ne 0) {
    throw "Failed to query Go version."
}
Write-Host "[INFO] Go version: $goVersion"

Write-Step -Title "Step 1: Prepare output directory"
if (Test-Path -LiteralPath $PackageDir) {
    Write-Host "[INFO] Removing old build output: `"$PackageDir`""
    Remove-Item -LiteralPath $PackageDir -Recurse -Force
}
Ensure-Directory -Path $PackageDir
Write-Host "[INFO] Output directory: `"$PackageDir`""
Remove-PackagePath -Path (Join-Path $BuildRoot "moqi-ime") -Label "legacy moqi-ime package output"
Remove-PackagePath -Path (Join-Path $BuildRoot ("backends." + "moqi-ime.json")) -Label "legacy backend snippet"

if (-not (Test-Path -LiteralPath (Join-Path $RimeDataDir "default.yaml"))) {
    throw "Missing Rime shared data directory: `"$RimeDataDir`"`nExpected default.yaml or pass -RimeDataSource."
}

Push-Location $RepoRoot
try {
    Write-Step -Title "Step 2: Sync Go dependencies"
    $tidyExitCode = Invoke-External -FilePath "go" -ArgumentList @("mod", "tidy") -IgnoreExitCode
    if ($tidyExitCode -ne 0) {
        Write-Warning "go mod tidy failed, continuing..."
    }

    Write-Step -Title "Step 3: Build go-backend server"
    Write-Host "[INFO] Building server.exe with dynamic DLL loading ..."

    $oldGoos = $env:GOOS
    $oldGoarch = $env:GOARCH
    $oldCgoEnabled = $env:CGO_ENABLED
    $goversioninfo = Get-GoTool -ToolName "goversioninfo" -ModuleAtVersion "github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    $env:CGO_ENABLED = "0"

    try {
        if (-not (Test-Path -LiteralPath $ServerIcon)) {
            throw "Missing server icon: `"$ServerIcon`""
        }

        Write-ServerVersionInfo -VersionInfoPath $ServerVersionInfo -IconPath $ServerIcon
        Remove-IfExists -Path $ServerResource
        $null = Invoke-External -FilePath $goversioninfo -ArgumentList @("-64", "-o", $ServerResource, $ServerVersionInfo)
        $null = Invoke-External -FilePath "go" -ArgumentList @("build", "-ldflags", "-s -w", "-o", $ServerExe, ".")
    }
    finally {
        Remove-IfExists -Path $ServerResource
        Remove-IfExists -Path $ServerVersionInfo
        $env:GOOS = $oldGoos
        $env:GOARCH = $oldGoarch
        $env:CGO_ENABLED = $oldCgoEnabled
    }

    Write-Host "[INFO] Built: `"$ServerExe`""

    Write-Step -Title "Step 4: Copy packaged input_methods"
    if (-not (Test-Path -LiteralPath $RimeDir)) {
        throw "Missing Rime input method directory: `"$RimeDir`""
    }

    $packageInputMethodsDir = Join-Path $PackageDir "input_methods"
    Ensure-Directory -Path $packageInputMethodsDir
    Copy-DirectoryContents -Source $RimeDir -Destination (Join-Path $packageInputMethodsDir "rime")
    Remove-PackagePath -Path (Join-Path $PackageRimeDir "icon.ico") -Label "legacy Rime icon"
    Remove-PackagePath -Path (Join-Path $PackageRimeDir "android") -Label "Android runtime directory"
    Remove-PackagePath -Path (Join-Path $PackageRimeDir "cloudclipboard") -Label "cloud clipboard runtime directory"
    Remove-PackagePath -Path (Join-Path $PackageRimeDir "templates") -Label "template runtime directory"
    Remove-PackagePath -Path (Join-Path $PackageRimeDir "test") -Label "test fixture runtime directory"
    Remove-PackagePath -Path (Join-Path $PackageRimeDir "icons") -Label "duplicate Rime icon directory"
    Remove-PackagePath -Path (Join-Path $PackageRimeDir "ai_config.json") -Label "AI runtime config"
    Remove-PackagePath -Path (Join-Path $PackageRimeDir "ime.json") -Label "backend profile metadata"
    Write-Host "[INFO] Packaged only input_methods\rime"

    Write-Step -Title "Step 5: Confirm source icons"
    if (-not (Test-Path -LiteralPath $IconsDir)) {
        Write-Warning "Missing icons directory: `"$IconsDir`""
    }
    else {
        Write-Host "[INFO] Source icons are available for server.exe resource stamping."
    }

    Write-Step -Title "Step 6: Prepare packaged Rime shared data"
    Prepare-RimeData -RimeDataDir $RimeDataDir -PackageRimeDataDir $PackageRimeDataDir

    $sourceAppearanceThemes = Join-Path $RimeDir "appearance_themes.json"
    $packageAppearanceThemes = Join-Path $PackageRimeDir "appearance_themes.json"
    if (-not (Test-Path -LiteralPath $sourceAppearanceThemes)) {
        throw "Missing builtin appearance themes file: `"$sourceAppearanceThemes`""
    }
    Copy-Item -LiteralPath $sourceAppearanceThemes -Destination $packageAppearanceThemes -Force
    Write-Host "[INFO] Copied canonical TypeDuck appearance themes to $canonicalAppearanceThemeRelativePath"

    $pathsToRemove = @(
        @{ Path = Join-Path $PackageDir "input_methods\rime\data\others"; Label = "rime shared data others directory" },
        @{ Path = Join-Path $PackageDir "input_methods\rime\data\appearance_themes.json"; Label = "duplicate data-path appearance themes" },
        @{ Path = Join-Path $PackageDir "input_methods\rime\data\android"; Label = "Android shared data directory" },
        @{ Path = Join-Path $PackageDir "input_methods\rime\data\cloudclipboard"; Label = "cloud clipboard shared data directory" },
        @{ Path = Join-Path $PackageDir "input_methods\rime\data\templates"; Label = "template shared data directory" },
        @{ Path = Join-Path $PackageDir "input_methods\rime\data\test"; Label = "test shared data directory" },
        @{ Path = Join-Path $PackageDir "input_methods\rime\data\icons"; Label = "duplicate shared data icon directory" }
    )
    foreach ($entry in $pathsToRemove) {
        Remove-PackagePath -Path $entry.Path -Label $entry.Label
    }

    $packagedGoFiles = Get-ChildItem -LiteralPath (Join-Path $PackageDir "input_methods\rime") -Filter "*.go" -Recurse -File -ErrorAction SilentlyContinue
    if ($packagedGoFiles) {
        $packagedGoFiles | Remove-Item -Force
        Write-Host "[INFO] Removed packaged Go source files"
    }

    $rimeDll = Join-Path $RimeDir "rime.dll"
    if (Test-Path -LiteralPath $rimeDll) {
        Copy-Item -LiteralPath $rimeDll -Destination (Join-Path $PackageDir "input_methods\rime\rime.dll") -Force
        Write-Host "[INFO] Copied rime.dll into package output"
    }

    Write-Step -Title "Step 7: Finalize TypeDuck runtime package"
    Write-Host "[INFO] Backend manifest snippets are intentionally not generated; the Windows launcher owns runtime discovery."
}
finally {
    Pop-Location
}

Write-Step -Title "Build completed"
Write-Host "Output directory:"
Write-Host "  `"$PackageDir`""
Write-Host ""
Write-Host "Install target:"
Write-Host "  C:\Program Files (x86)\TypeDuckIME\TypeDuckRuntime"
Write-Host ""
Write-Host "Notes:"
Write-Host "1. The Windows launcher owns the fixed TypeDuck runtime bridge."
Write-Host "2. Ensure TypeDuckRuntime\server.exe exists before installer staging."
Write-Host "3. Ensure TypeDuckRuntime\input_methods\rime contains rime.dll and appearance_themes.json."
Write-Host "4. Start or restart TypeDuckLauncher.exe after install."
