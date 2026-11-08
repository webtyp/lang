package langc

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"

	wpath "webtyp.com/filepath"
	"webtyp.com/modfind"
	"webtyp.com/msgtype"
)

// syncProject keeps an app's config/lang.json as configuration only (default
// and languages): a text is translated once, in the lang.json of the library
// that uses it, and every app gets it. A translation the project still holds
// is moved there the first time (never overwriting a different one: that is
// reported and stays). What remains is the project's own text — a module's
// job — or what no local checkout can receive.
func (t *Translations) syncProject(rootDir string, st state, mods []modfind.Module, uses keyUses) error {
	owners := map[string]modfind.Module{}
	for _, m := range mods {
		if !m.IsMain {
			owners[m.Path] = m
		}
	}
	edits := map[string]*libEdit{}
	var notes []note
	var own []string                   // translated texts only the project uses
	conflicts := map[string][]string{} // "key (code)" → "module says X"
	projectSays := map[string]string{}
	kept := map[string][]string{}
	n := len(st.dict.Languages)

	for _, key := range sortedKeys(st.dict.Keys) {
		vals := pad(st.dict.Keys[key], n)
		if !anyValue(vals) {
			continue // an empty slot is not configuration: dropped
		}
		var libs []modfind.Module
		for mod := range uses[key] {
			if m, ok := owners[mod]; ok {
				libs = append(libs, m)
			}
		}
		sort.Slice(libs, func(i, j int) bool { return libs[i].Path < libs[j].Path })
		if len(libs) == 0 {
			if _, used := uses[key]; used {
				kept[key] = vals
				own = append(own, key)
			}
			continue // used nowhere: dropped
		}
		moved := true
		for _, lib := range libs {
			if lib.LocalDir == "" {
				moved = false
				notes = append(notes, note{msgtype.Warning, []any{key, "can't move to", lib.Path, "— no local checkout of it in the workspace"}})
				continue
			}
			e, err := openLib(edits, lib)
			if err != nil {
				return err
			}
			for i, code := range st.dict.Languages {
				v := vals[i]
				if v == "" {
					continue
				}
				switch cur := valueFor(e.dict, key, code); {
				case cur == "":
					e.set(key, code, v)
				case cur != v:
					moved = false
					k := key + " (" + code + ")"
					projectSays[k] = v
					conflicts[k] = append(conflicts[k], lastPart(lib.Path)+" «"+cur+"»")
				}
			}
		}
		if !moved {
			kept[key] = vals
		}
	}

	if len(own) > 0 {
		notes = append(notes, note{msgtype.Warning, []any{len(own), "texts in", wpath.Tilde(st.path), "are the project's own (e.g.", strings.TrimSpace(own[0]) + ") — move them into a module"}})
	}
	for _, k := range sortedKeys(conflicts) {
		notes = append(notes, note{msgtype.Warning, []any{k + ":", "project «" + projectSays[k] + "» vs", strings.Join(conflicts[k], ", "), "— keep one"}})
	}
	for _, e := range edits {
		if err := e.write(); err != nil {
			return err
		}
	}
	st.dict.Keys = kept
	out := format(st.dict)
	if !bytes.Equal(out, st.raw) {
		if err := os.MkdirAll(filepath.Dir(st.path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(st.path, out, 0o644); err != nil {
			return err
		}
	}

	// Re-read the libraries: the move above changed their files.
	st.libs = dependencyDicts(mods)
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, nt := range notes {
		t.note(nt.typ, nt.parts...)
	}
	t.reportLibraryGaps(st, uses, owners)
	t.reportReuse(st)
	t.reportEmbeddedData(st)
	t.reportIdenticalValues(st)
	t.flush(st.path)
	return nil
}

// reportLibraryGaps names, per library, how many of its texts the app shows
// untranslated in a project language: the fix goes in that library's lang.json.
func (t *Translations) reportLibraryGaps(st state, uses keyUses, owners map[string]modfind.Module) {
	gaps := map[string]map[string]int{} // module → language → count
	var own []string                    // untranslated texts only the project itself uses
	for key, mods := range uses {
		for _, code := range st.dict.Languages {
			if valueFor(st.dict, key, code) != "" || st.libValue(key, code) != "" {
				continue
			}
			byLib := false
			for mod := range mods {
				if _, lib := owners[mod]; !lib {
					continue
				}
				byLib = true
				if gaps[mod] == nil {
					gaps[mod] = map[string]int{}
				}
				gaps[mod][code]++
			}
			if !byLib {
				own = append(own, key)
				break
			}
		}
	}
	if len(own) > 0 {
		sort.Strings(own)
		t.note(msgtype.Warning, len(own), "untranslated texts are the project's own (e.g.", own[0]+") — move them into a module")
	}
	mods := make([]string, 0, len(gaps))
	for m := range gaps {
		mods = append(mods, m)
	}
	sort.Strings(mods)
	for _, m := range mods {
		where := m
		if d := owners[m].SourceDir(); d != "" {
			where = wpath.Tilde(filepath.Join(d, fileName))
		}
		var langs []string
		most := 0
		for _, code := range st.dict.Languages {
			if c := gaps[m][code]; c > 0 {
				langs = append(langs, code)
				if c > most {
					most = c
				}
			}
		}
		if len(langs) > 0 {
			t.note(msgtype.Warning, most, "untranslated in", strings.Join(langs, ", "), "—", where)
		}
	}
}

type note struct {
	typ   msgtype.Type
	parts []any
}

// libEdit is a library's lang.json being written by the move.
type libEdit struct {
	path    string
	dict    langFile
	raw     []byte
	changed bool
}

func openLib(edits map[string]*libEdit, m modfind.Module) (*libEdit, error) {
	if e, ok := edits[m.Path]; ok {
		return e, nil
	}
	p := filepath.Join(m.LocalDir, fileName)
	d, raw, _, err := readDict(p, false)
	if err != nil {
		return nil, err
	}
	d.Languages = withSupportedLanguages(d.Languages)
	if d.Keys == nil {
		d.Keys = map[string][]string{}
	}
	e := &libEdit{path: p, dict: d, raw: raw}
	edits[m.Path] = e
	return e, nil
}

func (e *libEdit) set(key, code, value string) {
	n := len(e.dict.Languages)
	v := pad(e.dict.Keys[key], n)
	for i, c := range e.dict.Languages {
		if c == code {
			v[i] = value
		}
	}
	e.dict.Keys[key] = v
	e.changed = true
}

func (e *libEdit) write() error {
	if !e.changed {
		return nil
	}
	out := format(e.dict)
	if bytes.Equal(out, e.raw) {
		return nil
	}
	return os.WriteFile(e.path, out, 0o644)
}

// lastPart is the last element of a module path: device_manager.
func lastPart(path string) string { return path[strings.LastIndex(path, "/")+1:] }

func anyValue(v []string) bool {
	for _, s := range v {
		if s != "" {
			return true
		}
	}
	return false
}
