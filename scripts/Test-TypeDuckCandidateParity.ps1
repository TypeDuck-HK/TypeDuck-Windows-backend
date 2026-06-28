param(
  [string]$RepoRoot = ".",
  [string]$WindowsRepoRoot = "..\moqi-im-windows",
  [switch]$Strict
)

$ErrorActionPreference = "Stop"

function Resolve-FullPath {
  param([string]$Path)
  if ([System.IO.Path]::IsPathRooted($Path)) {
    return [System.IO.Path]::GetFullPath($Path)
  }
  return [System.IO.Path]::GetFullPath((Join-Path (Get-Location) $Path))
}

function Assert-File {
  param([string]$Path, [string]$Label)
  if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
    throw "$Label missing: $Path"
  }
}

function Assert-Contains {
  param([string]$Text, [string]$Pattern, [string]$Description)
  if ($Text -notmatch $Pattern) {
    throw $Description
  }
}

function Get-OptionalHash {
  param([string]$Path)
  if (Test-Path -LiteralPath $Path -PathType Leaf) {
    return (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash
  }
  return $null
}

$BackendRoot = Resolve-FullPath $RepoRoot
$WindowsRoot = Resolve-FullPath $WindowsRepoRoot

$contractPath = Join-Path $WindowsRoot ".planning/product/candidate-fixtures/phase-05/candidate-data-contract.json"
$candidateListPath = Join-Path $WindowsRoot ".planning/product/web-alpha-fixtures/2026-06-23/candidate-list-sample.json"
$dictionaryPath = Join-Path $WindowsRoot ".planning/product/web-alpha-fixtures/2026-06-23/dictionary-detail-sample.json"
$provenancePath = Join-Path $WindowsRoot ".planning/product/ui-fixtures/phase-05/runtime-provenance.json"
$moqiClientPath = Join-Path $WindowsRoot "MoqiTextService/MoqiClient.cpp"
$backendServerPath = Join-Path $WindowsRoot "MoqLauncher/BackendServer.cpp"
$pipeClientPath = Join-Path $WindowsRoot "MoqLauncher/PipeClient.cpp"

Assert-File $contractPath "Candidate data contract"
Assert-File $candidateListPath "Web alpha candidate fixture"
Assert-File $dictionaryPath "Web alpha dictionary fixture"
Assert-File $provenancePath "Runtime provenance"
Assert-File $moqiClientPath "TSF client candidate bridge"
Assert-File $backendServerPath "Launcher backend bridge"
Assert-File $pipeClientPath "Launcher pipe client"

$contract = Get-Content -Raw -Encoding UTF8 -LiteralPath $contractPath | ConvertFrom-Json
$candidateFixtureText = Get-Content -Raw -Encoding UTF8 -LiteralPath $candidateListPath
$dictionaryFixtureText = Get-Content -Raw -Encoding UTF8 -LiteralPath $dictionaryPath
$provenance = Get-Content -Raw -Encoding UTF8 -LiteralPath $provenancePath | ConvertFrom-Json
$moqiClient = Get-Content -Raw -LiteralPath $moqiClientPath
$backendServer = Get-Content -Raw -LiteralPath $backendServerPath
$pipeClient = Get-Content -Raw -LiteralPath $pipeClientPath

foreach ($inputName in @("hou", "housam", "nei", "multilingual", "reverse-lookup")) {
  $matched = @($contract.runtimeParity.samples | Where-Object { $_.input -eq $inputName -or $_.scenario -eq $inputName })
  if ($matched.Count -eq 0) {
    throw "Candidate parity contract is missing required sample '$inputName'."
  }
}

Assert-Contains $candidateFixtureText '"text":\s*"你"' "Web alpha candidate fixture must include nei candidate 你."
Assert-Contains $candidateFixtureText '"text":\s*"尼"' "Web alpha candidate fixture must include nei candidate 尼."
Assert-Contains $dictionaryFixtureText '好心你' "Web alpha dictionary fixture must include housam compound candidate 好心你."
Assert-Contains $dictionaryFixtureText 'More Languages' "Web alpha dictionary fixture must include multilingual dictionary rows."

$expectedRuntimeHash = [string]$provenance.candidateRuntimeDiagnosis.expectedRuntime.rimeDllSha256
$expectedServerHash = [string]$provenance.candidateRuntimeDiagnosis.expectedRuntime.serverSha256
if ([string]::IsNullOrWhiteSpace($expectedRuntimeHash) -or [string]::IsNullOrWhiteSpace($expectedServerHash)) {
  throw "runtime-provenance.json must record expected TypeDuck runtime rime.dll and server hashes."
}

$buildRime = Join-Path $BackendRoot "scripts/build/moqi-ime/input_methods/rime/rime.dll"
$sourceRime = Join-Path $BackendRoot "input_methods/rime/rime.dll"
$buildServer = Join-Path $BackendRoot "scripts/build/moqi-ime/server.exe"
$packagedSchema = Join-Path $BackendRoot "scripts/build/moqi-ime/input_methods/rime/data/jyut6ping3.schema.yaml"
$packagedTemplate = Join-Path $BackendRoot "scripts/build/moqi-ime/input_methods/rime/data/template.yaml"
$packagedBuildSchema = Join-Path $BackendRoot "scripts/build/moqi-ime/input_methods/rime/data/build/jyut6ping3.schema.yaml"
$backendLibrimeGo = Join-Path $BackendRoot "input_methods/rime/librime.go"
Assert-File $sourceRime "TypeDuck source rime.dll"
Assert-File $buildRime "TypeDuck packaged rime.dll"
Assert-File $buildServer "TypeDuck packaged server.exe"
Assert-File $packagedSchema "TypeDuck packaged source schema"
Assert-File $packagedTemplate "TypeDuck packaged source template"
Assert-File $packagedBuildSchema "TypeDuck packaged prebuilt schema"
Assert-File $backendLibrimeGo "Backend librime bridge"

$sourceRimeHash = Get-OptionalHash $sourceRime
$buildRimeHash = Get-OptionalHash $buildRime
$buildServerHash = Get-OptionalHash $buildServer
if ($sourceRimeHash -ne $expectedRuntimeHash -or $buildRimeHash -ne $expectedRuntimeHash) {
  throw "TypeDuck runtime rime.dll hash mismatch. source=$sourceRimeHash build=$buildRimeHash expected=$expectedRuntimeHash"
}
if ($buildServerHash -ne $expectedServerHash) {
  throw "TypeDuck backend server hash mismatch. build=$buildServerHash expected=$expectedServerHash"
}

$diagnosis = $provenance.candidateRuntimeDiagnosis.vmVsPreviewDivergence
if (-not $diagnosis -or [string]::IsNullOrWhiteSpace([string]$diagnosis.rootCause)) {
  throw "runtime-provenance.json must record the VM-vs-preview candidate divergence root cause."
}
if ([string]$diagnosis.disposition -notmatch 'not-accepted-as-current-evidence|fixed') {
  throw "VM-vs-preview divergence disposition must be fixed or explicitly not accepted as current evidence."
}
if ([string]$diagnosis.rootCause -notmatch 'TypeDuck-1\.1\.2|stale|external') {
  throw "VM-vs-preview divergence root cause must name the stale/external runtime path."
}

Assert-Contains $moqiClient 'candidate_entries\(\)' "TSF client must consume structured candidate_entries when present."
Assert-Contains $moqiClient 'raw_lookup_comment\(\)' "TSF client must prefer raw lookup comments for CandidateInfo parsing."
Assert-Contains $moqiClient 'candidate\["comment"\]' "TSF client must pass candidate comments to the renderer boundary."
Assert-Contains $backendServer 'writeBackendResponse' "Backend bridge must forward framed backend protobuf responses."
Assert-Contains $pipeClient 'backend_->handleClientMessage\(this, request\)' "Launcher pipe client must forward requests to the backend."

$packagedSchemaText = Get-Content -Raw -Encoding UTF8 -LiteralPath $packagedSchema
$packagedTemplateText = Get-Content -Raw -Encoding UTF8 -LiteralPath $packagedTemplate
$packagedSourceSchemaText = $packagedSchemaText + "`n" + $packagedTemplateText
$packagedBuildSchemaText = Get-Content -Raw -Encoding UTF8 -LiteralPath $packagedBuildSchema
$backendLibrimeGoText = Get-Content -Raw -LiteralPath $backendLibrimeGo
Assert-Contains $packagedSourceSchemaText 'dictionary_lookup_filter' "Packaged source schema must enable TypeDuck dictionary lookup filter."
Assert-Contains $packagedBuildSchemaText 'dictionary_lookup_filter' "Packaged prebuilt schema must enable TypeDuck dictionary lookup filter."
Assert-Contains $backendLibrimeGoText 'dictionary_lookup' "Backend must request the TypeDuck dictionary_lookup module before dictionary_lookup_filter can be created."
Assert-Contains $backendLibrimeGoText 'Initialize\(traits\)' "Backend must pass RimeTraits modules into RimeInitialize, not initialize with default modules only."

if ($moqiClient -match 'legacy Moqi fallback|Moqi fallback|set_comment\("Moqi|墨奇"') {
  throw "Legacy Moqi candidate fallback/substitution text found in TSF candidate bridge."
}

if ($Strict) {
  $running = Get-Process -ErrorAction SilentlyContinue | Where-Object {
    $_.ProcessName -match 'TypeDuck|Moqi'
  } | Select-Object ProcessName, Id, Path
  $staleRunning = @($running | Where-Object { $_.Path -match 'Rime\\TypeDuck-1\.1\.2' })
  if ($staleRunning.Count -gt 0 -and [string]$diagnosis.disposition -ne 'not-accepted-as-current-evidence') {
    throw "Stale external TypeDuck 1.1.2 process is running but provenance did not exclude it from current evidence."
  }
}

Write-Host "PASS: TypeDuck candidate parity guard passed."
