package lang_test

import (
	"os"
	"path/filepath"
	"testing"
	"webtyp.com/lang/langc"
	"webtyp.com/modfind"
)

type fakeModules struct {
	mods []modfind.Module
}

func (f fakeModules) Discover(rootDir string) ([]modfind.Module, error) {
	return f.mods, nil
}

func TestLangcSync(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "langc-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create minimal config layout for a project
	projDir := filepath.Join(tempDir, "project")
	os.MkdirAll(filepath.Join(projDir, "config"), 0755)

	logOutput := []string{}
	logFn := func(args ...any) {
		logOutput = append(logOutput, "logged")
	}

	// Module API has SourceDir as func
	mod := modfind.Module{Path: "webtyp.com/testmod", IsMain: true}
	mods := fakeModules{
		mods: []modfind.Module{mod},
	}

	translator := langc.New(mods, logFn)
	err = translator.SyncTranslations(projDir)
	if err != nil {
		t.Fatalf("SyncTranslations failed: %v", err)
	}

	// Verify it created config/lang.json
	jsonBytes, err := os.ReadFile(filepath.Join(projDir, "config", "lang.json"))
	if err != nil {
		t.Fatalf("Expected config/lang.json to be created")
	}

	jsonStr := string(jsonBytes)
	if !contains(jsonStr, "\"default\": \"es\"") {
		t.Errorf("Expected 'default' to be 'es'")
	}
	if !contains(jsonStr, "\"languages\": [\"es\"]") {
		t.Errorf("Expected languages to have 'es'")
	}
}

func contains(s, substr string) bool {
	// Simple helper
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
