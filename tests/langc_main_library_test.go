//go:build !wasm

package lang_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"webtyp.com/lang"
	"webtyp.com/lang/langc"
	"webtyp.com/modfind"
)

// fixture for a main library with a demo: a module example.com/demo with config/
// a root lang.json and a Go file using lang.Text("Confirmed").
func mainLibraryFixture(t *testing.T) (app string, mods fakeModules) {
	root := t.TempDir()
	app = filepath.Join(root, "demo")
	write(t, filepath.Join(app, "go.mod"), "module example.com/demo\nrequire webtyp.com/lang v0.0.0\n")
	write(t, filepath.Join(app, "config", ".keep"), "")
	write(t, filepath.Join(app, "lang.json"), `{"languages":["es"],"keys":{"Confirmed":["Confirmada"]}}`)
	write(t, filepath.Join(app, "main.go"), `package main
import tr "webtyp.com/lang"
func main() { _ = tr.Text("Confirmed") }
`)
	mods = fakeModules{{Path: "example.com/demo", Dir: app, LocalDir: app, IsMain: true}}
	return app, mods
}

func TestSync_MainModuleLibraryKeysAreNotCopiedToProject(t *testing.T) {
	app, mods := mainLibraryFixture(t)
	c := langc.New(mods, nil)
	if err := c.SyncTranslations(app); err != nil {
		t.Fatal(err)
	}
	d, _ := readDict(t, filepath.Join(app, "config", "lang.json"))
	if _, ok := d.Keys["Confirmed"]; ok {
		t.Errorf("expected Confirmed to not be copied to config/lang.json")
	}
}

func TestMissing_MainModuleLibraryCountsAsTranslated(t *testing.T) {
	app, mods := mainLibraryFixture(t)
	c := langc.New(mods, nil)
	missing, err := c.MissingTranslations(app)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range missing {
		if m.Key == "Confirmed" {
			t.Errorf("expected Confirmed to not be missing")
		}
	}
}

func TestBundle_UsesMainModuleLibraryValue(t *testing.T) {
	app, mods := mainLibraryFixture(t)
	c := langc.New(mods, nil)
	// Sync first to create config/lang.json
	if err := c.SyncTranslations(app); err != nil {
		t.Fatal(err)
	}
	bundle, err := c.BundleTranslations(app)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle) == 0 {
		t.Fatal("empty bundle")
	}
	var out struct {
		Keys map[string][]string `json:"keys"`
	}
	if err := json.Unmarshal(bundle, &out); err != nil {
		t.Fatalf("failed to unmarshal bundle json: %v", err)
	}

	if val := out.Keys["Confirmed"]; len(val) == 0 || val[0] != "Confirmada" {
		t.Errorf("expected [Confirmada], got %v", val)
	}
}

func TestBundle_ProjectValueWinsOverMainLibrary(t *testing.T) {
	app, mods := mainLibraryFixture(t)
	write(t, filepath.Join(app, "config", "lang.json"), `{"default":"es","languages":["es"],"keys":{"Confirmed":["Confirmado"]}}`)
	c := langc.New(mods, nil)
	bundle, err := c.BundleTranslations(app)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Keys map[string][]string `json:"keys"`
	}
	if err := json.Unmarshal(bundle, &out); err != nil {
		t.Fatalf("failed to unmarshal bundle json: %v", err)
	}

	if val := out.Keys["Confirmed"]; len(val) == 0 || val[0] != "Confirmado" {
		t.Errorf("expected [Confirmado], got %v", val)
	}
}

func TestSync_ProjectWithoutRootLangJSONUnchanged(t *testing.T) {
	// A plain app without root lang.json
	root := t.TempDir()
	app := filepath.Join(root, "demo")
	write(t, filepath.Join(app, "go.mod"), "module example.com/demo\nrequire webtyp.com/lang v0.0.0\n")
	write(t, filepath.Join(app, "config", ".keep"), "")
	write(t, filepath.Join(app, "main.go"), `package main
import tr "webtyp.com/lang"
func main() { _ = tr.Text("Confirmed") }
`)
	mods := fakeModules{{Path: "example.com/demo", Dir: app, LocalDir: app, IsMain: true}}
	c := langc.New(mods, nil)
	if err := c.SyncTranslations(app); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(app, "lang.json")); !os.IsNotExist(err) {
		t.Errorf("expected lang.json to not be created")
	}
	missing, err := c.MissingTranslations(app)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 1 || missing[0].Key != "Confirmed" {
		t.Errorf("expected Confirmed reported untranslated, got %v", missing)
	}
}

