package i18n

import (
	"bytes"
	"html/template"
	"testing"
)

func TestFuncMapT(t *testing.T) {
	tpl := template.Must(template.New("x").Funcs(FuncMap("es")).Parse(`{{t "auth.login"}}`))
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, nil); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := buf.String(); got != "Iniciar sesión" {
		t.Errorf("rendered = %q, want %q", got, "Iniciar sesión")
	}
}

func TestFuncMapTf(t *testing.T) {
	tpl := template.Must(template.New("x").Funcs(FuncMap("en")).Parse(`{{tf "items_count.other" "count" "9"}}`))
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, nil); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := buf.String(); got != "9 items" {
		t.Errorf("rendered = %q, want %q", got, "9 items")
	}
}

func TestFuncMapTp(t *testing.T) {
	tpl := template.Must(template.New("x").Funcs(FuncMap("en")).Parse(`{{tp "items_count" 1}}|{{tp "items_count" 4}}`))
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, nil); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := buf.String(), "1 item|4 items"; got != want {
		t.Errorf("rendered = %q, want %q", got, want)
	}
}
