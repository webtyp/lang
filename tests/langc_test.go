//go:build !wasm

package lang_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"webtyp.com/lang/langc"
	"webtyp.com/modfind"
)

// fakeModules is the test double for modfind.Discoverer.
type fakeModules []modfind.Module

func (f fakeModules) Discover(string) ([]modfind.Module, error) { return f, nil }

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

type dict struct {
	Default   string              `json:"default"`
	Languages []string            `json:"languages"`
	Keys      map[string][]string `json:"keys"`
}

func readDict(t *testing.T, path string) (dict, string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var d dict
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("%s is not JSON: %v\n%s", path, err, raw)
	}
	return d, string(raw)
}

// fixture: a project (example.com/app, with config/) using a library
// (example.com/lib) that ships its own lang.json and declares lang.Text fields.
func fixture(t *testing.T) (app, lib string, mods fakeModules) {
	root := t.TempDir()
	app = filepath.Join(root, "app")
	lib = filepath.Join(root, "lib")
	write(t, filepath.Join(app, "go.mod"), "module example.com/app\nrequire webtyp.com/lang v0.0.0\n")
	write(t, filepath.Join(app, "config", ".keep"), "")
	write(t, filepath.Join(app, "ui", "screen.go"), `package ui

import (
	"webtyp.com/fmt"
	"webtyp.com/input"
	tr "webtyp.com/lang"
	"webtyp.com/model"

	"example.com/lib/bar"
)

var DeviceModel = model.Definition{Fields: model.Fields{
	{Name: "ip", Label: "IP address", Help: "Format: 192.168.1.1"},
	{Name: "is_active"},
}}

func render() {
	_ = tr.Translate("Delete", "%s?", "This", "action")
	_ = tr.Translate("Pick a conversation")
	_ = fmt.Err("name", "required")
	_ = input.Radio(fmt.KeyValue{Key: "pc", Value: "Computer"})
	_ = bar.Bar{Placeholder: "Search patients", Name: "Ana"}
	_ = []bar.Bar{{Placeholder: "Find rooms"}}
	_ = tr.Text("Converted")
	_ = 192.168
}

type presenter struct{}

func (presenter) SearchPlaceholder() tr.Text { return "Search devices" }
`)
	write(t, filepath.Join(lib, "go.mod"), "module example.com/lib\nrequire webtyp.com/lang v0.0.0\n")
	write(t, filepath.Join(lib, "bar", "bar.go"), `package bar

import (
	. "webtyp.com/lang"
)

type Bar struct {
	Placeholder Text
	Name        string
}

func (b Bar) Render() string { return Translate(b.Placeholder, "Close").String() }
`)
	write(t, filepath.Join(lib, "lang.json"), `{"languages": ["fr", "es"], "keys": {"Close": ["Fermer", "Cerrar"], "Delete": ["Supprimer", "Eliminar"]}}`)
	mods = fakeModules{
		{Path: "example.com/app", Dir: app, LocalDir: app, IsMain: true},
		{Path: "example.com/lib", Dir: lib, LocalDir: lib},
	}
	return app, lib, mods
}

func TestSync_CreatesProjectFileWithEveryKeySource(t *testing.T) {
	app, _, mods := fixture(t)
	var logs []string
	tr := langc.New(mods, func(a ...any) {
		parts := make([]string, len(a))
		for i, v := range a {
			b, _ := json.Marshal(v)
			parts[i] = strings.Trim(string(b), `"`)
		}
		logs = append(logs, strings.Join(parts, " "))
	})
	if err := tr.SyncTranslations(app); err != nil {
		t.Fatal(err)
	}
	d, raw := readDict(t, filepath.Join(app, "config", "lang.json"))
	if d.Default != "es" || len(d.Languages) != 1 || d.Languages[0] != "es" {
		t.Errorf("new project file must default to es: %+v", d)
	}
	want := []string{
		"IP address", "Format: 192.168.1.1", "is active", // rules 3, 6 (and humanised name)
		"This", "action", "Pick a conversation", // rule 1: one key per argument, as written
		"name", "required", // rule 2
		"Computer",                      // rule 4
		"Search patients", "Find rooms", // rule 7, direct and elided element
		"Converted",      // rule 7, conversion
		"Search devices", // rule 8
	}
	for _, k := range want {
		if _, ok := d.Keys[k]; !ok {
			t.Errorf("missing key %q in\n%s", k, raw)
		}
	}
	for _, k := range []string{"Ana", "%s?", "Pick", "conversation", "Delete", "Close"} {
		if _, ok := d.Keys[k]; ok {
			t.Errorf("key %q must not be in the project file:\n%s", k, raw)
		}
	}
	// "Delete" and "Close" are translated by the library for every project
	// language (es), matched by code although the library lists fr first.
	if !strings.Contains(raw, `"IP address": [""]`) {
		t.Errorf("each key must sit on one line as a positional list:\n%s", raw)
	}
	if len(logs) == 0 || !strings.Contains(logs[0], "untranslated (es)") {
		t.Errorf("sync must log what is untranslated, got %v", logs)
	}
}

