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
		// A library that ships its own demo is a project (it has config/) AND a
		// library: its root lang.json translates its own texts and must count.
		own := filepath.Join(rootDir, fileName)
		if d, _, ok, err := readDict(own, false); err == nil && ok && validate(own, d) == nil {
			st.libs = append(st.libs, library{module: modulePath(rootDir), dict: d})
		}
		st.libs = append(st.libs, dependencyDicts(mods)...)
		return st, mods, nil
	}
	// A library run on its own scans only itself, but its screens show texts of its dependencies
	// (crudview's "Cancel", "Delete"): their dictionaries are merged too. Its own file still wins.
	deps, err := t.modules.Discover(rootDir)
	if err != nil {
		return st, nil, err
	}
	st.libs = dependencyDicts(deps)
	return st, mods, nil
}

// dependencyDicts reads the lang.json of every non-main module; a broken file is skipped, never
// blocking the caller.
func dependencyDicts(mods []modfind.Module) []library {
	var out []library
	for _, m := range mods {
		if m.IsMain || m.SourceDir() == "" {
			continue
		}
		p := filepath.Join(m.SourceDir(), fileName)
		d, _, ok, err := readDict(p, false)
		if err != nil || !ok || validate(p, d) != nil {
			continue
		}
		out = append(out, library{module: m.Path, dict: d})
	}
	return out
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

// withSupportedLanguages returns existing with every lang.Supported() code
// appended that it doesn't already have (order and existing values kept). "en"
// is never added: the Go string literal that keys a translation already IS
// the English text, so it needs no column of its own.
func withSupportedLanguages(existing []string) []string {
	out := existing
	for _, code := range lang.Supported() {
		if code == "en" {
			continue
		}
		found := false
		for _, have := range out {
			if have == code {
				found = true
				break
			}
		}
		if !found {
			out = append(out, code)
		}
	}
	return out
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

	st.dict.Languages = withSupportedLanguages(st.dict.Languages)

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
	t.reportReuse(st)
	t.reportEmbeddedData(st)
	t.reportIdenticalValues(st)
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
	if !st.isProject {
		// webtyp is a global framework: a library's own dictionary serves every
		// consumer, so it carries every language the ecosystem supports. A
		// project (an app) keeps full control of its own config/lang.json —
		// BundleTranslations only ever emits the project's own languages
		// (langc.go BundleTranslations), so this never bloats a consumer that
		// wants fewer than all nine.
		st.dict.Languages = withSupportedLanguages(st.dict.Languages)
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
	t.reportReuse(st)
	t.reportEmbeddedData(st)
	t.reportIdenticalValues(st)
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

// reportReuse flags dictionary keys that duplicate each other's words instead
// of reusing a shared one — every unreused phrase is a dictionary row AND
// binary bytes in every WASM consumer, in every language. Two multi-word keys
// that share a leading or trailing run of 2+ words (e.g. "not a valid
// integer" / "not a valid decimal number" share "not a valid") are a sign the
// call site should pass the shared run as its own fmt.Err argument and vary
// only the rest — never fatal, same severity as the untranslated-count line.
func (t *Translations) reportReuse(st state) {
	type tokenized struct {
		key   string
		words []string
	}
	var multi []tokenized
	for k := range st.dict.Keys {
		if w := strings.Fields(k); len(w) >= 2 {
			multi = append(multi, tokenized{key: k, words: w})
		}
	}
	sort.Slice(multi, func(i, j int) bool { return multi[i].key < multi[j].key })

	reported := map[string]bool{}
	flag := func(frag string, a, b string) {
		if reported[frag] {
			return
		}
		reported[frag] = true
		t.log("lang:", a, "and", b, "share", frag, "— extract it as its own key and reuse it, see", wpath.Tilde(st.path))
	}
	for i := 0; i < len(multi); i++ {
		for j := i + 1; j < len(multi); j++ {
			if p := commonRun(multi[i].words, multi[j].words, true); len(p) >= 2 {
				flag(strings.Join(p, " "), multi[i].key, multi[j].key)
			}
			if s := commonRun(multi[i].words, multi[j].words, false); len(s) >= 2 {
				flag(strings.Join(s, " "), multi[i].key, multi[j].key)
			}
		}
	}
}

// reportEmbeddedData flags a key that embeds a comma-separated run of 3+ very
// short tokens (2 characters or less each — "c, r, u, d"): a sign the key
// hardcodes a literal/enum code list instead of passing it as data (a
// variable built from the same constants the code already validates
// against, e.g. a slice of {code, letter} pairs). A hardcoded list goes
// stale the moment a code is added or renamed, and — worse — gets
// "translated" into every language by mistake, when it should read the same
// in all of them.
func (t *Translations) reportEmbeddedData(st state) {
	for k := range st.dict.Keys {
		if run := shortCommaRun(k); run >= 3 {
			t.log("lang: ERROR", k, "embeds a", run, "-item code list — pass it as data (a variable), not a literal, see", wpath.Tilde(st.path))
		}
	}
}

// shortCommaRun returns the length of the longest run of comma-separated
// segments whose final word is 1-2 characters long (0 if none).
func shortCommaRun(s string) int {
	best, run := 0, 0
	for _, part := range strings.Split(s, ",") {
		word := strings.TrimSpace(part)
		if i := strings.LastIndexByte(word, ' '); i >= 0 {
			word = word[i+1:]
		}
		if len(word) >= 1 && len(word) <= 2 {
			run++
			if run > best {
				best = run
			}
			continue
		}
		run = 0
	}
	return best
}

// reportIdenticalValues flags a key whose translated value ends in the exact
// same run of words in EVERY language the dictionary carries (2+ languages,
// all translated) — if nobody wrote anything different for it in any
// language, it almost certainly isn't prose at all: it's a code, a symbol or
// a number baked into an otherwise-translated phrase ("— the verbs are c, r,
// u, d" ends in "c, r, u, d" unchanged in es/zh/hi/ar/pt/fr/de/ru). Pass it
// as data (a variable) instead of writing it inside the literal.
func (t *Translations) reportIdenticalValues(st state) {
	n := len(st.dict.Languages)
	if n < 2 {
		return
	}
	for key, values := range st.dict.Keys {
		if len(values) < n {
			continue
		}
		run := strings.Fields(values[0])
		complete := values[0] != ""
		for _, v := range values[1:n] {
			if v == "" {
				complete = false
				break
			}
			run = commonRun(run, strings.Fields(v), false)
			if len(run) == 0 {
				break
			}
		}
		if !complete || len(run) == 0 {
			continue
		}
		frag := strings.Join(run, " ")
		if strings.HasSuffix(key, frag) {
			t.log("lang: ERROR", key, "ends with", frag, "unchanged in every language — pass it as data (a variable), not a literal, see", wpath.Tilde(st.path))
		}
	}
}

// commonRun returns the longest shared leading (fromStart) or trailing run of
// whole words between a and b.
func commonRun(a, b []string, fromStart bool) []string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var run []string
	for i := 0; i < n; i++ {
		var wa, wb string
		if fromStart {
			wa, wb = a[i], b[i]
		} else {
			wa, wb = a[len(a)-1-i], b[len(b)-1-i]
		}
		if wa != wb {
			break
		}
		if fromStart {
			run = append(run, wa)
		} else {
			run = append([]string{wa}, run...)
		}
	}
	return run
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
	// A library run on its own has no config/ and so no default: the page would then follow the browser's language and
	// ignore the only dictionary it carries. The first language of the file is the one its authors translate into.
	def := st.dict.Default
	if def == "" && len(st.dict.Languages) > 0 {
		def = st.dict.Languages[0]
	}
	payload, err := json.Marshal(langFile{Default: def, Languages: st.dict.Languages, Keys: keys})
	if err != nil {
		return "", err
	}
	return `<script type="application/json" id="` + lang.ScriptID + `">` + string(payload) + `</script>`, nil
}
