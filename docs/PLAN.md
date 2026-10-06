---
PLAN: "feat: framework-layer translations — Load (backend), Current, langc tool mode; client rule 2 limited to the project"
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 12385598363423994912
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan — lang: the framework's own tools translate too

Phase **T6** of the master plan `SOURCE_SELECTION_MASTER_PLAN.md` (orchestration only — everything
this plan needs is inline). Builds on lang v0.1.1 (page dictionary, `langc`).

Read [AGENTS.md](../AGENTS.md) first. The root package compiles to WASM, so `encoding/json` and `os`
are allowed only in `*.back.go` (`//go:build !wasm`) files. `langc/` is backend tooling. Tests live
in `tests/` and use only the public API. **Never export a symbol so a test can reach it.**

## Why

webtyp is a framework for people with little development experience, so **two layers** need
translations:

1. **What the user builds (the app).** Already done: `config/lang.json` → inlined in `index.html` →
   only the browser translates.
2. **The framework itself, for the developer.** The TUI of `webtyp/app`, `devtui`, `devbrowser`,
   `server` and `sitec` log messages through `lang.Translate` (about 15 files), and devtui has a
   language selector. But no dictionary exists for this layer, so it is always English. Under the
   current design ("the backend never has a dictionary") it never could translate.

The maintainer decided (2026-10-06):
- each framework tool ships its own `lang.json`, **embedded** in its binary with `go:embed`;
- `webtyp/app` passes them to `lang` at startup through a backend loader. This works with the
  prebuilt binary of `app-releases`, and the tool's translations ship with each tool release;
- the TUI starts in the system language (`LANG`), and a language picked in devtui's selector is
  saved and respected next time;
- `langc` gets a **tool mode** that scans the backend build (where tool messages live, including
  `fmt.Err`), so these files are generated like an app's;
- key rule 2 (`fmt.Err`) in **project** mode is limited to the project's own module. An end-to-end
  run on mjosefa-cms listed internal errors of `orm`, `json`, `jwt`… that a person never reads.

Also a bug: devtui reads the current language with `lang.OutLang()`, but `OutLang()` with no
arguments **re-detects and sets** the language, so it would erase a saved choice.

## Design gate

1. **Prior art.** Go's `golang.org/x/text/message` catalogs are built in code and selected per
   process; GNU gettext tools ship `.mo` catalogs with each program and pick `LANG`/`LC_MESSAGES`;
   VS Code ships `nls` bundles inside each extension and stores the chosen display language in
   user settings. We follow gettext and VS Code: each tool carries its catalog, the system locale
   is the default, and an explicit choice is persisted.
2. **Novice-name test.**
   - `lang.Load(dicts ...[]byte) error`: "load these dictionaries".
   - `lang.Current() string`: "the current language".
   - `langc` `SyncToolTranslations(rootDir)` and `langc sync -tool`: "sync a tool's translations".
3. **Complexity ledger.**
   ```
   Concepts the developer must learn   +2 (Load for tools, Current) — app developers never call Load
   Files they must touch to do X       +0 for app developers; tool authors: one lang.json + one embed file
   Lines at the call site              +1 in webtyp/app's startup
   Ways to do the same thing           −1 (reading the language no longer goes through OutLang())
   ```
4. **Where it belongs.** Loading and lookup are `lang`'s job. Each tool owns its dictionary. The
   composition root (`webtyp/app`) decides which dictionaries are loaded and stores the chosen
   language.
5. **What it deletes.** The read-through-`OutLang()` misuse in devtui (phase E2), and rule-2 keys
   from libraries in project mode.

## Stage 1 — `lang.Current()` (`language.go`)

```go
// Current returns the active output language code ("EN", "ES", …) without
// changing it. Use it to read the language; OutLang() with no arguments
// re-detects and sets it.
func Current() string { return getCurrentLang().String() }
```

Test (`tests/current_test.go`, backend): `lang.OutLang("es")`, then `lang.Current() == "ES"` twice
in a row, and the language does not change.

## Stage 2 — `lang.Load` for backend tools (`load.back.go`, `//go:build !wasm`)

```go
// Load installs translation dictionaries for a backend tool (the framework's
// TUI and logs). Each argument is one lang.json (shape: {"languages": [...],
// "keys": {"key": ["v1", ...]}}, values positional in the order of
// "languages"). Dictionaries merge in call order; for a key and a language the
// first non-empty value wins. Apps never call it: in the browser the
// dictionary comes from the page.
func Load(dicts ...[]byte) error
```

