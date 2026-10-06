//go:build wasm

package lang

import (
	"syscall/js"
)

// getSystemLang detects browser language from navigator.language
func getSystemLang() lang {
	navigator := js.Global().Get("navigator")
	if navigator.IsUndefined() {
		return EN
	}

	language := navigator.Get("language")
	if language.IsUndefined() {
		return EN
	}

	return langParser(language.String())
}
