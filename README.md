# TypeDuck Windows Backend

TypeDuck Windows Backend is the Go runtime process that powers TypeDuck Windows by handling protobuf IME requests, running the TypeDuck Rime service, and returning composition, candidate, settings, and dictionary data to the Windows frontend.

TypeDuck Windows Backend 係 TypeDuck Windows 嘅 Go runtime：負責處理 protobuf 輸入法請求、執行 TypeDuck Rime 服務，並將組字、候選、設定同字典資料回傳畀 Windows 前端。

## What It Does / 功能

- Runs as `TypeDuckRuntime/server.exe` launched by TypeDuck Windows.
- 由 TypeDuck Windows 啟動為 `TypeDuckRuntime/server.exe`。
- Reads length-prefixed protobuf requests from standard input and writes length-prefixed protobuf responses to standard output.
- 經標準輸入讀取 length-prefixed protobuf 請求，經標準輸出回傳 length-prefixed protobuf 回應。
- Owns the Rime session, key handling, candidate generation, settings side effects, and dictionary lookup comments.
- 負責 Rime session、按鍵處理、候選生成、設定套用同字典式候選註解。
- Produces the runtime package consumed by the TypeDuck Windows installer.
- 產生 TypeDuck Windows 安裝程式使用嘅 runtime package。

## Repository / 倉庫

- Backend runtime: [TypeDuck-HK/TypeDuck-Windows-backend](https://github.com/TypeDuck-HK/TypeDuck-Windows-backend)
- Windows frontend and installer: [TypeDuck-HK/TypeDuck-Windows](https://github.com/TypeDuck-HK/TypeDuck-Windows)

## First Run / 第一次執行

Build the runtime package with a TypeDuck Rime schema source:

請使用 TypeDuck Rime schema source 建立 runtime package：

```powershell
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/build.ps1 -RimeDataSource <schema-source>
```

The build output is:

輸出位置：

```text
scripts/build/TypeDuckRuntime/
```

The Windows installer expects this layout:

Windows 安裝程式預期以下結構：

```text
TypeDuckRuntime/
├── server.exe
└── input_methods/
    └── rime/
        ├── rime.dll
        ├── appearance_themes.json
        └── data/
```

## User-Facing Behavior / 用戶可見行為

The backend is not launched directly by users. It is started by `TypeDuckLauncher.exe` and serves the TypeDuck Cantonese IME profile registered under Chinese (Traditional, Hong Kong) / `zh-HK`.

用戶一般唔會直接開啟 backend。佢由 `TypeDuckLauncher.exe` 啟動，並服務註冊於中文（繁體，香港）/ `zh-HK` 嘅 TypeDuck 粵語輸入法。

## Architecture

```text
TypeDuckLauncher.exe
        |
        | stdin/stdout, protobuf frames
        v
server.exe
        |
        v
imecore request/response adapter
        |
        v
input_methods/rime
        |
        v
librime + TypeDuck Rime data + dictionary lookup filter
```

`server.go` owns process startup, logging, service registration, client sessions, and request dispatch. `protocol_io.go` owns the binary frame format. `imecore/protocol.go` converts between generated protobuf messages and backend request/response structs. `input_methods/rime/` owns the TypeDuck Rime implementation and native librime integration.

## Build Requirements

- Windows for the packaged runtime target.
- Go matching the version declared in `go.mod`.
- PowerShell 7 or newer for repository scripts.
- A TypeDuck Rime schema source supplied through `-RimeDataSource`.
- Packaged Rime engine files, including `rime.dll`, available under `input_methods/rime/`.

## Build

```powershell
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/build.ps1 -RimeDataSource <schema-source>
```

The script:

- Prepares `scripts/build/TypeDuckRuntime`.
- Builds `server.exe` for Windows amd64.
- Copies and prunes `input_methods/rime`.
- Copies TypeDuck Rime shared data into `input_methods/rime/data`.
- Copies `appearance_themes.json`.
- Keeps the runtime layout expected by `TypeDuck-Windows`.

For local compiler-only checks:

```powershell
go test ./...
```

## Protocol

The backend uses the shared protobuf schema in this repository. Incoming requests are decoded into `imecore.Request`; outgoing responses are built from `imecore.Response`.

Important method families:

- Lifecycle: `METHOD_INIT`, `METHOD_CLOSE`, activation/deactivation.
- Key handling: filter and key up/down methods.
- Candidate operations: highlight, select, and page changes.
- TypeDuck settings: `METHOD_TYPEDUCK_SETTINGS_UPDATE`.
- Runtime deploy: `METHOD_TYPEDUCK_DEPLOY`.

Do not edit the generated Go protobuf binding by hand. Update the schema source, regenerate the Go binding, and keep the frontend schema aligned.

## Rime Runtime

The TypeDuck Rime service is implemented in `input_methods/rime/`.

Key files:

- `rime.go` - session lifecycle, request handling, candidate responses, settings, deploy, and notifications.
- `rime_keyevent.go` - Windows key and modifier translation.
- `librime.go` - Rime API wrapper and required module list.
- `native_cgo.go` / `native_stub.go` - platform-specific native binding boundary.
- `appearance_config.go` - settings-to-Rime custom YAML mapping.
- `ime.json` - TypeDuck profile metadata, including `zh-HK` locale and profile GUID.

The backend requests the `dictionary_lookup` Rime module so the packaged `rime-dictionary-lookup-filter` can provide dictionary-style candidate comments.

## Settings

The Windows settings app writes TypeDuck preferences in the frontend repository. The launcher forwards Rime-affecting settings to this backend through `METHOD_TYPEDUCK_SETTINGS_UPDATE`. The backend applies those settings to Rime config and redeploy/reload paths where required.

Settings handled by the backend include candidate page size, completion, correction, sentence composition, learning, and Cangjie version behavior. Interface-only display preferences remain frontend-owned.

## Tests

Run the Go test suite:

```powershell
go test ./...
```

Useful focused runs:

```powershell
go test ./imecore
go test ./input_methods/rime
go test ./mobilebridge
```

Runtime/package checks:

```powershell
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/Test-TypeDuckCandidateParity.ps1 -RepoRoot .
pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/Test-TypeDuckSettingsCustomization.ps1 -RepoRoot .
```

Native Rime tests are gated by the local runtime environment. Use `MOQI_RIME_PACKAGE_DIR`, `MOQI_RIME_INIT_MAX_MS`, and `MOQI_REAL_APPDATA` only for tests that explicitly require them.

## Logging and Data

The backend writes daily logs under the TypeDuck local app data log directory when available, then falls back to temp or current-directory logging.

Rime user data uses the TypeDuck user data folder. Packaged shared data lives beside `server.exe` under `input_methods/rime/data`.

Routine response payloads should not be logged because they can include typed text or candidate contents.

## Release Relationship

The backend repository can build a runtime package independently, but the TypeDuck Windows installer is the release authority for end-user distribution. A release-ready pair of commits must keep these aligned:

- Protobuf schema and generated bindings in both repositories.
- TypeDuck profile GUID and `zh-HK` metadata.
- Runtime package layout.
- Rime dictionary lookup support.
- Settings payload and Rime side effects.
- Installer staging and pruning rules in the frontend repository.

## License

This repository is licensed under the [MIT License](LICENSE).

## Acknowledgement

Thanks to the Moqi IME project for its earlier input-method runtime engineering work.