func TestSync_KeepsTranslationsRemovesUnusedAndIsStable(t *testing.T) {
	app, _, mods := fixture(t)
	path := filepath.Join(app, "config", "lang.json")
	write(t, path, `{"default": "es", "languages": ["es"], "keys": {"This": ["Esta"], "Gone": ["Ya no"], "Close": ["Cerrar ya"]}}`)
	tr := langc.New(mods, nil)
	if err := tr.SyncTranslations(app); err != nil {
		t.Fatal(err)
	}
	d, _ := readDict(t, path)
	if got := d.Keys["This"]; len(got) != 1 || got[0] != "Esta" {
		t.Errorf("an existing translation must never be overwritten, got %v", got)
	}
	if _, ok := d.Keys["Gone"]; ok {
		t.Error("a key used nowhere must be removed")
	}
	if got := d.Keys["Close"]; len(got) != 1 || got[0] != "Cerrar ya" {
		t.Errorf("a project value differing from the library is an override and is kept, got %v", got)
	}

	info, _ := os.Stat(path)
	time.Sleep(20 * time.Millisecond)
	if err := tr.SyncTranslations(app); err != nil {
		t.Fatal(err)
	}
	if again, _ := os.Stat(path); !again.ModTime().Equal(info.ModTime()) {
		t.Error("the file must not be rewritten when nothing changed")
	}
}

func TestSync_PositionalIntegrity(t *testing.T) {
	app, _, mods := fixture(t)
	path := filepath.Join(app, "config", "lang.json")
	write(t, path, `{"default": "es", "languages": ["es", "fr"], "keys": {"This": ["Esta"]}}`)
	tr := langc.New(mods, nil)
	if err := tr.SyncTranslations(app); err != nil {
		t.Fatal(err)
	}
	d, _ := readDict(t, path)
	if got := d.Keys["This"]; len(got) != 2 || got[0] != "Esta" || got[1] != "" {
		t.Errorf("appending a language pads every list, got %v", got)
	}

	bad := `{"default": "es", "languages": ["es"], "keys": {"This": ["Esta", "Cette"]}}`
	write(t, path, bad)
	err := tr.SyncTranslations(app)
	if err == nil || !strings.Contains(err.Error(), "more values than languages") || !strings.Contains(err.Error(), `"This"`) {
		t.Errorf("a list longer than languages must fail naming the key, got %v", err)
	}
	if raw, _ := os.ReadFile(path); string(raw) != bad {
		t.Error("a failing sync must leave the file untouched")
	}

	write(t, path, `{"default": "es", "languages": ["es", "xx"], "keys": {}}`)
	if err := tr.SyncTranslations(app); err == nil || !strings.Contains(err.Error(), `"xx"`) {
		t.Errorf("an unknown language code must fail naming it, got %v", err)
	}
}

func TestBundle_MergesLibrariesProjectWinsAndEscapes(t *testing.T) {
	app, _, mods := fixture(t)
	write(t, filepath.Join(app, "config", "lang.json"),
		`{"default": "es", "languages": ["es"], "keys": {"Delete": ["Borrar"], "This": ["Esta"], "Send": [""], "Tag": ["<b>"]}}`)
	tr := langc.New(mods, nil)
	el, err := tr.BundleTranslations(app)
	if err != nil {
		t.Fatal(err)
	}
	prefix := `<script type="application/json" id="webtyp-lang">`
	if !strings.HasPrefix(el, prefix) || !strings.HasSuffix(el, "</script>") {
		t.Fatalf("not the dictionary element: %s", el)
	}
	payload := strings.TrimSuffix(strings.TrimPrefix(el, prefix), "</script>")
	if strings.Contains(payload, "<") {
		t.Errorf("the payload must escape <, got %s", payload)
	}
	var d dict
	if err := json.Unmarshal([]byte(payload), &d); err != nil {
		t.Fatal(err)
	}
	checks := map[string]string{"Delete": "Borrar", "This": "Esta", "Close": "Cerrar", "Tag": "<b>"}
	for k, want := range checks {
		if got := d.Keys[k]; len(got) != 1 || got[0] != want {
			t.Errorf("bundle[%q] = %v, want [%q]", k, got, want)
		}
	}
	if _, ok := d.Keys["Send"]; ok {
		t.Error("an all-empty key must be dropped from the bundle")
	}
}

func TestMissing_ListsModules(t *testing.T) {
	app, _, mods := fixture(t)
	tr := langc.New(mods, nil)
	if err := tr.SyncTranslations(app); err != nil {
		t.Fatal(err)
	}
	missing, err := tr.MissingTranslations(app)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range missing {
		if m.Key == "Close" || m.Key == "Delete" {
			t.Errorf("%q is translated by the library and must not be missing", m.Key)
		}
		if m.Key == "Search devices" && m.Language == "es" && len(m.Modules) == 1 && m.Modules[0] == "example.com/app" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected \"Search devices\" [es] from example.com/app in %+v", missing)
	}
}

