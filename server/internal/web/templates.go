package web

import (
	"embed"
	"fmt"
	"html/template"
)

//go:embed templates/*.html
var templatesFS embed.FS

// Each page composes with layout.html. We pre-build one template per page
// (parsing layout + page together) so that page-level {{define "title"}}
// and {{define "content"}} blocks don't collide across pages.
func mustLoadTemplates() map[string]*template.Template {
	pages := []string{"login.html", "setup.html", "dashboard.html"}
	tmpls := make(map[string]*template.Template, len(pages))
	for _, p := range pages {
		t, err := template.ParseFS(templatesFS, "templates/layout.html", "templates/"+p)
		if err != nil {
			panic(fmt.Errorf("parse template %s: %w", p, err))
		}
		tmpls[p] = t
	}
	return tmpls
}
