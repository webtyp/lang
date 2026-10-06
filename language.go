package lang

import "webtyp.com/fmt"

// ScriptID is the id of the <script type="application/json"> element in which
// the page carries the merged dictionary (written by langc, read here).
const ScriptID = "webtyp-lang"

// Private global configuration
var (
	defLang lang = EN
	explicitlySet bool
)

// Language enumeration for supported languages
type lang uint8

// String devuelve el nombre del lenguaje como string (ej: EN => "EN")
func (l lang) String() string {
	switch l {
	case EN:
		return "EN"
	case ES:
		return "ES"
	case ZH:
		return "ZH"
	case HI:
		return "HI"
	case AR:
		return "AR"
	case PT:
		return "PT"
	case FR:
		return "FR"
	case DE:
		return "DE"
	case RU:
		return "RU"
	default:
		return "EN" // fallback
	}
}

const (
	// Group 1: Core Essential Languages (Maximum Global Reach)
	EN lang = iota // 0 - English (default)
	ES             // 1 - Spanish
	ZH             // 2 - Chinese
	HI             // 3 - Hindi
	AR             // 4 - Arabic

	// Group 2: Extended Reach Languages (Europe & Americas)
	PT // 5 - Portuguese
	FR // 6 - French
	DE // 7 - German
	RU // 8 - Russian
)

func resolveDefaultLang() lang {
	loadFromPage()
	sysLang := getSystemLang()
	if pageSet {
		for _, pl := range pageLangs {
			if pl == sysLang {
				return sysLang
			}
		}
		return pageDef
	}
	return sysLang
}

// OutLang sets and returns the current output language as a string.
func OutLang(l ...any) string {
	if len(l) == 0 {
		l := resolveDefaultLang()
		setDefaultLang(l)
		return l.String()
	}

	var newLang lang
	switch v := l[0].(type) {
	case lang:
		newLang = v
	case string:
		newLang = langParser(v)
	default:
		return getCurrentLang().String()
	}

	setDefaultLang(newLang)
	explicitlySet = true
	return newLang.String()
}

// langParser processes language strings and returns the first valid language found.
func langParser(langStrings ...string) lang {
	for _, langStr := range langStrings {
		if langStr == "" {
			continue
		}

		// Use fmt.Split instead of c.splitStr
		parts := fmt.Split(langStr, ".")
		code := parts[0]
		parts = fmt.Split(code, "_")
		code = parts[0]
		parts = fmt.Split(code, "-")
		code = parts[0]

		if code == "" {
			continue
		}

		if l, ok := mapLangCode(code); ok {
			return l
		}
	}

	return EN
}

func mapLangCode(strVal string) (lang, bool) {
	// Use fmt.Convert(s).ToLower().String() instead of c.changeCase
	code := fmt.Convert(strVal).ToLower().String()
	switch code {
	case "en":
		return EN, true
	case "es":
		return ES, true
	case "zh":
		return ZH, true
	case "hi":
		return HI, true
	case "ar":
		return AR, true
	case "pt":
		return PT, true
	case "fr":
		return FR, true
	case "de":
		return DE, true
	case "ru":
		return RU, true
	}
	return EN, false
}
