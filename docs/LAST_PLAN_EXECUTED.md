---
PLAN: "feat: bundle translations as external JSON and read preloaded dictionary in WASM"
EXECUTOR: jules
REVIEWER: none
---

> Read AGENTS.md at the repo root before any change.
> **Finish by yourself:** when the delivery check passes, commit, push and open the pull request. Never ask whether to proceed or finalize — nobody answers, and a session that waits ends without your work.

# Plan: `lang` con diccionario en archivo externo (sin `<script>` en línea)

Master: `SECURITY_OBSERVABILITY_MASTER_PLAN.md` (D3, O3b-3).

## Tools

```bash
export GOPRIVATE=webtyp.com/*,github.com/veltylabs/*
export GOFLAGS=-mod=mod
go install webtyp.com/devflow/cmd/gotest@latest
export PATH="$PATH:$(go env GOPATH)/bin"
```

## Ecosystem search

- Buscado en `~/Dev/Project/webtyp` (`lang`, `sitec`, `js`).
- Hoy `lang/langc/langc.go:659` (`BundleTranslations`) envuelve el JSON en `<script type="application/json" id="webtyp-lang">...</script>` y `lang/load.front.go:29` lee ese elemento del DOM.
- **Decisión:** cambiar `lang` para emitir JSON puro (que `sitec` escribirá como `lang.<hash>.json`) y leer el objeto precargado en `globalThis.__webtypLang` (cargado en paralelo con `browser.wasm` por `webtyp/js`).

## Design gate

1. **Prior art:**
   - [i18next http-backend](https://github.com/i18next/i18next-http-backend): sirve los diccionarios como archivos JSON estáticos cacheables.
   - [MDN — CSP `script-src`](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Content-Security-Policy/script-src): eliminar cualquier `<script>` en línea deja el HTML completamente libre de bloques incrustados.
   - [FormatJS / React Intl](https://formatjs.io/docs/getting-started/message-distribution/): entrega catálogos compilados como archivos JSON con hash en el nombre.
2. **Novice-name test:**
   - `lang.MetaName = "webtyp-lang"` (nombre del `<meta name="webtyp-lang" content="/lang.<hash>.json">`).
   - `lang.GlobalDictKey = "__webtypLang"` (propiedad de `globalThis` donde `js.PageBootstrap` deja el JSON ya parseado antes de `go.run`).
   - `langc.Translations.BundleTranslations(rootDir string) ([]byte, error)` devuelve el JSON puro (nil/vacío si no hay diccionario).
3. **Complexity ledger:**
   - Conceptos a aprender: `0` (reemplaza `ScriptID` por `MetaName` + `GlobalDictKey`)
   - Archivos a tocar: `0`
   - Líneas en el call site: `0`
   - Formas de hacer lo mismo: `0` (`ScriptID` y el `<script>` incrustado se eliminan en el mismo cambio).
4. **Where it belongs:** `webtyp.com/lang` es el dueño del formato del diccionario y su carga en WASM; `sitec` solo emite el archivo con hash y el `<meta>`, y `js` solo precarga la URL del `<meta>`. Ninguno conoce las entrañas del otro.
5. **What does this change delete?** Borra `lang.ScriptID` (`language.go:7`), la envoltura `<script type="application/json" id="webtyp-lang">` en `langc/langc.go:703`, y la lectura `doc.Call("getElementById", ScriptID)` en `load.front.go:29`.

## APIs used

Ninguna dependencia externa nueva; solo tipos internos de `webtyp.com/lang`.

## Reglas de entrega y anti-footguns

- `load.front.go` compila a WASM con TinyGo: **no usar `map` ni stdlib `strings`/`fmt`**.
- `lang/langc` es tooling de servidor (`!wasm`): stdlib es legítima allí.
- Hacer commit después de cada etapa. No usar `git reset`, `git restore`, `git checkout -- <file>` ni `git stash`.

---

## Etapa 1: Diagrama y pruebas (TDD)

1. Actualizar `docs/diagrams/project_dictionaries.md` mostrando que `BundleTranslations` entrega JSON puro para un archivo externo `lang.<hash>.json` (sin `<script>` en HTML) y que el navegador lo precarga en `globalThis.__webtypLang`.
2. Actualizar pruebas existentes:
   - `tests/langc_test.go:250` y `tests/langc_main_library_test.go`: `BundleTranslations(app)` devuelve `([]byte, error)` con JSON válido (`{"default":...,"languages":[...],"keys":{...}}`) y SIN `<script`.
   - `tests/page_dictionary_test.go`: inyectar el objeto parseado en `js.Global().Set(lang.GlobalDictKey, ...)` en lugar de crear un `<script id="webtyp-lang">`.

## Etapa 2: Constantes y `langc.BundleTranslations`

1. En `language.go`:
   - Eliminar `const ScriptID = "webtyp-lang"`.
   - Agregar:
     ```go
     // MetaName is the <meta name="..."> emitted by sitec pointing to the external dictionary JSON file.
     const MetaName = "webtyp-lang"

     // GlobalDictKey is the globalThis property where the page bootstrap stores the parsed dictionary JSON
     // before starting the WASM binary.
     const GlobalDictKey = "__webtypLang"
     ```
2. En `langc/langc.go:659`:
   - Cambiar firma a `func (t *Translations) BundleTranslations(rootDir string) ([]byte, error)`.
   - Cuando `!st.exists`: `return nil, nil`.
   - Al final (`:703`): `return payload, nil` (JSON puro, sin `<script>`).

## Etapa 3: Carga en WASM (`load.front.go`)

En `load.front.go`:
1. Reemplazar la lectura de `document.getElementById(ScriptID)` + `JSON.parse` por:
   ```go
   parsed := js.Global().Get(GlobalDictKey)
   if parsed.IsUndefined() || parsed.IsNull() {
       return
   }
   loaded = true
   ```
2. Mantener exactamente igual la extracción de `default`, `languages` y `keys` desde `parsed` y la llamada a `sortDict()`.

## Etapa 4: Verificación y documentación

1. Verificar con grep que `ScriptID` y `webtyp-lang` como `<script>` no existen:
   `grep -rn 'ScriptID\|<script' . --include='*.go'` → vacío.
2. Actualizar `AGENTS.md`, `README.md` (≤ 150 líneas, 4 secciones) y `docs/ARCHITECTURE.md` (4 secciones).
3. Ejecutar `gotest`.

## Tabla de etapas

| Etapa | Archivos | Qué |
|---|---|---|
| 1 | `docs/diagrams/project_dictionaries.md`, `tests/langc_test.go`, `tests/langc_main_library_test.go`, `tests/page_dictionary_test.go` | Diagrama y pruebas TDD |
| 2 | `language.go`, `langc/langc.go` | `MetaName`, `GlobalDictKey` y `BundleTranslations` → `([]byte, error)` |
| 3 | `load.front.go` | Lectura síncrona desde `globalThis.__webtypLang` |
| 4 | `AGENTS.md`, `README.md`, `docs/ARCHITECTURE.md` | Documentación y `gotest` verde |
