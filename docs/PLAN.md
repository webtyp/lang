---
PLAN: "fix(langc): a library that ships its own demo reads its own root lang.json"
EXECUTOR: jules
REVIEWER: none
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

> **Phase A3** of
> [`veltylabs/mjosefa-cms/docs/RESERVA_Y_PAGOS_MASTER_PLAN.md`](https://github.com/veltylabs/mjosefa-cms/blob/main/docs/RESERVA_Y_PAGOS_MASTER_PLAN.md).
> Unblocks the Spanish demo of `veltylabs/appointment_booking` (Phase B).

# Plan — `langc`: the main module's root `lang.json` counts as a library dictionary

## 0. The defect (verified 2026-10-12)

`veltylabs/appointment_booking` is a library with a runnable demo: it has `config/` (so
`langc` treats it as a **project**) and a root `lang.json` that translates its own texts
(`"Confirmed": ["Confirmada"]`, …). Running its demo, the dev log says
`lang: 177 untranslated (es) — see …/config/lang.json`, and the page shows `Confirmed`,
`Client Id`, `Day` in English.

Cause, `langc/langc.go`, `(*Translations).load`:

```go
if st.isProject {
	for _, m := range mods {
		if m.IsMain || m.SourceDir() == "" {
			continue // ← skips the main module, so its root lang.json is never read
		}
		…
```

For a plain app this is right (an app has no root `lang.json`). For a library with a demo it
drops the library's own dictionary: `SyncTranslations` copies every one of its keys into
`config/lang.json` as untranslated, and `BundleTranslations` ships them untranslated. The
same library translates fine inside another app (there it is a dependency, not main).

No exported symbol changes: the api-design gate does not apply.

## 1. Rules

- Tests only in `tests/` (external package). `gotest` (prerequisite:
  `go install webtyp.com/devflow/cmd/gotest@latest`), never `go test`.
- `langc` is backend tooling (`!wasm`) and already uses stdlib (`os`, `path/filepath`,
  `encoding/json`, `strings`): **do not** "fix" those imports.

## 2. Method — failing test first, 100% of the changed function

1. Write the tests of §3 → `gotest` → **confirm they fail** → commit
   `test(langc): main module lang.json ignored (red)`.
2. Fix (§4) → green → commit `fix(langc): …`.
3. Coverage: `load` at 100.0% in
   `go test -coverpkg=webtyp.com/lang/langc -coverprofile=/tmp/c.out ./tests/... && go tool cover -func=/tmp/c.out | grep load`.

Diagram: [`docs/diagrams/project_dictionaries.md`](diagrams/project_dictionaries.md).

## 3. Tests — diagram → test (`tests/langc_main_library_test.go`)

Reuse the helpers of `tests/langc_test.go` (`fakeModules`, `write`, `readDict`). Fixture: a
module `example.com/demo` with `config/`, a root `lang.json`
`{"languages":["es"],"keys":{"Confirmed":["Confirmada"]}}`, and a Go file using
`lang.Text("Confirmed")`; `fakeModules` returns it with `IsMain: true`.

| Branch | Test |
|---|---|
| `P2-yes`, sync | `TestSync_MainModuleLibraryKeysAreNotCopiedToProject` — after `SyncTranslations`, `config/lang.json` has no `Confirmed` key |
| `P2-yes`, missing | `TestMissing_MainModuleLibraryCountsAsTranslated` — `MissingTranslations` does not list `Confirmed` |
| `P2-yes`, bundle | `TestBundle_UsesMainModuleLibraryValue` — the bundle contains `"Confirmed":["Confirmada"]` |
| precedence | `TestBundle_ProjectValueWinsOverMainLibrary` — `config/lang.json` with `"Confirmed":["Confirmado"]` ⇒ bundle has `Confirmado` |
| `P2-no` (absent) | `TestSync_ProjectWithoutRootLangJSONUnchanged` — today's behaviour for a plain app |
| `P2-no` (broken) | `TestLoad_BrokenMainLibraryFileIsSkipped` — invalid JSON in root `lang.json` does not block, key reported untranslated |
| `P1-no` | existing library tests stay green (no change) |

## 4. Fix (`langc/langc.go`, `load`)

Inside `if st.isProject { … }`, **before** the dependency loop:

```go
// A library that ships its own demo is a project (it has config/) AND a
// library: its root lang.json translates its own texts and must count.
own := filepath.Join(rootDir, fileName)
if own != st.path {
	if d, _, ok, err := readDict(own, false); err == nil && ok && validate(own, d) == nil {
		st.libs = append(st.libs, library{module: modulePath(rootDir), dict: d})
	}
}
```

Keep the `m.IsMain` skip in the loop (the main module's file is now added once, first).

## 5. Acceptance criteria

- `gotest` green; the §2 coverage line shows `load` at 100.0%.
- Every test of §3 exists with its exact name.
- PR history: red `test(langc)` commit before the `fix(langc)` commit.
- Before opening the PR: `git diff --stat main` lists exactly `langc/langc.go`,
  `tests/langc_main_library_test.go`, `README.md` (one line under the dictionaries section:
  "a library with a demo also reads its own root `lang.json`"); nothing deleted.