func TestLoad_BrokenMainLibraryFileIsSkipped(t *testing.T) {
	app, mods := mainLibraryFixture(t)
	write(t, filepath.Join(app, "lang.json"), `invalid json`)
	c := langc.New(mods, nil)
	// MissingTranslations runs load under the hood
	// but we must check if it's missing or not. If missing, the broken dict was successfully skipped
	missing, err := c.MissingTranslations(app)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range missing {
		if m.Key == "Confirmed" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected Confirmed to be missing because library dict is invalid")
	}
}

func TestLoad_InvalidProjectDictionaryReturnsError(t *testing.T) {
	app, mods := mainLibraryFixture(t)
	write(t, filepath.Join(app, "config", "lang.json"), `invalid json`)
	c := langc.New(mods, nil)
	err := c.SyncTranslations(app)
	if err == nil {
		t.Fatal("expected error from invalid project dictionary")
	}
}

type errorModules struct{}

func (errorModules) Discover(string) ([]modfind.Module, error) { return nil, os.ErrPermission }

func TestLoad_DiscoverErrorReturnsError(t *testing.T) {
	app, _ := mainLibraryFixture(t)
	c := langc.New(errorModules{}, nil)
	_, err := c.MissingTranslations(app)
	if err == nil {
		t.Fatal("expected error from Discover")
	}
}

func TestLoad_BrokenDependencyDictionaryIsSkipped(t *testing.T) {
	app, baseMods := mainLibraryFixture(t)
	lib := filepath.Join(filepath.Dir(app), "lib")
	write(t, filepath.Join(lib, "go.mod"), "module example.com/lib\nrequire webtyp.com/lang v0.0.0\n")
	write(t, filepath.Join(lib, "lang.json"), `invalid json`)
	write(t, filepath.Join(lib, "lib.go"), `package lib
import tr "webtyp.com/lang"
func Do() { _ = tr.Text("LibKey") }
`)
	write(t, filepath.Join(app, "main.go"), `package main
import "example.com/lib"
func main() { lib.Do() }
`)
	mods := append(baseMods, modfind.Module{Path: "example.com/lib", Dir: lib, LocalDir: lib, IsMain: false})

	c := langc.New(mods, nil)
	missing, err := c.MissingTranslations(app)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range missing {
		if m.Key == "LibKey" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected LibKey to be missing because dependency library dict is invalid")
	}
}

// A library run on its own (no config/) ships a lang.json without "default": the bundle must still name one, or the
// page follows the browser's language and never uses the dictionary.
func TestBundle_LibraryWithoutDefaultUsesItsFirstLanguage(t *testing.T) {
	root := t.TempDir()
	lib := filepath.Join(root, "lib")
	write(t, filepath.Join(lib, "go.mod"), "module example.com/lib\nrequire webtyp.com/lang v0.0.0\n")
	write(t, filepath.Join(lib, "lang.json"), `{"languages":["es"],"keys":{"Name":["Nombre"]}}`)
	write(t, filepath.Join(lib, "main.go"), `package main
import tr "webtyp.com/lang"
func main() { _ = tr.Text("Name") }
`)
	c := langc.New(fakeModules{{Path: "example.com/lib", Dir: lib, LocalDir: lib, IsMain: true}}, nil)
	bundle, err := c.BundleTranslations(lib)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Default string `json:"default"`
	}
	if err := json.Unmarshal(bundle, &out); err != nil {
		t.Fatalf("failed to unmarshal bundle json: %v", err)
	}
	if out.Default != "es" {
		t.Errorf("expected default es, got %q", out.Default)
	}
}

