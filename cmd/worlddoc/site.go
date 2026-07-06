package main

import (
	_ "embed"
	"fmt"
	"html"
	"html/template"
	"strings"
)

//go:embed site.css
var siteCSS string

// siteSections is the fixed sidebar order: key (active marker), label, and the
// per-pack file it links to.
var siteSections = []struct{ key, label, file string }{
	{"overview", "Overview", "index.html"},
	{"map", "Map", "map.html"},
	{"gazetteer", "Gazetteer", "gazetteer.html"},
	{"catalogs", "Catalogs", "catalogs.html"},
	{"health", "Health", "health.html"},
	{"guide", "Guide", "guide.html"},
}

// siteNavPacks is the set of world packs shown in the sidebar's pack switcher.
// The driver sets it (from discovery) before rendering so the switcher lists
// every pack even on a single-pack run.
var siteNavPacks []string

var pageTmpl = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} · {{.Pack}}</title>
<style>{{.CSS}}</style>
</head>
<body>
{{.Nav}}
<main>
<div class="page-head"><h1>{{.Title}}</h1>{{if .Lede}}<p class="lede">{{.Lede}}</p>{{end}}</div>
{{.Body}}
</main>
</body>
</html>
`))

type pageData struct {
	Title string
	Pack  string
	Lede  string
	CSS   template.CSS
	Nav   template.HTML
	Body  template.HTML
}

// renderPage wraps a pre-built HTML body in the shared shell (sidebar + head).
func renderPage(pack, active, title, lede, body string) (string, error) {
	var buf strings.Builder
	err := pageTmpl.Execute(&buf, pageData{
		Title: title,
		Pack:  pack,
		Lede:  lede,
		CSS:   template.CSS(siteCSS),
		Nav:   template.HTML(renderNav(pack, active)),
		Body:  template.HTML(body),
	})
	if err != nil {
		return "", fmt.Errorf("rendering page shell: %w", err)
	}
	return buf.String(), nil
}

func renderNav(pack, active string) string {
	var b strings.Builder
	b.WriteString(`<div class="sidebar">`)
	fmt.Fprintf(&b, `<div class="brand"><h1>%s</h1><small>World Docs</small></div>`, esc(pack))
	b.WriteString(`<nav>`)
	for _, s := range siteSections {
		cls := ""
		if s.key == active {
			cls = ` class="active"`
		}
		fmt.Fprintf(&b, `<a%s href="%s">%s</a>`, cls, s.file, esc(s.label))
	}
	b.WriteString(`</nav>`)
	if len(siteNavPacks) > 1 {
		activeFile := "index.html"
		for _, s := range siteSections {
			if s.key == active {
				activeFile = s.file
			}
		}
		b.WriteString(`<span class="lbl">Packs</span><nav>`)
		for _, p := range siteNavPacks {
			if p == pack {
				fmt.Fprintf(&b, `<a class="current">%s</a>`, esc(p))
			} else {
				fmt.Fprintf(&b, `<a href="../%s/%s">%s</a>`, esc(p), activeFile, esc(p))
			}
		}
		b.WriteString(`</nav>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// --- HTML fragment helpers (content is escaped; callers assemble trusted HTML) ---

func esc(s string) string { return html.EscapeString(s) }

// escName strips the engine's color markup then escapes — for display names.
func escName(s string) string { return html.EscapeString(clean(s)) }

// codeID renders an id as inline code.
func codeID(s string) string { return `<code>` + esc(s) + `</code>` }

// tag renders a colored pill. label is escaped; class is written raw into the
// attribute, so callers must pass a static literal class — never content-derived
// text.
func tag(class, label string) string {
	return fmt.Sprintf(`<span class="tag %s">%s</span>`, class, esc(label))
}

// htmlTable renders a table; header cells are plain text, row cells are trusted
// HTML fragments (callers escape their content). Empty cells render as a dash.
func htmlTable(headers []string, rows [][]string) string {
	var b strings.Builder
	b.WriteString("<table><thead><tr>")
	for _, h := range headers {
		fmt.Fprintf(&b, "<th>%s</th>", esc(h))
	}
	b.WriteString("</tr></thead><tbody>")
	for _, r := range rows {
		b.WriteString("<tr>")
		for _, c := range r {
			if c == "" {
				c = "—"
			}
			fmt.Fprintf(&b, "<td>%s</td>", c)
		}
		b.WriteString("</tr>")
	}
	b.WriteString("</tbody></table>")
	return b.String()
}
