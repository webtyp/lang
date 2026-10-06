//go:build wasm

package lang

// WASM is single-threaded: no mutex needed
func setDefaultLang(l lang) {
	defLang = l
}

func getCurrentLang() lang {
	if !explicitlySet {
		defLang = resolveDefaultLang()
		explicitlySet = true
	}
	return defLang
}
