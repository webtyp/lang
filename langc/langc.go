// Package langc keeps translation dictionaries in step with the code and builds
// the dictionary the browser reads. It is backend tooling: the root package
// webtyp.com/lang never imports it.
package langc

import (
	"bytes"
	"encoding/json"
	"go/build"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	wpath "webtyp.com/filepath"
	"webtyp.com/lang"
	"webtyp.com/modfind"
)

// Translations keeps a project's config/lang.json (or a library's lang.json) in
// step with the texts its code uses, and builds the dictionary the client reads.
type Translations struct {
	modules modfind.Discoverer
	log     func(...any)
}

// New returns a Translations that discovers the project's modules through
// modules and reports what is untranslated through log.
func New(modules modfind.Discoverer, log func(...any)) *Translations {
	if log == nil {
		log = func(...any) {}
	}
	return &Translations{modules: modules, log: log}
}

// Missing is one untranslated key in one language, with the modules that use it.
type Missing struct {
	Key      string
	Language string
	Modules  []string
}

// library is a dependency's lang.json.
type library struct {
	module string
	dict   langFile
}

// state is everything one operation needs about the module at rootDir.
type state struct {
	path      string
	isProject bool
	dict      langFile
	raw       []byte
	exists    bool
	libs      []library
}

// load reads the dictionary at rootDir and, for a project, the lang.json of
// every library in its module graph (Discover order).
func (t *Translations) load(rootDir string) (state, []modfind.Module, error) {
	var st state
	st.path, st.isProject = dictPath(rootDir)
	var err error
	if st.dict, st.raw, st.exists, err = readDict(st.path, st.isProject); err != nil {
		return st, nil, err
	}
	if err := validate(st.path, st.dict); err != nil {
		return st, nil, err
	}
	mods, err := t.scope(rootDir, st.isProject)
	if err != nil {
		return st, nil, err
	}
	if st.isProject {
		for _, m := range mods {
			if m.IsMain || m.SourceDir() == "" {
				continue
			}
			p := filepath.Join(m.SourceDir(), fileName)
			d, _, ok, err := readDict(p, false)
			if err != nil || !ok || validate(p, d) != nil {
				continue // a broken library file never blocks the project
			}
			st.libs = append(st.libs, library{module: m.Path, dict: d})
		}
	}
	return st, mods, nil
}

// scope lists the modules to scan: for a project, the main module plus every
// module whose go.mod mentions webtyp.com/; for a library, only itself.
func (t *Translations) scope(rootDir string, isProject bool) ([]modfind.Module, error) {
	if !isProject {
		return []modfind.Module{{Path: modulePath(rootDir), Dir: rootDir, LocalDir: rootDir, IsMain: true}}, nil
	}
	all, err := t.modules.Discover(rootDir)
	if err != nil {
		return nil, err
	}
	var out []modfind.Module
	for _, m := range all {
		if m.IsMain {
			out = append(out, m)
			continue
		}
		gomod, err := os.ReadFile(filepath.Join(m.SourceDir(), "go.mod"))
		if err == nil && bytes.Contains(gomod, []byte("webtyp.com/")) {
			out = append(out, m)
		}
	}
	return out, nil
}

// modulePath reads the module path of the go.mod at dir.
func modulePath(dir string) string {
	gomod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return filepath.Base(dir)
	}
	for _, line := range strings.Split(string(gomod), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			p := strings.TrimSpace(strings.TrimPrefix(line, "module "))
			if i := strings.Index(p, "//"); i >= 0 {
				p = strings.TrimSpace(p[:i])
			}
			if u, err := strconv.Unquote(p); err == nil {
				p = u
			}
			return p
		}
	}
	return filepath.Base(dir)
}

func scan(mods []modfind.Module, ctx build.Context, isProject bool) keyUses {
	var files []*srcFile
	var rule2Scope map[string]bool

	if isProject {
		rule2Scope = map[string]bool{}
		for _, m := range mods {
			if m.IsMain {
				rule2Scope[m.Path] = true
			}
		}
	}

	for _, m := range mods {
		files = append(files, parseModule(m, ctx)...)
	}
	return collectKeys(files, collectTextFields(files), rule2Scope)
}

// libValue returns the first non-empty library value of key for code.
func (st state) libValue(key, code string) string {
	for _, l := range st.libs {
		if v := valueFor(l.dict, key, code); v != "" {
			return v
		}
	}
	return ""
}