// A library run on its own (no config/) renders screens of its dependencies (crudview's "Cancel",
// "Delete"): the bundle must carry their dictionaries too, or those texts show in English.
func TestBundle_LibraryWithoutConfigMergesDependencyDictionaries(t *testing.T) {
	root := t.TempDir()
	lib := filepath.Join(root, "lib")
	dep := filepath.Join(root, "dep")
	write(t, filepath.Join(lib, "go.mod"), "module example.com/lib\nrequire webtyp.com/lang v0.0.0\n")
	write(t, filepath.Join(lib, "lang.json"), `{"languages":["es"],"keys":{"Name":["Nombre"]}}`)
	write(t, filepath.Join(lib, "main.go"), `package main
import tr "webtyp.com/lang"
func main() { _ = tr.Text("Name") }
`)
	write(t, filepath.Join(dep, "go.mod"), "module example.com/dep\nrequire webtyp.com/lang v0.0.0\n")
	write(t, filepath.Join(dep, "lang.json"), `{"languages":["es"],"keys":{"Cancel":["Cancelar"],"Name":["Nombre de la dependencia"]}}`)
	mods := fakeModules{
		{Path: "example.com/lib", Dir: lib, LocalDir: lib, IsMain: true},
		{Path: "example.com/dep", Dir: dep, LocalDir: dep, IsMain: false},
	}
	bundle, err := langc.New(mods, nil).BundleTranslations(lib)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Keys map[string][]string `json:"keys"`
	}
	if err := json.Unmarshal(bundle, &out); err != nil {
		t.Fatalf("failed to unmarshal bundle json: %v", err)
	}
	if got := out.Keys["Cancel"]; len(got) != 1 || got[0] != "Cancelar" {
		t.Errorf("dependency key Cancel = %v, want [Cancelar]", got)
	}
	if got := out.Keys["Name"]; len(got) != 1 || got[0] != "Nombre" {
		t.Errorf("the library's own value must win: Name = %v, want [Nombre]", got)
	}
}

func TestDiagnose_ReportsUntranslatedWithoutWriting(t *testing.T) {
	app, mods := mainLibraryFixture(t)
	write(t, filepath.Join(app, "lang.json"), `{"languages":["es","pt"],"keys":{"Confirmed":["Confirmada",""]}}`)
	write(t, filepath.Join(app, "config", "lang.json"), `{"languages":["es","pt"],"keys":{}}`)
	var logged []string
	c := langc.New(mods, nil)
	c.SetLog(func(a ...any) { logged = append(logged, fmt.Sprint(a...)) })
	before, _ := os.ReadFile(filepath.Join(app, "config", "lang.json"))
	if err := c.Diagnose(app); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(app, "config", "lang.json"))
	if string(before) != string(after) {
		t.Errorf("Diagnose must not rewrite the dictionary")
	}
	if len(logged) == 0 || !strings.Contains(strings.Join(logged, "\n"), "untranslated in pt") {
		t.Errorf("expected an untranslated report for pt, got %v", logged)
	}
}

func TestDiagnose_BrokenDictionaryIsLangsAndRecovers(t *testing.T) {
	app := t.TempDir()
	write(t, filepath.Join(app, "go.mod"), "module example.com/lib\n")
	mods := fakeModules{{Path: "example.com/lib", Dir: app, LocalDir: app, IsMain: true}}
	var logged []string
	c := langc.New(mods, nil)
	c.SetLog(func(a ...any) { logged = append(logged, fmt.Sprint(a...)) })
	write(t, filepath.Join(app, "lang.json"), `{"languages":["esj"],"keys":{}}`)
	_ = c.Diagnose(app)
	_ = c.Diagnose(app)
	if len(logged) != 1 || !strings.Contains(logged[0], "esj") {
		t.Fatalf("broken dictionary must be reported once by LANG, got %v", logged)
	}
	write(t, filepath.Join(app, "lang.json"), `{"languages":["es"],"keys":{"Confirmed":["Confirmada"]}}`)
	_ = c.Diagnose(app)
	if len(logged) != 2 || !strings.Contains(logged[1], "valid again") {
		t.Fatalf("recovery must be said, got %v", logged)
	}
}

