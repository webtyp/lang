//go:build !wasm

package lang_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"webtyp.com/lang"
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
	{Name: "id"},
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
	_ = tr.Translate(msgSaved, bar.MsgClosed)
	_ = 192.168
}

const msgSaved = "Saved"

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

// MsgClosed is a named constant passed to Translate from another package.
const MsgClosed = "Closed"

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
		"Converted",       // rule 7, conversion
		"Search devices",  // rule 8
		"Saved", "Closed", // named constants (same package and imported)
	}
	// An app's file is configuration only: the keys are what is reported
	// untranslated, never written into it.
	if len(d.Keys) != 0 {
		t.Errorf("the project file must hold no keys:\n%s", raw)
	}
	missing, err := tr.MissingTranslations(app)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, m := range missing {
		got[m.Key] = true
	}
	for _, k := range want {
		if !got[k] {
			t.Errorf("missing key %q in %v", k, missing)
		}
	}
	// "Delete" and "Close" are translated by the library for every project
	// language (es), matched by code although the library lists fr first.
	for _, k := range []string{"Ana", "%s?", "Pick", "conversation", "Delete", "Close", "id"} {
		if got[k] {
			t.Errorf("key %q must not be reported untranslated", k)
		}
	}
	if len(logs) == 0 || !strings.Contains(strings.Join(logs, "\n"), "untranslated") {
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

	// A library key no scan can see (a month name built at run time) is kept,
	// and its existing translation survives the library being widened to
	// every language webtyp supports (a global framework: a library serves
	// every consumer, so its own dictionary carries every supported language).
	write(t, filepath.Join(lib, "lang.json"), `{"languages": ["es"], "keys": {"January": ["Enero"]}}`)
	if err := tr.SyncTranslations(lib); err != nil {
		t.Fatal(err)
	}
	d, raw = readDict(t, filepath.Join(lib, "lang.json"))
	if got := d.Keys["January"]; len(got) != len(d.Languages) || got[0] != "Enero" {
		t.Errorf("library mode must keep keys it cannot see in code:\n%s", raw)
	}
	if len(d.Languages) != len(lang.Supported())-1 {
		t.Errorf("a library must be widened to every supported language except en, got %v", d.Languages)
	}
	for _, code := range d.Languages {
		if code == "en" {
			t.Errorf("en must never get a column: %v", d.Languages)
		}
	}
}

