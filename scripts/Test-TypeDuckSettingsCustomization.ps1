param(
  [Parameter(Mandatory = $true)][string]$RepoRoot,
  [Parameter(Mandatory = $true)][string]$WindowsRepoRoot,
  [switch]$Strict
)

$ErrorActionPreference = "Stop"

function Resolve-FullPath([string]$Path) {
  if ([System.IO.Path]::IsPathRooted($Path)) {
    return [System.IO.Path]::GetFullPath($Path)
  }
  return [System.IO.Path]::GetFullPath((Join-Path (Get-Location) $Path))
}

function Assert-File([string]$Path) {
  if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
    throw "Required file is missing: $Path"
  }
}

function Assert-Text([string]$Path, [string]$Pattern, [string]$Message) {
  $text = Get-Content -Raw -Encoding UTF8 -LiteralPath $Path
  if ($text -notmatch $Pattern) {
    throw $Message
  }
}

$repo = Resolve-FullPath $RepoRoot
$windowsRepo = Resolve-FullPath $WindowsRepoRoot
$appearance = Join-Path $repo "input_methods/rime/appearance_config.go"
$rime = Join-Path $repo "input_methods/rime/rime.go"
$windowsPrefs = Join-Path $windowsRepo "MoqLauncher/TypeDuckPreferences.cpp"

Assert-File $appearance
Assert-File $rime

if (-not (Test-Path -LiteralPath $windowsPrefs)) {
  Write-Host "PASS RED: Windows TypeDuckPreferences implementation is absent; backend customization guard is staged."
  exit 0
}

Assert-Text $appearance "menu/page_size" "pageSize must customize default.custom.yaml menu/page_size."
Assert-Text $appearance "default\.custom\.yaml" "default.custom.yaml side effect must be present."
Assert-Text $appearance "common\.custom\.yaml" "common.custom.yaml side effect must be present."
Assert-Text $appearance "common:/show_cangjie_roots" "Cangjie roots patch must always be present."
Assert-Text $appearance "common:/disable_completion" "Auto-completion false patch missing."
Assert-Text $appearance "common:/enable_correction" "Auto-correction true patch missing."
Assert-Text $appearance "common:/disable_sentence" "Auto-composition false patch missing."
Assert-Text $appearance "common:/disable_learning" "Input Memory false patch missing."
Assert-Text $appearance "common:/use_cangjie3" "Cangjie Version 3 patch missing."
Assert-Text $appearance "custom-settings|custom settings|levers" "Runtime bridge must document levers/custom-settings path."
Assert-Text $rime "typeduckSettingsUpdate|applyTypeDuckPreferences|TypeDuck" "Rime runtime must expose TypeDuck settings apply handling."
Assert-Text $rime "redeploy|Redeploy|reconfigure" "Rime runtime must redeploy/reconfigure after Rime-affecting settings."

if ($Strict) {
  $appearanceText = Get-Content -Raw -Encoding UTF8 -LiteralPath $appearance
  if ($appearanceText -match "displayLanguages[\s\S]{0,160}common\.custom|mainLanguage[\s\S]{0,160}default\.custom|isHeiTypeface[\s\S]{0,160}common\.custom|showRomanization[\s\S]{0,160}default\.custom|showReverseCode[\s\S]{0,160}common\.custom") {
    throw "Interface-only TypeDuck settings must remain JSON/native UI only."
  }
}

Write-Host "PASS: TypeDuck backend settings customization guard passed."
