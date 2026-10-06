//go:build !wasm

package lang_test

import (
	"testing"

	"webtyp.com/lang"
)

func TestCurrent(t *testing.T) {
	lang.OutLang("es")
	if lang.Current() != "ES" {
		t.Errorf("expected ES, got %s", lang.Current())
	}
	if lang.Current() != "ES" {
		t.Errorf("expected ES, got %s", lang.Current())
	}
}
