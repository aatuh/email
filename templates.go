package email

import (
	"fmt"
	htmltmpl "html/template"
	"io/fs"
	"strings"
	texttmpl "text/template"
)

// TemplateSet loads and renders text and HTML templates from an fs.FS.
//
// Convention:
//
//	name.txt.tmpl  -> plain text body
//	name.html.tmpl -> HTML body
//
// Both files are optional; at least one must exist to render a message.
type TemplateSet struct {
	texts *texttmpl.Template
	htmls *htmltmpl.Template
}

// MustLoadTemplates panics on error; useful for init.
//
// Parameters:
//   - fsys: The filesystem.
//
// Returns:
//   - *TemplateSet: The template set.
func MustLoadTemplates(fsys fs.FS) *TemplateSet {
	ts, err := LoadTemplates(fsys)
	if err != nil {
		panic(err)
	}
	return ts
}

// LoadTemplates walks fsys and parses *.txt.tmpl and *.html.tmpl.
//
// Parameters:
//   - fsys: The filesystem.
//
// Returns:
//   - *TemplateSet: The template set.
//   - error: The error if the template set fails to load.
func LoadTemplates(fsys fs.FS) (*TemplateSet, error) {
	textRoot := texttmpl.New("text")
	htmlRoot := htmltmpl.New("html")
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		lower := strings.ToLower(path)
		switch {
		case strings.HasSuffix(lower, ".txt.tmpl"):
			b, rerr := fs.ReadFile(fsys, path)
			if rerr != nil {
				return rerr
			}
			_, perr := textRoot.New(path).Parse(string(b))
			return perr
		case strings.HasSuffix(lower, ".html.tmpl"):
			b, rerr := fs.ReadFile(fsys, path)
			if rerr != nil {
				return rerr
			}
			_, perr := htmlRoot.New(path).Parse(string(b))
			return perr
		default:
			return nil
		}
	})
	if err != nil {
		return nil, err
	}
	return &TemplateSet{texts: textRoot, htmls: htmlRoot}, nil
}

// Render renders "name" by locating "name.txt.tmpl" and "name.html.tmpl"
// anywhere in the parsed set. If only one exists, the other return is nil.
//
// Parameters:
//   - name: The name of the template.
//   - data: The data to render the template with.
//
// Returns:
//   - []byte: The plain text body.
//   - []byte: The HTML body.
//   - error: The error if the template fails to render.
func (t *TemplateSet) Render(name string, data any) ([]byte, []byte, error) {
	var plain, html []byte
	txtName := name + ".txt.tmpl"
	htmlName := name + ".html.tmpl"

	if tmpl := t.texts.Lookup(txtName); tmpl != nil {
		var b strings.Builder
		if err := tmpl.Execute(&b, data); err != nil {
			return nil, nil, fmt.Errorf("render text: %w", err)
		}
		plain = []byte(b.String())
	}

	if tmpl := t.htmls.Lookup(htmlName); tmpl != nil {
		var b strings.Builder
		if err := tmpl.Execute(&b, data); err != nil {
			return nil, nil, fmt.Errorf("render html: %w", err)
		}
		html = []byte(b.String())
	}

	if plain == nil && html == nil {
		return nil, nil, fmt.Errorf("template %q not found", name)
	}
	return plain, html, nil
}
