//go:build !wasm

package lang_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	if bundle == "" {
		t.Fatal("empty bundle")
	}
	var out struct {
		Keys map[string][]string `json:"keys"`
	}
	if err := json.Unmarshal([]byte(bundle[strings.Index(bundle, ">")+1:strings.LastIndex(bundle, "<")]), &out); err != nil {
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
	if err := json.Unmarshal([]byte(bundle[strings.Index(bundle, ">")+1:strings.LastIndex(bundle, "<")]), &out); err != nil {
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
	d, _ := readDict(t, filepath.Join(app, "config", "lang.json"))
	if _, ok := d.Keys["Confirmed"]; !ok {
		t.Errorf("expected Confirmed to be added as untranslated")
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
