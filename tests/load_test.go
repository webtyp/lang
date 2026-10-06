//go:build !wasm

package lang_test

import (
	"strings"
	"testing"

	"webtyp.com/lang"
)

func TestLoad(t *testing.T) {
	dict1 := []byte(`{"languages": ["es", "fr"], "keys": {"Hello": ["Hola", ""]}}`)
	dict2 := []byte(`{"languages": ["fr"], "keys": {"Hello": ["Salut"]}}`)

	err := lang.Load(dict1, dict2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lang.OutLang("fr")
	conv := lang.Translate("Hello").String()
	if conv != "Salut" {
		t.Errorf("expected Salut, got %s", conv)
	}

	lang.OutLang("es")
	conv = lang.Translate("Hello").String()
	if conv != "Hola" {
		t.Errorf("expected Hola, got %s", conv)
	}

	// A failing call loads nothing, not even its valid dictionaries.
	ok := []byte(`{"languages": ["es"], "keys": {"Partial": ["Parcial"]}}`)
	if err := lang.Load(ok, []byte(`{"languages": ["xx"], "keys": {}}`)); err == nil {
		t.Fatal("expected an error for an unknown code")
	}
	lang.OutLang("es")
	if got := lang.Translate("Partial").String(); got != "Partial" {
		t.Errorf("a failing Load must load nothing, got %q", got)
	}

	// Keys match case-insensitively, like lookups.
	if err := lang.Load([]byte(`{"languages": ["es"], "keys": {"hello": ["Hola2"], "World": ["Mundo"]}}`)); err != nil {
		t.Fatal(err)
	}
	if got := lang.Translate("world").String(); got != "Mundo" {
		t.Errorf("case-insensitive lookup after Load, got %q", got)
	}

	// Test unknown code
	dictUnknown := []byte(`{"languages": ["xx"], "keys": {"Test": ["Valor"]}}`)
	err = lang.Load(dictUnknown)
	if err == nil || !strings.Contains(err.Error(), "unknown language code") {
		t.Errorf("expected unknown language error, got %v", err)
	}

	// Test list longer than languages
	dictLong := []byte(`{"languages": ["es"], "keys": {"Test": ["Uno", "Dos"]}}`)
	err = lang.Load(dictLong)
	if err == nil || !strings.Contains(err.Error(), "more values than languages") {
		t.Errorf("expected more values than languages error, got %v", err)
	}

	// Test system default lang fallback
	t.Setenv("LANG", "es_CL.UTF-8")
	// After loading, we just loaded es and fr from dict1 and dict2
	out := lang.OutLang()
	if out != "ES" {
		t.Errorf("expected OutLang() to fallback to ES via system lang, got %s", out)
	}
}
