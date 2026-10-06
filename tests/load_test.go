//go:build !wasm

package lang_test

import (
	"os"
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
	os.Setenv("LANG", "es_CL.UTF-8")
	// After loading, we just loaded es and fr from dict1 and dict2
	out := lang.OutLang()
	if out != "ES" {
		t.Errorf("expected OutLang() to fallback to ES via system lang, got %s", out)
	}
}
