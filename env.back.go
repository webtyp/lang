//go:build !wasm

package lang

import (
	"os"
)

// getSystemLang detects system language from environment variables
func getSystemLang() lang {
	return langParser(
		os.Getenv("LANG"),
		os.Getenv("LANGUAGE"),
		os.Getenv("LC_ALL"),
		os.Getenv("LC_MESSAGES"),
	)
}
