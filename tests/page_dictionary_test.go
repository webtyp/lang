//go:build wasm

package lang_test

import (
	"syscall/js"
	"testing"

	"webtyp.com/fmt"
	"webtyp.com/lang"
)

func TestMain(m *testing.M) {
	// A lookup before the page carries the dictionary (as a package init or
	// a consumer's TestMain may do) must not disable translations for good.
	_ = lang.Translate("Delete").String()

	doc := js.Global().Get("document")
	if !doc.IsUndefined() {
		script := doc.Call("createElement", "script")
		script.Set("type", "application/json")
		script.Set("id", lang.ScriptID)
		payload := `{"default":"es","languages":["es","fr"],"keys":{"Delete":["Eliminar","Supprimer"],"This":["Este","Ce"],"action":["acción","action"],"Pick a conversation":["Elige una conversación","Choisissez une conversation"],"name":["nombre","nom"],"required":["requerido","requis"],"Send":["",""]}}`
		script.Set("textContent", payload)
		doc.Get("head").Call("appendChild", script)
	}

	m.Run()
}

func TestPageDictionaryLoading(t *testing.T) {
	t.Run("basic lookup in ES", func(t *testing.T) {
		got := lang.Translate("es", "Delete").String()
		if got != "Eliminar" {
			t.Errorf("expected 'Eliminar', got %q", got)
		}
	})

	t.Run("lang.Text lookup in ES", func(t *testing.T) {
		got := lang.Translate("es", lang.Text("Delete")).String()
		if got != "Eliminar" {
			t.Errorf("expected 'Eliminar', got %q", got)
		}
	})

	t.Run("separate arguments are separate keys", func(t *testing.T) {
		got := lang.Translate("es", "This", "action").String()
		if got != "Este acción" {
			t.Errorf("expected 'Este acción', got %q", got)
		}
	})

	t.Run("empty slot passes through as English", func(t *testing.T) {
		if got := lang.Translate("es", "Send").String(); got != "Send" {
			t.Errorf("expected 'Send', got %q", got)
		}
	})

	t.Run("unit phrase lookup", func(t *testing.T) {
		got := lang.Translate("es", "Pick a conversation").String()
		if got != "Elige una conversación" {
			t.Errorf("expected 'Elige una conversación', got %q", got)
		}
	})

	t.Run("no key returns English", func(t *testing.T) {
		got := lang.Translate("es", "Pick a chat").String()
		if got != "Pick a chat" {
			t.Errorf("expected 'Pick a chat', got %q", got)
		}
	})

	t.Run("fmt.Err translates correctly", func(t *testing.T) {
		lang.OutLang("es")
		err := fmt.Err("name", "required").Error()
		if err != "nombre requerido" {
			t.Errorf("expected 'nombre requerido', got %q", err)
		}
	})

	t.Run("browser language not translated falls back to the page default", func(t *testing.T) {
		nav := js.Global().Get("navigator").Get("language").String()
		if nav == "es" || nav == "fr" || len(nav) > 1 && (nav[:2] == "es" || nav[:2] == "fr") {
			t.Skip("browser language is in the page's languages:", nav)
		}
		if got := lang.OutLang(); got != "ES" {
			t.Errorf("OutLang() = %q, want the page default ES (browser language %q is not translated)", got, nav)
		}
	})
}
