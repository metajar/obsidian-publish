// Package markdown renders page Markdown to a complete, self-contained HTML
// document at publish time (renders are stored, not computed per request).
//
// Rendering uses goldmark with CommonMark + GFM extensions (tables,
// strikethrough, task lists, autolinks). Raw HTML in notes is NOT rendered
// (goldmark's unsafe mode stays off), so published pages cannot inject
// scripts into readers' browsers.
package markdown

import (
	"bytes"
	"fmt"
	"html"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
)

// defaultCSS is the minimal, readable default typography, used whenever no
// global theme or per-page theme override is set. A custom theme replaces it
// entirely — the owner owns the result (except the password-entry form, which
// always carries its own stylesheet).
const defaultCSS = `
:root { color-scheme: light dark; }
* { box-sizing: border-box; }
body {
  margin: 0 auto;
  padding: 2rem 1.25rem 4rem;
  max-width: 42rem;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  font-size: 1.05rem;
  line-height: 1.7;
}
h1, h2, h3, h4, h5, h6 { line-height: 1.25; margin-top: 2rem; }
h1:first-of-type { margin-top: 0.5rem; }
a { color: #0b6bcb; }
img { max-width: 100%; }
code, pre { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 0.9em; }
code { background: rgba(127,127,127,.15); padding: 0.15em 0.35em; border-radius: 3px; }
pre { padding: 1rem 1.25rem; overflow-x: auto; border-radius: 6px; background: rgba(127,127,127,.12); }
pre code { background: none; padding: 0; }
blockquote { margin: 1.5rem 0; padding: 0.25rem 1.25rem; border-left: 4px solid rgba(127,127,127,.4); color: rgba(128,128,128,1); }
table { border-collapse: collapse; margin: 1.5rem 0; }
th, td { border: 1px solid rgba(127,127,127,.5); padding: 0.4rem 0.75rem; text-align: left; }
th { background: rgba(127,127,127,.12); }
ul.contains-task-list { list-style: none; padding-left: 0.5rem; }
`

// Fragment renders Markdown to an HTML body fragment (no <html> wrapper).
func Fragment(markdown string) (string, error) {
	var buf bytes.Buffer
	if err := md.Convert([]byte(markdown), &buf); err != nil {
		return "", fmt.Errorf("markdown: convert: %w", err)
	}
	return buf.String(), nil
}

// Document renders Markdown into a complete HTML document titled title,
// embedding css as the page stylesheet. An empty css means the built-in
// default; a non-empty css replaces the default entirely.
func Document(title, markdown, css string) (string, error) {
	body, err := Fragment(markdown)
	if err != nil {
		return "", err
	}
	if css == "" {
		css = defaultCSS
	}
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	b.WriteString("<title>")
	b.WriteString(html.EscapeString(title))
	b.WriteString("</title>\n<style>")
	b.WriteString(css)
	b.WriteString("\n</style>\n</head>\n<body>\n<article>\n")
	b.WriteString(body)
	b.WriteString("\n</article>\n</body>\n</html>\n")
	return b.String(), nil
}
