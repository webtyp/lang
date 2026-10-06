# AGENTS.md — webtyp/lang

Working notes for AI agents operating in this repository. End-user docs: [README.md](README.md).

## What this repo is

Translation for the webtyp ecosystem: the lookup engine for the dictionary the page carries, the
output language (`OutLang`, constants `EN`, `ES`, …), `Translate`, and the `langc` generator. Importing it installs the translator
hook of `webtyp.com/fmt` (`fmt.SetTranslator`), so `fmt.Err`, `fmt.Println` and `fmt.Sprintf("%L")`
translate too. It was split out of `webtyp.com/fmt` because it keeps growing: changing it must not
force a new `fmt` release on the whole ecosystem.

## Two packages, two rules

- `lang` (the root package) **compiles to WASM** — rules below.
- `lang/langc` and `cmd/langc` are **backend tooling** (the generator that keeps `config/lang.json`
  and library `lang.json` files in step with the code, and builds the dictionary sitec inlines in the
  HTML). There `os`, `go/ast`, `go/parser`, `encoding/json`, `map` and `path/filepath` are
  legitimate — do NOT "fix" them. The root package must never import `langc`.

## Where translations live

Translations are **data**, never Go: a project's `config/lang.json` (with `default` and `languages`)
and each library's `lang.json` at its module root. sitec inlines the merged dictionary in
`index.html` as `<script type="application/json" id="webtyp-lang">`; the root package reads it once in
the browser. A server never translates. There is no Go API to register words.

## The root package compiles to WASM

- Do NOT import `strings`, `strconv`, `errors` or stdlib `fmt`; use `webtyp.com/fmt`.
- `os`/`sync` only in `*.back.go` (`//go:build !wasm`); `syscall/js` only in `*.front.go`
  (`//go:build wasm`).
- No `map` (TinyGo hashmap runtime): the dictionary is a sorted slice with binary search — keep it so.

## The build that defines "done"

```bash
go install webtyp.com/devflow/cmd/gotest@latest   # once
gotest
```

## Rules

- Tests live in `tests/` as `package lang_test` (public API only). A root-level test is allowed only
  with a top-of-file justification of the unexported identifier it needs. **Never export a symbol so
  a test can reach it.**
- `fmt` never imports `lang`. The dependency goes one way: `lang` → `fmt`.
- Every repeated string is a named constant.