// TestSync_ReportsReusableWords: a library whose code builds two error
// messages that duplicate the same leading words ("not a valid X") should be
// nudged to extract the shared run into its own key instead — every
// unreused phrase is dictionary rows and WASM binary bytes, in every
// language, in every consumer.
func TestSync_ReportsReusableWords(t *testing.T) {
	_, lib, _ := fixture(t)
	write(t, filepath.Join(lib, "bar", "bar.go"), `package bar

import "webtyp.com/fmt"

func ValidateInt(v string) error  { return fmt.Err("not a valid integer") }
func ValidateFloat(v string) error { return fmt.Err("not a valid decimal number") }
`)
	if err := os.Remove(filepath.Join(lib, "lang.json")); err != nil {
		t.Fatal(err)
	}
	var logs []string
	tr := langc.New(fakeModules{{Path: "example.com/lib", Dir: lib, LocalDir: lib, IsMain: true}}, func(a ...any) {
		parts := make([]string, len(a))
		for i, v := range a {
			b, _ := json.Marshal(v)
			parts[i] = strings.Trim(string(b), `"`)
		}
		logs = append(logs, strings.Join(parts, " "))
	})
	if err := tr.SyncTranslations(lib); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range logs {
		if strings.Contains(l, "share not a valid —") {
			found = true
		}
	}
	if !found {
		t.Errorf("sync must flag keys sharing 2+ leading words, got %v", logs)
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

// TestSync_SameDiagnosticsReportedOnce: every module reload syncs again; a
// sync that finds the same problems must not repeat them in the TUI.
func TestSync_SameDiagnosticsReportedOnce(t *testing.T) {
	app, _, mods := fixture(t)
	n := 0
	tr := langc.New(mods, func(...any) { n++ })
	for i := 0; i < 3; i++ {
		if err := tr.SyncTranslations(app); err != nil {
			t.Fatal(err)
		}
	}
	first := n
	if first == 0 {
		t.Fatal("the first sync must report what is untranslated")
	}
	if err := tr.SyncTranslations(app); err != nil {
		t.Fatal(err)
	}
	if n != first {
		t.Errorf("unchanged diagnostics reported again: %d lines after 4 syncs, want %d", n, first)
	}
}

// TestSync_ToolModuleOutsideClientBuild: a module the project imports only
// from a file excluded from the client build (config/deploy.go, tag deploy)
// is a tool — its texts must not land in the project's dictionary.
func TestSync_ToolModuleOutsideClientBuild(t *testing.T) {
	app, _, mods := fixture(t)
	tool := filepath.Join(filepath.Dir(app), "tool")
	write(t, filepath.Join(tool, "go.mod"), "module example.com/tool\nrequire webtyp.com/lang v0.0.0\n")
	write(t, filepath.Join(tool, "tool.go"), `package tool

import "webtyp.com/lang"

func Check() string { return lang.Translate("Release requires a clean working tree").String() }
`)
	write(t, filepath.Join(app, "config", "deploy.go"), `//go:build deploy

package config

import "example.com/tool"

var _ = tool.Check
`)
	// A package reached only from a server file still reaches the app: an
	// error it returns is translated in the browser.
	srv := filepath.Join(filepath.Dir(app), "srv")
	write(t, filepath.Join(srv, "go.mod"), "module example.com/srv\nrequire webtyp.com/lang v0.0.0\n")
	write(t, filepath.Join(srv, "srv.go"), `package srv

import "webtyp.com/lang"

func Label() string { return lang.Translate("sign count").String() }
`)
	write(t, filepath.Join(app, "web", "server", "main.go"), `//go:build !wasm

package main

import "example.com/srv"

func main() { _ = srv.Label() }
`)
	mods = append(mods,
		modfind.Module{Path: "example.com/tool", Dir: tool, LocalDir: tool},
		modfind.Module{Path: "example.com/srv", Dir: srv, LocalDir: srv})
	missing, err := langc.New(mods, nil).MissingTranslations(app)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, m := range missing {
		got[m.Key] = true
	}
	if got["Release requires a clean working tree"] {
		t.Errorf("a tool module's text must not count for the app: %v", missing)
	}
	if !got["sign count"] {
		t.Errorf("a package reached from the server build must count: %v", missing)
	}
	if !got["Pick a conversation"] {
		t.Errorf("the project's own keys must count: %v", missing)
	}
}

// TestSync_ProjectTranslationMovesToItsLibrary: an app's config/lang.json is
// configuration only. A translation it still holds moves, once, to the
// lang.json of the library that uses the key — every app then gets it — and
// a different translation already there is never overwritten.
func TestSync_ProjectTranslationMovesToItsLibrary(t *testing.T) {
	app, lib, mods := fixture(t)
	write(t, filepath.Join(app, "config", "lang.json"), `{"default": "es", "languages": ["es"], "keys": {
  "Find rooms": ["Buscar salas"],
  "Close": ["Clausurar"],
  "Pick a conversation": ["Elige una conversación"],
  "Unused": ["Sin uso"]
}}`)
	write(t, filepath.Join(lib, "bar", "find.go"), `package bar

import . "webtyp.com/lang"

func Find() string { return Translate("Find rooms").String() }
`)
	if err := langc.New(mods, nil).SyncTranslations(app); err != nil {
		t.Fatal(err)
	}
	ld, lraw := readDict(t, filepath.Join(lib, "lang.json"))
	es := -1
	for i, c := range ld.Languages {
		if c == "es" {
			es = i
		}
	}
	if v := ld.Keys["Find rooms"]; es < 0 || len(v) <= es || v[es] != "Buscar salas" {
		t.Errorf("the project's translation must move to the library:\n%s", lraw)
	}
	if v := ld.Keys["Close"]; v[es] != "Cerrar" {
		t.Errorf("a library translation must never be overwritten:\n%s", lraw)
	}
	d, raw := readDict(t, filepath.Join(app, "config", "lang.json"))
	if _, ok := d.Keys["Find rooms"]; ok {
		t.Errorf("a moved key must leave the project file:\n%s", raw)
	}
	if _, ok := d.Keys["Unused"]; ok {
		t.Errorf("a key used nowhere must leave the project file:\n%s", raw)
	}
	if _, ok := d.Keys["Close"]; !ok {
		t.Errorf("a conflicting translation stays until a person keeps one:\n%s", raw)
	}
	if _, ok := d.Keys["Pick a conversation"]; !ok {
		t.Errorf("the project's own text stays until it moves into a module:\n%s", raw)
	}
}

// TestSync_LibraryDropsKeyItsDependencyTranslates: a shared column ("tenant
// id") is translated once, in the dependency that declares it; a library that
// uses it drops its copy unless it says something different.
func TestSync_LibraryDropsKeyItsDependencyTranslates(t *testing.T) {
	root := t.TempDir()
	lib := filepath.Join(root, "lib")
	base := filepath.Join(root, "base")
	write(t, filepath.Join(base, "go.mod"), "module example.com/base\n")
	write(t, filepath.Join(base, "lang.json"), `{"languages": ["es"], "keys": {"tenant id": ["id del inquilino"], "user id": ["id de usuario"]}}`)
	write(t, filepath.Join(lib, "go.mod"), "module example.com/lib\nrequire webtyp.com/lang v0.0.0\n")
	write(t, filepath.Join(lib, "lib.go"), `package lib

import "webtyp.com/lang"

var _ = []string{lang.Translate("tenant id").String(), lang.Translate("user id").String(), lang.Translate("Own").String()}
`)
	write(t, filepath.Join(lib, "lang.json"), `{"languages": ["es"], "keys": {"tenant id": ["id de tenant"], "user id": [""], "Own": ["Propio"]}}`)
	mods := fakeModules{
		{Path: "example.com/lib", Dir: lib, LocalDir: lib, IsMain: true},
		{Path: "example.com/base", Dir: base, LocalDir: base},
	}
	if err := langc.New(mods, nil).SyncTranslations(lib); err != nil {
		t.Fatal(err)
	}
	d, raw := readDict(t, filepath.Join(lib, "lang.json"))
	if _, ok := d.Keys["user id"]; ok {
		t.Errorf("a key the dependency translates must leave the library:\n%s", raw)
	}
	if _, ok := d.Keys["tenant id"]; !ok {
		t.Errorf("a library value different from the dependency's stays:\n%s", raw)
	}
	if _, ok := d.Keys["Own"]; !ok {
		t.Errorf("the library's own key stays:\n%s", raw)
	}
}

// TestScan_ClientBuildIsTheBrowserBinary: the browser binary is built with
// -tags browser. A server-only file (!browser) is not browser text; a
// browser.go is.
func TestScan_ClientBuildIsTheBrowserBinary(t *testing.T) {
	app, _, mods := fixture(t)
	write(t, filepath.Join(app, "ui", "server.go"), `//go:build !browser

package ui

import "webtyp.com/lang"

var _ = lang.Translate("Server boot failed")
`)
	write(t, filepath.Join(app, "ui", "browser.go"), `//go:build browser

package ui

import "webtyp.com/lang"

var _ = lang.Translate("Screen title")
`)
	missing, err := langc.New(mods, nil).MissingTranslations(app)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, m := range missing {
		got[m.Key] = true
	}
	if got["Server boot failed"] {
		t.Errorf("a !browser file is not browser text: %v", missing)
	}
	if !got["Screen title"] {
		t.Errorf("a browser.go is browser text: %v", missing)
	}
}

// TestScan_WireDefinitionIsNoText: a model.Definition{Wire: true} is a
// transport format; its field names are protocol, never screen text.
func TestScan_WireDefinitionIsNoText(t *testing.T) {
	app, _, mods := fixture(t)
	write(t, filepath.Join(app, "ui", "wire.go"), `package ui

import "webtyp.com/model"

var rpcModel = model.Definition{Name: "rpc", Wire: true, Fields: model.Fields{{Name: "jsonrpc"}, {Name: "next_cursor"}}}

var formModel = model.Definition{Name: "contact", Fields: model.Fields{{Name: "phone_number"}}}
`)
	missing, err := langc.New(mods, nil).MissingTranslations(app)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, m := range missing {
		got[m.Key] = true
	}
	if got["jsonrpc"] || got["next cursor"] {
		t.Errorf("a Wire definition's fields are no text: %v", missing)
	}
	if !got["phone number"] {
		t.Errorf("a regular definition's fields are text: %v", missing)
	}
}
