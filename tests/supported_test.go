package lang_test

import (
	"strings"
	"testing"

	"webtyp.com/lang"
)

func TestSupported(t *testing.T) {
	supported := lang.Supported()
	if len(supported) != 9 {
		t.Errorf("expected 9 supported languages, got %d", len(supported))
	}

	for _, code := range supported {
		upperCode := strings.ToUpper(code)
		if out := lang.OutLang(code); out != upperCode {
			t.Errorf("expected OutLang(%q) to be %q, got %q", code, upperCode, out)
		}
	}

	supported[0] = "xx"
	if lang.Supported()[0] == "xx" {
		t.Error("Supported() must return a fresh slice on every call")
	}
}
