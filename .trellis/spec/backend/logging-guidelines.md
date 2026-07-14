# Logging Guidelines

> Structured logging conventions for this project's Go backend.

---

## Overview

The project uses a structured logging system built on top of Go's `log/slog` package, adapted through `internal/logging`. Every log entry carries typed fields (component, stage, action, etc.) for filtering and debugging. There are **no free-form string log messages** with inline variables — all dynamic data goes into fields.

---

## Architecture

```
internal/logging/       ← Interfaces (Logger, Entry, Fields, StepToken)
    logger.go           ← Pure Go interface definitions
    (slog adapter)      ← Implementation via NewSlogAdapter()
    
internal/envsetup/
    logger_bridge.go    ← Bridge between envsetup and logging.Logger
                        ← Also emits log entries as Wails runtime events

internal/producews/
    diagnostics.go      ← Separate bounded disk/memory diagnostic sink
                        ← Reuses logging export-redaction helpers
```

The `logging.Logger` interface remains startup-pipeline-specific. Its export
redaction helpers (`SanitizeExportText`, `SanitizeExportPath`,
`SanitizeExportURL`, `SanitizeExportMetaValue`, `SanitizeExportDemoPath`) are
shared by startup and produce diagnostics. Other low-level packages
(`release`, `config`, `download`) return errors normally; they do not log
independently.

---

## Log Levels

| Level | String | When to Use |
|-------|--------|-------------|
| `info` | `"info"` | Normal operation: step started, step completed, config loaded, version checked |
| `warning` | `"warning"` | Recoverable issues: fallback to local version, validation skipped item, retry ignored |
| `error` | `"error"` | Failures: download failed, extraction failed, config parse failed |

Normalization via `normalizeLogLevel()`:
```go
"warn", "warning" → LevelWarn
"err", "error"    → LevelError
anything else     → LevelInfo
```

---

## Structured Fields

Every log entry carries these fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `component` | string | Yes | Which component is logging (e.g., `"hlae"`, `"plugin"`, `"startup"`, `"source"`) |
| `stage` | string | Yes | Lifecycle stage (e.g., `"check"`, `"download"`, `"extract"`, `"ready"`) |
| `action` | string | Yes | Specific action (e.g., `"validate_local"`, `"download_asset"`, `"skip_install"`) |
| `source` | string | Conditional | Download source context (`"github"` / `"custom"`) |
| `attempt` | int | Conditional | Retry attempt number |
| `elapsed_ms` | int64 | Conditional | Duration of the step in milliseconds (set by `StepDone`/`StepFail`) |
| `error` | string | Conditional | Error message on failure |
| `meta` | map[string]string | Conditional | Extra key-value pairs (URLs, paths, version numbers) |

### Example log entry structure

```go
s.emitLogWithFields("info", "HLAE 安装完成", logFields{
    Component: componentHLAE,   // "hlae"
    Stage:     "ready",
    Action:    "component_ready",
    Source:    "github",
    Meta: map[string]string{
        "version": installedVersion,
        "path":    cfg.HLAEExe,
    },
})
```

### Example warning log

```go
s.emitLogWithFields("warning", "广告条目已忽略: "+reason, logFields{
    Component: "ads",
    Stage:     "validate",
    Action:    "skip_item",
    Error:     reason,
})
```

---

## Step Timing Pattern

The most common logging pattern is **step timing** — recording when an operation starts, and logging duration on success/failure:

### StepStart

```go
started := s.logStepStart(componentID, "check", "validate_local", source, 0, map[string]string{
    "local_exe": cfg.HLAEExe,
})
```

### StepDone (success)

```go
s.logStepDone(componentID, "check", "validate_local", source, 0, started, map[string]string{
    "local_ready": strconv.FormatBool(localReady),
    "local_ver":   localVersion,
})
```

### StepFail (failure)

```go
s.logStepFail(componentID, "extract", "unzip", source, 0, started, err, nil)
```

The step functions:
- Auto-set `elapsed_ms` from `started.StartedAt` vs current time
- Fill missing component/stage/action/source from fallback args if `started` token is empty
- Log through `logging.Logger` interface automatically