func TestWatch_LangJSONEditIsJudgedThenOrchestratorIsTold(t *testing.T) {
	app := t.TempDir()
	write(t, filepath.Join(app, "go.mod"), "module example.com/lib\n")
	write(t, filepath.Join(app, "lang.json"), `{"languages":["esj"],"keys":{}}`)
	mods := fakeModules{{Path: "example.com/lib", Dir: app, LocalDir: app, IsMain: true}}
	var logged []string
	told := 0
	c := langc.New(mods, nil)
	c.SetLog(func(a ...any) { logged = append(logged, fmt.Sprint(a...)) })
	c.Watch(app, func() { told++ })

	if err := c.NewFileEvent("package.json", ".json", filepath.Join(app, "package.json"), "write"); err != nil || told != 0 {
		t.Fatalf("a non-dictionary .json is not LANG's (told=%d, err=%v)", told, err)
	}
	if err := c.NewFileEvent("lang.json", ".json", filepath.Join(app, "lang.json"), "write"); err != nil {
		t.Fatal(err)
	}
	if told != 1 || len(logged) != 1 || !strings.Contains(logged[0], "esj") {
		t.Fatalf("expected one LANG error and one notification, got told=%d logged=%v", told, logged)
	}
}

func TestDictionary_TranslatesDiagnostics(t *testing.T) {
	if err := lang.Load(langc.Dictionary()); err != nil {
		t.Fatalf("lang.Load(langc.Dictionary()): %v", err)
	}
	prev := lang.OutLang()
	defer lang.OutLang(prev)
	lang.OutLang("es")

	app := t.TempDir()
	write(t, filepath.Join(app, "go.mod"), "module example.com/lib\n")
	write(t, filepath.Join(app, "main.go"), "package main\nimport tr \"webtyp.com/lang\"\nfunc main() { _ = tr.Text(\"Confirmed\") }\n")
	mods := fakeModules{{Path: "example.com/lib", Dir: app, LocalDir: app, IsMain: true}}

	var logged []string
	c := langc.New(mods, nil)
	c.SetLog(func(a ...any) { logged = append(logged, fmt.Sprint(a...)) })
	c.Watch(app, nil)

	write(t, filepath.Join(app, "lang.json"), `{"languages":["esk"],"keys":{}}`)
	if err := c.NewFileEvent("lang.json", ".json", filepath.Join(app, "lang.json"), "write"); err != nil {
		t.Fatal(err)
	}
	if len(logged) != 1 || strings.Contains(logged[0], "lang: ") || !strings.Contains(logged[0], "código de idioma") || !strings.Contains(logged[0], "desconocido") || !strings.Contains(logged[0], `"esk"`) {
		t.Fatalf("expected Spanish unknown-language-code error without redundant lang: prefix, got %v", logged)
	}

	write(t, filepath.Join(app, "lang.json"), `{"languages":["es"],"keys":{"Confirmed":[""]}}`)
	if err := c.NewFileEvent("lang.json", ".json", filepath.Join(app, "lang.json"), "write"); err != nil {
		t.Fatal(err)
	}
	all := strings.Join(logged, "\n")
	if !strings.Contains(all, "diccionario válido de nuevo") || !strings.Contains(all, "sin traducir en es") {
		t.Fatalf("expected Spanish recovery and untranslated report, got %v", logged)
	}

	// Verify lang/langc itself passes SyncToolTranslations with zero untranslated/reuse/placeholder warnings.
	langcDir, err := filepath.Abs("../langc")
	if err != nil {
		t.Fatal(err)
	}
	var selfLogs []string
	self := langc.New(fakeModules{{Path: "webtyp.com/lang/langc", Dir: langcDir, LocalDir: langcDir, IsMain: true}}, func(a ...any) {
		selfLogs = append(selfLogs, fmt.Sprint(a...))
	})
	lang.OutLang("en")
	if err := self.SyncToolTranslations(langcDir); err != nil {
		t.Fatalf("SyncToolTranslations on langc: %v", err)
	}
	if len(selfLogs) != 0 {
		t.Fatalf("langc's own dictionary must have zero diagnostics, got %v", selfLogs)
	}
}
