//go:build wasm

package lang

import (
	"syscall/js"
)

var (
	loaded    bool
	pageLangs []lang
	pageDef   lang = EN
	pageSet   bool
)

func loadFromPage() {
	if loaded {
		return
	}

	doc := js.Global().Get("document")
	if doc.IsUndefined() {
		return
	}

	// Not marked loaded until the element exists: a lookup made before the
	// page carries the dictionary (a package init, a test's TestMain that
	// inserts it later) must not disable translations for good.
	script := doc.Call("getElementById", ScriptID)
	if script.IsNull() || script.IsUndefined() {
		return
	}
	loaded = true

	jsonStr := script.Get("textContent").String()
	if jsonStr == "" {
		return
	}

	json := js.Global().Get("JSON")
	parsed := json.Call("parse", jsonStr)

	// Extract default lang
	def := parsed.Get("default")
	if !def.IsUndefined() && !def.IsNull() {
		pageDef = langParser(def.String())
		pageSet = true
	}

	// Extract languages array
	langs := parsed.Get("languages")
	if langs.IsUndefined() || langs.IsNull() {
		return
	}

	lenLangs := langs.Length()
	// codeToLang[i] is the language of position i; known[i] is false for an
	// unknown code, whose position is skipped (never written to EN, which
	// holds the key itself).
	codeToLang := make([]lang, lenLangs)
	known := make([]bool, lenLangs)
	pageLangs = make([]lang, 0, lenLangs)
	for i := 0; i < lenLangs; i++ {
		code := langs.Index(i).String()
		if l, ok := mapLangCode(code); ok {
			codeToLang[i] = l
			known[i] = true
			pageLangs = append(pageLangs, l)
		}
	}

	// Parse keys
	keys := parsed.Get("keys")
	if keys.IsUndefined() || keys.IsNull() {
		return
	}

	jsKeys := js.Global().Get("Object").Call("keys", keys)
	numKeys := jsKeys.Length()
	dictEntries = make([]entry, 0, numKeys)

	for i := 0; i < numKeys; i++ {
		key := jsKeys.Index(i).String()
		values := keys.Get(key)

		var e entry
		e.translations[EN] = key // The key is its EN representation

		valLen := values.Length()
		for j := 0; j < valLen && j < lenLangs; j++ {
			if !known[j] || codeToLang[j] == EN {
				continue
			}
			e.translations[codeToLang[j]] = values.Index(j).String()
		}
		dictEntries = append(dictEntries, e)
	}

	sortDict()
}
