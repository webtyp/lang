//go:build wasm

package lang

var explicitlySet bool

// WASM is single-threaded: no mutex needed
func setDefaultLang(l lang) {
	defLang = l
	explicitlySet = true
}

func getCurrentLang() lang {
	if !explicitlySet {
		defLang = resolveDefaultLang()
		explicitlySet = true
	}
	return defLang
}
