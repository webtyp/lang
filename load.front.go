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
	loaded = true

	doc := js.Global().Get("document")
	if doc.IsUndefined() {
		return
	}

	script := doc.Call("getElementById", ScriptID)
	if script.IsNull() || script.IsUndefined() {
		return
	}

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
	var codeToLang = make([]lang, lenLangs)
	pageLangs = make([]lang, 0, lenLangs)
	for i := 0; i < lenLangs; i++ {
		code := langs.Index(i).String()
		if l, ok := mapLangCode(code); ok {
			codeToLang[i] = l
			pageLangs = append(pageLangs, l)
		} else {
			// fallback/unknown code is ignored but keeps position valid
			codeToLang[i] = EN
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
			val := values.Index(j).String()
			l := codeToLang[j]
			e.translations[l] = val
		}
		dictEntries = append(dictEntries, e)
	}

	sortDict()
}
