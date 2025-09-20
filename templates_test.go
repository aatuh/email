package email

import (
    "testing"
    "testing/fstest"
)

func TestTemplatesRender(t *testing.T) {
    mfs := fstest.MapFS{
        "templates/welcome.txt.tmpl": {Data: []byte("Hi {{.Name}}")},
        "templates/welcome.html.tmpl": {Data: []byte("<b>{{.Name}}</b>")},
        "other/ignore.txt":           {Data: []byte("ignored")},
    }
    ts, err := LoadTemplates(mfs)
    if err != nil { t.Fatalf("load: %v", err) }

    p, h, err := ts.Render("templates/welcome", map[string]any{"Name": "Ada"})
    if err != nil { t.Fatalf("render: %v", err) }
    if string(p) != "Hi Ada" || string(h) != "<b>Ada</b>" {
        t.Fatalf("unexpected output: %q | %q", p, h)
    }

    // Missing template should error.
    if _, _, err := ts.Render("missing", nil); err == nil {
        t.Fatalf("expected error for missing template")
    }
}