---

## Log Buffer

Startup logs are kept in a ring buffer (`envsetup.Service.logs`) with a maximum of **12,000 entries**:

```go
const startupLogBufferLimit = 12000

if len(s.logs) > startupLogBufferLimit {
    s.logs = append([]LogMessage(nil), s.logs[len(s.logs)-startupLogBufferLimit:]...)
}
```

All log entries are also emitted as Wails runtime events (`"log"`) so the frontend can display them in real-time.

---

## The Two Audit Trails

1. **Internal buffer** (`Service.logs`) — consumed by `ExportStartupLogs()` for clipboard/file export
2. **Wails events** (`"log"`) — consumed by the frontend startup wizard for real-time display

Both contain the same data; neither is persisted to disk during normal operation.
`producews` is a separate support-diagnostics exception: its bounded rotating
log is persisted under `<dataDir>/logs` and must not reuse startup's Wails log
event or in-memory buffer.

---

## What Not to Log

- ❌ **Do not log sensitive values** — the `slog` adapter auto-sanitizes tokens, auth headers, and home directory paths
- ❌ **Do not use `fmt.Sprintf` for message construction** — put dynamic values in fields, not in the message string
- ❌ **Do not log in low-level packages** (`download`, `endpoints`, `config`) — return errors instead; only `envsetup` and its bridge log
- ❌ **Do not use the global `log` package** or `fmt.Println` — all startup logging goes through `envsetup.emitLogWithFields`
- ❌ **Do not log the same error twice** — log where it's handled, not where it originates

## Scenario: Shared support-export redaction

### 1. Scope / Trigger

- Trigger: adding a startup/produce support export, incident report, or a new
  dynamic diagnostic field.
- Scope: `internal/logging` sanitizer helpers, `ExportStartupLogs`,
  `producews.Diagnostics`, and plugin-log tail ingestion.

### 2. Signatures

```go
func SanitizeExportText(value string) string
func SanitizeExportPath(value string) string
func SanitizeExportURL(rawURL string) string
func SanitizeExportMetaValue(key, value string) string
func SanitizeExportDemoPath(value string) string
```

### 3. Contracts

- Every persisted/exported support artifact runs its final content through
  `SanitizeExportText`; structured diagnostic metadata uses
  `SanitizeExportMetaValue` first.
- URL credentials, sensitive query values, authorization/token text, and home
  prefixes are redacted. Any demo path is replaced with `<redacted-demo>`,
  including queue/take snapshots and plugin log tail.
- Sanitization is pure and shared. Do not duplicate regular expressions or
  path masking in `envsetup` and `producews`.

### 4. Validation & Error Matrix

| Input | Exported result |
| --- | --- |
| `token`, auth, password field | `***` |
| URL with credential/unknown query | credentials removed; safe query subset retained |
| Current-user path | home prefix becomes `~` |
| Absolute or structured demo path | `<redacted-demo>` |
| Empty/invalid URL | sanitized text, no sanitizer error |

### 5. Good / Base / Bad Cases

- Good: a plugin log says `playdemo D:\captures\private.dem`; exported text
  contains only `<redacted-demo>`.
- Base: a release URL retains version/source metadata after query filtering.
- Bad: call `os.UserHomeDir` plus a local regex inside a new export path; its
  behavior will drift from existing incident exports.

### 6. Tests Required

- `internal/logging`: credentials, URL filtering, Unix and Windows demo paths.
- `internal/envsetup`: existing startup export redaction regression tests.
- `internal/producews`: queue/take/plugin-tail report contains no source demo
  path and has the redaction marker.

### 7. Wrong vs Correct

#### Wrong

```go
report.WriteString(strings.ReplaceAll(path, home, "~"))
```

This misses demo paths outside the home directory and omits credentials/URLs.

#### Correct

```go
event.Meta["demo_path"] = logging.SanitizeExportMetaValue("demo_path", path)
report.WriteString(logging.SanitizeExportText(content))
```
