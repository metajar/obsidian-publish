package markdown

import (
	"strings"
	"testing"
)

func TestGFMFeatures(t *testing.T) {
	src := `| a | b |
| - | - |
| 1 | 2 |

~~struck~~

- [ ] todo
- [x] done

https://example.com/auto
`
	html, err := Fragment(src)
	if err != nil {
		t.Fatalf("Fragment: %v", err)
	}
	for _, want := range []string{"<table>", "<del>struck</del>", `disabled`, "todo", `<a href="https://example.com/auto"`} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered HTML missing %q:\n%s", want, html)
		}
	}
}

func TestRawHTMLNotRendered(t *testing.T) {
	html, err := Fragment("hello <script>alert(1)</script> <img src=x onerror=alert(1)>")
	if err != nil {
		t.Fatalf("Fragment: %v", err)
	}
	if strings.Contains(html, "<script>") || strings.Contains(html, "onerror") {
		t.Errorf("raw HTML leaked into output:\n%s", html)
	}
}

func TestDocumentStructure(t *testing.T) {
	doc, err := Document("My <Title> & Such", "# Heading\n\nbody")
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	for _, want := range []string{"<!DOCTYPE html>", "<title>My &lt;Title&gt; &amp; Such</title>", "<h1>Heading</h1>", "body", "</html>"} {
		if !strings.Contains(doc, want) {
			t.Errorf("document missing %q", want)
		}
	}
}