// SyncToolTranslations updates <rootDir>/lang.json for a backend tool: it scans
// the module's BACKEND build (not js/wasm) and applies every rule, including
// fmt.Err everywhere in the module. Same file shape and merge rules as a library.
func (t *Translations) SyncToolTranslations(rootDir string) error {
	st, mods, err := t.load(rootDir)
	if err != nil {
		return err
	}

	uses := scan(mods, build.Default, false)

	supportedCodes := []string{}
	for _, c := range lang.Supported() {
		if c != "en" {
			supportedCodes = append(supportedCodes, c)
		}
	}

	for _, reqCode := range supportedCodes {
		found := false
		for _, exCode := range st.dict.Languages {
			if exCode == reqCode {
				found = true
				break
			}
		}
		if !found {
			st.dict.Languages = append(st.dict.Languages, reqCode)
		}
	}

	n := len(st.dict.Languages)
	keys := map[string][]string{}

	for key := range uses {
		existing, had := st.dict.Keys[key]
		if had {
			keys[key] = pad(existing, n)
			continue
		}
		keys[key] = make([]string, n)
	}

	for key, v := range st.dict.Keys {
		if _, ok := keys[key]; !ok {
			keys[key] = pad(v, n)
		}
	}

	st.dict.Keys = keys
	out := format(st.dict)

	if !bytes.Equal(out, st.raw) {
		if err := os.MkdirAll(filepath.Dir(st.path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(st.path, out, 0o644); err != nil {
			return err
		}
	}
	t.report(st, uses)
	return nil
}

// SyncTranslations updates the dictionary file of the module at rootDir: every
// key the code uses gets a slot per language (never overwriting a translation),
// a project's keys used nowhere are removed (a library's are kept: it may
// translate texts built at run time), and the file is written only when it changes.
func (t *Translations) SyncTranslations(rootDir string) error {
	st, mods, err := t.load(rootDir)
	if err != nil {
		return err
	}
	uses := scan(mods, clientBuild, st.isProject)
	n := len(st.dict.Languages)
	keys := map[string][]string{}
	for key := range uses {
		existing, had := st.dict.Keys[key]
		if had {
			keys[key] = pad(existing, n)
			continue
		}
		if st.isProject && st.allFromLibraries(key) {
			continue // libraries already translate it for every project language
		}
		keys[key] = make([]string, n)
	}
	if !st.isProject {
		// A library may translate texts it builds at run time (date's month
		// names), which no scan can see: its existing keys are never removed.
		for key, v := range st.dict.Keys {
			if _, ok := keys[key]; !ok {
				keys[key] = pad(v, n)
			}
		}
	}
	st.dict.Keys = keys
	out := format(st.dict)
	if !bytes.Equal(out, st.raw) {
		if err := os.MkdirAll(filepath.Dir(st.path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(st.path, out, 0o644); err != nil {
			return err
		}
	}
	t.report(st, uses)
	return nil
}

func (st state) allFromLibraries(key string) bool {
	for _, code := range st.dict.Languages {
		if st.libValue(key, code) == "" {
			return false
		}
	}
	return true
}

// missing lists the used keys with no translation (project, then libraries)
// for each language of the dictionary at rootDir.
func (st state) missing(uses keyUses) []Missing {
	var out []Missing
	for _, code := range st.dict.Languages {
		for _, key := range sortedUses(uses) {
			if valueFor(st.dict, key, code) != "" || st.libValue(key, code) != "" {
				continue
			}
			mods := make([]string, 0, len(uses[key]))
			for m := range uses[key] {
				mods = append(mods, m)
			}
			sort.Strings(mods)
			out = append(out, Missing{Key: key, Language: code, Modules: mods})
		}
	}
	return out
}

func sortedUses(u keyUses) []string {
	keys := make([]string, 0, len(u))
	for k := range u {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (t *Translations) report(st state, uses keyUses) {
	count := map[string]int{}
	for _, m := range st.missing(uses) {
		count[m.Language]++
	}
	for _, code := range st.dict.Languages {
		if n := count[code]; n > 0 {
			t.log("lang:", n, "untranslated ("+code+") — see", wpath.Tilde(st.path))
		}
	}
}

// MissingTranslations lists every key used by the code that has no translation
// after merging the project's file with its libraries' files.
func (t *Translations) MissingTranslations(rootDir string) ([]Missing, error) {
	st, mods, err := t.load(rootDir)
	if err != nil {
		return nil, err
	}
	return st.missing(scan(mods, clientBuild, st.isProject)), nil
}

// BundleTranslations returns the <script type="application/json"
// id="webtyp-lang"> element with the merged dictionary (libraries + project;
// the project wins), or "" when the project has no dictionary file.
func (t *Translations) BundleTranslations(rootDir string) (string, error) {
	st, _, err := t.load(rootDir)
	if err != nil {
		return "", err
	}
	if !st.exists {
		return "", nil
	}
	keys := map[string][]string{}
	add := func(key string) {
		if _, done := keys[key]; done {
			return
		}
		v := make([]string, len(st.dict.Languages))
		has := false
		for i, code := range st.dict.Languages {
			v[i] = valueFor(st.dict, key, code)
			if v[i] == "" {
				v[i] = st.libValue(key, code)
			}
			has = has || v[i] != ""
		}
		if has {
			keys[key] = v
		}
	}
	for key := range st.dict.Keys {
		add(key)
	}
	for _, l := range st.libs {
		for key := range l.dict.Keys {
			add(key)
		}
	}
	payload, err := json.Marshal(langFile{Default: st.dict.Default, Languages: st.dict.Languages, Keys: keys})
	if err != nil {
		return "", err
	}
	return `<script type="application/json" id="` + lang.ScriptID + `">` + string(payload) + `</script>`, nil
}