func TestSync_LibraryMode(t *testing.T) {
	_, lib, _ := fixture(t)
	write(t, filepath.Join(lib, "ui.go"), `package lib
import "webtyp.com/fmt"
func init() { _ = fmt.Err("internal failure") }
`)

	if err := os.Remove(filepath.Join(lib, "lang.json")); err != nil {
		t.Fatal(err)
	}
	tr := langc.New(fakeModules{{Path: "example.com/lib", Dir: lib, LocalDir: lib, IsMain: true}}, nil)
	if err := tr.SyncTranslations(lib); err != nil {
		t.Fatal(err)
	}
	d, raw := readDict(t, filepath.Join(lib, "lang.json"))
	if d.Default != "" || strings.Contains(raw, `"default"`) {
		t.Errorf("a library file has no default:\n%s", raw)
	}
	if _, ok := d.Keys["Close"]; !ok {
		t.Errorf("library keys must be collected from its own code:\n%s", raw)
	}

	if _, ok := d.Keys["internal failure"]; !ok {
		t.Errorf("library mode must pick up fmt.Err in its own module")
	}

	// A library key no scan can see (a month name built at run time) is kept.
	write(t, filepath.Join(lib, "lang.json"), `{"languages": ["es"], "keys": {"January": ["Enero"]}}`)
	if err := tr.SyncTranslations(lib); err != nil {
		t.Fatal(err)
	}
	d, raw = readDict(t, filepath.Join(lib, "lang.json"))
	if got := d.Keys["January"]; len(got) != 1 || got[0] != "Enero" {
		t.Errorf("library mode must keep keys it cannot see in code:\n%s", raw)
	}
}

func TestSyncToolMode(t *testing.T) {
	root := t.TempDir()
	tool := filepath.Join(root, "tool")
	write(t, filepath.Join(tool, "go.mod"), "module example.com/tool\nrequire webtyp.com/lang v0.0.0\n")
	write(t, filepath.Join(tool, "main.go"), `//go:build !wasm
package main
import (
	"webtyp.com/lang"
	"webtyp.com/fmt"
)
func main() {
	lang.Translate("Server", "started")
	fmt.Err("port", "busy")
}
`)

	mods := fakeModules{{Path: "example.com/tool", Dir: tool, LocalDir: tool, IsMain: true}}
	tr := langc.New(mods, nil)

	// Tool mode
	if err := tr.SyncToolTranslations(tool); err != nil {
		t.Fatal(err)
	}

	d, raw := readDict(t, filepath.Join(tool, "lang.json"))

	if len(d.Languages) != 8 {
		t.Errorf("expected 8 languages for tool mode, got %d: %v", len(d.Languages), d.Languages)
	}
	if d.Languages[0] != "es" || d.Languages[7] != "ru" {
		t.Errorf("unexpected languages for tool mode: %v", d.Languages)
	}

	for _, k := range []string{"Server", "started", "port", "busy"} {
		if v, ok := d.Keys[k]; !ok {
			t.Errorf("tool mode missing key %q:\n%s", k, raw)
		} else if len(v) != 8 {
			t.Errorf("tool mode key %q does not have 8 slots: %v", k, v)
		}
	}

	// Library mode should NOT see them since they are in !wasm file
	if err := os.Remove(filepath.Join(tool, "lang.json")); err != nil {
		t.Fatal(err)
	}
	if err := tr.SyncTranslations(tool); err != nil {
		t.Fatal(err)
	}
	dClient, _ := readDict(t, filepath.Join(tool, "lang.json"))
	for _, k := range []string{"Server", "started", "port", "busy"} {
		if _, ok := dClient.Keys[k]; ok {
			t.Errorf("client mode should NOT pick up keys from !wasm file, got %q", k)
		}
	}
}

func TestSyncToolMode_AppendsLanguages(t *testing.T) {
	root := t.TempDir()
	tool := filepath.Join(root, "tool")
	write(t, filepath.Join(tool, "go.mod"), "module example.com/tool\n")
	write(t, filepath.Join(tool, "lang.json"), `{"languages": ["es"], "keys": {"Hola": ["Hello"]}}`)
	write(t, filepath.Join(tool, "main.go"), `package main`)

	mods := fakeModules{{Path: "example.com/tool", Dir: tool, LocalDir: tool, IsMain: true}}
	tr := langc.New(mods, nil)

	if err := tr.SyncToolTranslations(tool); err != nil {
		t.Fatal(err)
	}

	d, _ := readDict(t, filepath.Join(tool, "lang.json"))
	if len(d.Languages) != 8 {
		t.Errorf("expected 8 languages, got %d: %v", len(d.Languages), d.Languages)
	}

	if d.Languages[0] != "es" || d.Languages[1] != "zh" {
		t.Errorf("expected es, zh, got %s, %s", d.Languages[0], d.Languages[1])
	}

	if got := d.Keys["Hola"]; len(got) != 8 || got[0] != "Hello" || got[1] != "" {
		t.Errorf("expected 8 slots padded correctly, got %v", got)
	}
}
