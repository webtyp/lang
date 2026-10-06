package langc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const (
	// projectDir marks a project: its dictionary lives in <root>/config/lang.json.
	projectDir = "config"
	// fileName is the dictionary file of a project (in config/) and of a library (at its root).
	fileName = "lang.json"
	// defaultLanguage is the language a new dictionary file starts with.
	defaultLanguage = "es"
)

// knownCodes are the language codes webtyp.com/lang can show.
var knownCodes = []string{"en", "es", "zh", "hi", "ar", "pt", "fr", "de", "ru"}

// langFile is the on-disk shape. Keys hold positional lists: the value at
// index i belongs to Languages[i].
type langFile struct {
	Default   string              `json:"default,omitempty"`
	Languages []string            `json:"languages"`
	Keys      map[string][]string `json:"keys"`
}

// dictPath returns the dictionary file of the module at rootDir and whether it
// is a project (has config/) or a library.
func dictPath(rootDir string) (path string, isProject bool) {
	if info, err := os.Stat(filepath.Join(rootDir, projectDir)); err == nil && info.IsDir() {
		return filepath.Join(rootDir, projectDir, fileName), true
	}
	return filepath.Join(rootDir, fileName), false
}

// readDict reads a dictionary file. A missing file yields the default shape
// (with a default language only for a project) and exists=false.
func readDict(path string, isProject bool) (f langFile, raw []byte, exists bool, err error) {
	raw, err = os.ReadFile(path)
	if os.IsNotExist(err) {
		f = langFile{Languages: []string{defaultLanguage}, Keys: map[string][]string{}}
		if isProject {
			f.Default = defaultLanguage
		}
		return f, nil, false, nil
	}
	if err != nil {
		return f, nil, false, err
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return f, raw, true, fmt.Errorf("lang: %s: %w", path, err)
	}
	if f.Keys == nil {
		f.Keys = map[string][]string{}
	}
	return f, raw, true, nil
}

// validate checks the positional integrity of a dictionary: known, unique
// codes, and no list longer than languages (a removed or reordered language
// whose values were not moved — never guessed).
func validate(path string, f langFile) error {
	seen := map[string]bool{}
	for _, code := range f.Languages {
		if !isKnownCode(code) {
			return fmt.Errorf("lang: %s: unknown language code %q", path, code)
		}
		if seen[code] {
			return fmt.Errorf("lang: %s: duplicate language code %q", path, code)
		}
		seen[code] = true
	}
	var bad []string
	for k, v := range f.Keys {
		if len(v) > len(f.Languages) {
			bad = append(bad, k)
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return fmt.Errorf("lang: %s: %d keys have more values than languages (e.g. %q) — fix languages or the lists", path, len(bad), bad[0])
	}
	return nil
}

func isKnownCode(code string) bool {
	for _, c := range knownCodes {
		if c == code {
			return true
		}
	}
	return false
}

// pad returns v with "" appended until it has n values.
func pad(v []string, n int) []string {
	out := append([]string{}, v...)
	for len(out) < n {
		out = append(out, "")
	}
	return out
}

// valueFor returns f's value of key for language code, "" when absent.
func valueFor(f langFile, key, code string) string {
	v := f.Keys[key]
	for i, c := range f.Languages {
		if c == code && i < len(v) {
			return v[i]
		}
	}
	return ""
}

// format writes the file with one key per line, keys sorted by byte order.
func format(f langFile) []byte {
	var b bytes.Buffer
	b.WriteString("{\n")
	if f.Default != "" {
		d, _ := json.Marshal(f.Default)
		fmt.Fprintf(&b, "  \"default\": %s,\n", d)
	}
	fmt.Fprintf(&b, "  \"languages\": %s,\n", list(f.Languages))
	b.WriteString("  \"keys\": {")
	keys := sortedKeys(f.Keys)
	for i, k := range keys {
		kj, _ := json.Marshal(k)
		sep := ","
		if i == len(keys)-1 {
			sep = ""
		}
		fmt.Fprintf(&b, "\n    %s: %s%s", kj, list(f.Keys[k]), sep)
	}
	if len(keys) > 0 {
		b.WriteString("\n  ")
	}
	b.WriteString("}\n}\n")
	return b.Bytes()
}

// list marshals a string list with a space after each comma: ["a", "b"].
func list(v []string) []byte {
	if v == nil {
		v = []string{}
	}
	parts := make([][]byte, len(v))
	for i, s := range v {
		parts[i], _ = json.Marshal(s)
	}
	return append(append([]byte("["), bytes.Join(parts, []byte(", "))...), ']')
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
