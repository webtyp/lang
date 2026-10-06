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
	// Parse and validate every dictionary first: a failing call loads nothing.
	files := make([]langFile, len(dicts))
	codes := make([][]lang, len(dicts))
	for i, raw := range dicts {
		if err := json.Unmarshal(raw, &files[i]); err != nil {
			return stdlibfmt.Errorf("lang: dictionary %d: %w", i, err)
		}
		codes[i] = make([]lang, len(files[i].Languages))
		for j, code := range files[i].Languages {
			l, ok := mapLangCode(code)
			if !ok {
				return stdlibfmt.Errorf("lang: dictionary %d: unknown language code %q", i, code)
			}
			codes[i][j] = l
		}
		for k, v := range files[i].Keys {
			if len(v) > len(files[i].Languages) {
				return stdlibfmt.Errorf("lang: dictionary %d: key %q has more values than languages", i, k)
			}
		}
	}
	for i, f := range files {
		for _, l := range codes[i] {
			if l != EN && !containsLang(pageLangs, l) {
				pageLangs = append(pageLangs, l)
			}
		}
		for key, values := range f.Keys {
			idx := -1
			for j := range dictEntries {
				if compareCaseInsensitive(dictEntries[j].translations[EN], key) == 0 {
					idx = j
					break
				}
			}
			if idx < 0 {
				var e entry
				e.translations[EN] = key
				dictEntries = append(dictEntries, e)
				idx = len(dictEntries) - 1
			}
			for k, v := range values {
				l := codes[i][k]
				// first non-empty value wins; EN holds the key itself
				if l != EN && v != "" && dictEntries[idx].translations[l] == "" {
					dictEntries[idx].translations[l] = v
				}
			}
		}
		if f.Default != "" && !pageSet {
			if l, ok := mapLangCode(f.Default); ok {
				pageDef = l
			}
		}
	}
	if len(pageLangs) > 0 {
		pageSet = true
	}
	sortDict()
	return nil
}

func containsLang(list []lang, l lang) bool {
	for _, x := range list {
		if x == l {
			return true
		}
	}
	return false
}