- Parse with `encoding/json` (allowed: backend-only file).
- Validate each dictionary like `langc` does: known codes and no list longer than `languages`. On
  error return `lang: dictionary <index>: <reason>` and load nothing from that call.
- Fill `dictEntries` exactly as `loadFromPage` does in WASM (an unknown code's position is skipped,
  EN holds the key), then `sortDict()`. Calling `Load` again merges into what is already loaded.
- After `Load`, `OutLang()` with no args resolves: the system language (`getSystemLang`, which reads
  `LANG`/`LANGUAGE`/`LC_ALL`/`LC_MESSAGES`) if some loaded dictionary has it, else EN.
- **WASM has no `Load`.** The function lives only in `load.back.go`, so an app calling it does not
  compile; that is intended.

Tests (`tests/load_test.go`, `//go:build !wasm`):
- load two dictionaries with an overlapping key: the first non-empty value wins, and an empty slot
  falls back to the second;
- `Translate` under `OutLang("es")` translates; an unknown code is skipped and does not overwrite
  EN;
- a list longer than `languages` → error, and nothing from that call is loaded;
- `t.Setenv("LANG", "es_CL.UTF-8")` with an `es` dictionary loaded → `OutLang()` returns `"ES"`.

## Stage 3 — `langc` tool mode

```go
// SyncToolTranslations updates <rootDir>/lang.json for a backend tool: it scans
// the module's BACKEND build (not js/wasm) and applies every rule, including
// fmt.Err everywhere in the module. Same file shape and merge rules as a library.
func (t *Translations) SyncToolTranslations(rootDir string) error
```

- Build context for the scan: `build.Default` (the host), instead of the js/wasm `clientBuild`.
  Make the context a parameter of the internal parse function; there are two entry points and no
  boolean flag in the public API.
- The file is `<rootDir>/lang.json` (library shape, no `default`). Existing keys are never removed
  (library rule).
- CLI: `langc sync -tool [dir]` calls it. `langc sync [dir]` keeps calling `SyncTranslations`.
  Usage text lists both.

## Stage 4 — client rule 2 limited to the project

In `SyncTranslations` / `MissingTranslations` **project** mode, rule 2 (`fmt.Err` literals)
applies only to files of the **main** module. Rules 1 and 3–8 still apply to every scanned module.
Library mode (client) and tool mode keep rule 2 for their own module.

Implementation: `collectKeys` receives which modules rule 2 applies to (a set of module paths);
every other rule is unchanged.

## Stage 5 — tests for langc (`tests/langc_test.go`, backend)

- Project mode: a library in the fixture with `fmt.Err("internal failure")` → **not** a key; the
  project's own `fmt.Err("name", "required")` → keys `name` and `required`.
- Tool mode: a fixture module with a `//go:build !wasm` file calling
  `lang.Translate("Server", "started")` and `fmt.Err("port", "busy")` → `SyncToolTranslations`
  writes `lang.json` with `Server`, `started`, `port` and `busy`. The client `SyncTranslations` on
  the same module (library mode) does **not** see them (the file is excluded from js/wasm).
- CLI: no test (thin `main`).

## Stage 6 — docs

`README.md`: a section "Two layers": the app (page dictionary, `config/lang.json`) and the
framework's tools (`lang.json` embedded per tool, `lang.Load` at startup, `langc sync -tool`), with
`lang.Current()` for reading the language.

## Acceptance

- `gotest` passes (includes WASM).
- `GOOS=js GOARCH=wasm go list -deps . | grep -E '^encoding/json$'` → empty (Load stays out of WASM).
- `grep -n "func Load" *.go` → only in `load.back.go`.

## Stages

| # | Stage | Files |
|---|---|---|
| 1 | `Current` | `language.go`, `tests/current_test.go` |
| 2 | `Load` | `load.back.go`, `tests/load_test.go` |
| 3 | Tool mode | `langc/langc.go`, `langc/scan.go`, `cmd/langc/main.go` |
| 4 | Rule 2 scope | `langc/langc.go`, `langc/scan.go` |
| 5 | Tests | `tests/langc_test.go` |
| 6 | Docs | `README.md` |
