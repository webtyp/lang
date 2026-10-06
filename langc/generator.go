package langc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"webtyp.com/modfind"
	wfilepath "webtyp.com/filepath"
)

// Translations keeps a project's config/lang.json (or a library's lang.json) in
// step with the texts its code uses, and builds the dictionary the client reads.
type Translations struct {
	modules modfind.Discoverer
	log     func(...any)
}

// New creates a new Translations instance.
func New(modules modfind.Discoverer, log func(...any)) *Translations {
	return &Translations{
		modules: modules,
		log:     log,
	}
}

// langFile represents the structure of the JSON file
type langFile struct {
	Default   string              `json:"default,omitempty"`
	Languages []string            `json:"languages"`
	Keys      map[string][]string `json:"keys"`
}

// Missing is one untranslated key in one language, with the modules that use it.
type Missing struct {
	Key      string
	Language string
	Modules  []string
}

func (t *Translations) SyncTranslations(rootDir string) error {
	isProject := false
	if info, err := os.Stat(filepath.Join(rootDir, "config")); err == nil && info.IsDir() {
		isProject = true
	}

	langFilePath := filepath.Join(rootDir, "lang.json")
	if isProject {
		langFilePath = filepath.Join(rootDir, "config", "lang.json")
	}

	var fileData langFile
	fileBytes, err := os.ReadFile(langFilePath)
	if os.IsNotExist(err) {
		fileData = langFile{
			Languages: []string{"es"},
			Keys:      make(map[string][]string),
		}
		if isProject {
			fileData.Default = "es"
		}
	} else if err != nil {
		return err
	} else {
		if err := json.Unmarshal(fileBytes, &fileData); err != nil {
			return err
		}
	}

	// Validations
	if err := validateLanguages(langFilePath, fileData.Languages, fileData.Keys); err != nil {
		return err
	}

	// Discovery
	var mods []modfind.Module
	if isProject {
		foundMods, err := t.modules.Discover(rootDir)
		if err != nil {
			return err
		}
		for _, m := range foundMods {
			hasWebtyp := false
			if m.IsMain {
				hasWebtyp = true
			} else {
				modBytes, _ := os.ReadFile(filepath.Join(m.SourceDir(), "go.mod"))
				if strings.Contains(string(modBytes), "webtyp.com/") {
					hasWebtyp = true
				}
			}
			if hasWebtyp {
				mods = append(mods, m)
			}
		}
	} else {
		mod := modfind.Module{Path: "library", IsMain: true}
		// Stub out the SourceDir for testing/library root mode. We shouldn't need a real mock SourceDir right now.
		mods = []modfind.Module{mod}
	}

	keysUsed := make(map[string]map[string]bool)
	textFields := make(map[string]map[string]bool)

	for _, mod := range mods {
		scanPass1(mod, textFields)
	}
	for _, mod := range mods {
		scanPass2(mod, textFields, keysUsed)
	}

	mergedKeys := make(map[string][]string)
	for key := range keysUsed {
		if fileVals, exists := fileData.Keys[key]; exists {
			mergedKeys[key] = append([]string{}, fileVals...)
			for len(mergedKeys[key]) < len(fileData.Languages) {
				mergedKeys[key] = append(mergedKeys[key], "")
			}
		} else {
			mergedKeys[key] = make([]string, len(fileData.Languages))
		}
	}

	fileData.Keys = mergedKeys

	newBytes, err := formatLangFile(fileData)
	if err != nil {
		return err
	}

	if !bytes.Equal(fileBytes, newBytes) {
		if err := os.WriteFile(langFilePath, newBytes, 0644); err != nil {
			return err
		}
	}

	missingCount := make(map[string]int)
	for _, vals := range mergedKeys {
		for i, val := range vals {
			if val == "" {
				missingCount[fileData.Languages[i]]++
			}
		}
	}
	for _, code := range fileData.Languages {
		if c := missingCount[code]; c > 0 {
			t.log(fmt.Sprintf("lang: %d untranslated (%s) — see %s", c, code, wfilepath.Tilde(langFilePath)))
		}
	}

	return nil
}

func validateLanguages(path string, langs []string, keys map[string][]string) error {
	seen := make(map[string]bool)
	valid := map[string]bool{"en":true,"es":true,"zh":true,"hi":true,"ar":true,"pt":true,"fr":true,"de":true,"ru":true}

	for _, l := range langs {
		if !valid[l] {
			return fmt.Errorf("lang: %s: unknown language code %q", path, l)
		}
		if seen[l] {
			return fmt.Errorf("lang: %s: duplicate language code %q", path, l)
		}
		seen[l] = true
	}

	badKeys := 0
	sampleKey := ""
	for key, vals := range keys {
		if len(vals) > len(langs) {
			badKeys++
			sampleKey = key
		}
	}
	if badKeys > 0 {
		return fmt.Errorf("lang: %s: %d keys have more values than languages (e.g. %q) — fix languages or the lists", path, badKeys, sampleKey)
	}
	return nil
}

func formatLangFile(data langFile) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("{\n")
	if data.Default != "" {
		buf.WriteString(fmt.Sprintf("  \"default\": %q,\n", data.Default))
	}

	langBytes, _ := json.Marshal(data.Languages)
	buf.WriteString(fmt.Sprintf("  \"languages\": %s,\n", string(langBytes)))
	buf.WriteString("  \"keys\": {\n")

	var keys []string
	for k := range data.Keys {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for i, k := range keys {
		kBytes, _ := json.Marshal(k)
		vBytes, _ := json.Marshal(data.Keys[k])

		buf.WriteString(fmt.Sprintf("    %s: %s", string(kBytes), string(vBytes)))
		if i < len(keys)-1 {
			buf.WriteString(",")
		}
		buf.WriteString("\n")
	}
	buf.WriteString("  }\n}\n")
	return buf.Bytes(), nil
}

// scan passes stubs
func scanPass1(mod modfind.Module, textFields map[string]map[string]bool) {}
func scanPass2(mod modfind.Module, textFields map[string]map[string]bool, keysUsed map[string]map[string]bool) {}

func (t *Translations) BundleTranslations(rootDir string) (string, error) {
	return "", nil
}

func (t *Translations) MissingTranslations(rootDir string) ([]Missing, error) {
	return nil, nil
}
