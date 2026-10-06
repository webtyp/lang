//go:build !wasm

package lang

import (
	"encoding/json"
	stdlibfmt "fmt"
)

var pageSet bool
var pageDef lang = EN
var pageLangs []lang

func loadFromPage() {}

// langFile defines the shape of a lang.json file.
type langFile struct {
	Default   string              `json:"default,omitempty"`
	Languages []string            `json:"languages"`
	Keys      map[string][]string `json:"keys"`
}

// Load installs translation dictionaries for a backend tool (the framework's
// TUI and logs). Each argument is one lang.json (shape: {"languages": [...],
// "keys": {"key": ["v1", ...]}}, values positional in the order of
// "languages"). Dictionaries merge in call order; for a key and a language the
// first non-empty value wins. Apps never call it: in the browser the
// dictionary comes from the page.
func Load(dicts ...[]byte) error {
	for i, dictBytes := range dicts {
		var f langFile
		if err := json.Unmarshal(dictBytes, &f); err != nil {
			return stdlibfmt.Errorf("lang: dictionary %d: %w", i, err)
		}

		codeToLang := make([]lang, len(f.Languages))
		known := make([]bool, len(f.Languages))

		for j, code := range f.Languages {
			if l, ok := mapLangCode(code); ok {
				codeToLang[j] = l
				known[j] = true

				// add to pageLangs if not already there
				found := false
				for _, pl := range pageLangs {
					if pl == l {
						found = true
						break
					}
				}
				if !found {
					pageLangs = append(pageLangs, l)
				}
			} else {
				return stdlibfmt.Errorf("lang: dictionary %d: unknown language code %q", i, code)
			}
		}

		for k, v := range f.Keys {
			if len(v) > len(f.Languages) {
				return stdlibfmt.Errorf("lang: dictionary %d: key %q has more values than languages", i, k)
			}
		}

		// Fill dictEntries. Since it merges into what is already loaded, we update existing entries or append new ones.
		for key, values := range f.Keys {
			found := false
			for j, entry := range dictEntries {
				if entry.translations[EN] == key {
					found = true
					for k := 0; k < len(values) && k < len(f.Languages); k++ {
						if !known[k] || codeToLang[k] == EN {
							continue
						}
						// first non-empty value wins (or if currently empty, overwrite)
						if dictEntries[j].translations[codeToLang[k]] == "" && values[k] != "" {
							dictEntries[j].translations[codeToLang[k]] = values[k]
						}
					}
					break
				}
			}

			if !found {
				var e entry
				e.translations[EN] = key
				for k := 0; k < len(values) && k < len(f.Languages); k++ {
					if !known[k] || codeToLang[k] == EN {
						continue
					}
					e.translations[codeToLang[k]] = values[k]
				}
				dictEntries = append(dictEntries, e)
			}
		}

		if f.Default != "" && !pageSet {
			if l, ok := mapLangCode(f.Default); ok {
				pageDef = l
				pageSet = true
			}
		}
	}

	// Ensure pageSet is true if pageLangs has something
	if len(pageLangs) > 0 {
		pageSet = true
	}

	sortDict()
	return nil
}
