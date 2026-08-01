package jobs

import (
	"strings"
	"testing"
)

func TestDistillHTMLPreservesMetadataAndResolvesURLs(t *testing.T) {
	html := `<!doctype html><html><head>
<title>Example Manga</title>
<meta property="og:type" content="book">
<script type="application/ld+json">{"name":"Example Manga"}</script>
<script>window.noise = true</script></head><body>
<nav>menu</nav><main><h2>Chapters</h2><a href="/chapter/1">Chapter 1</a></main>
</body></html>`

	result := distillHTML(html, "https://example.test/manga/title")
	for _, expected := range []string{
		"# Document metadata", "title: Example Manga", "og:type: book",
		`json-ld: {"name":"Example Manga"}`, "https://example.test/chapter/1",
	} {
		if !strings.Contains(result, expected) {
			t.Errorf("distilled content missing %q:\n%s", expected, result)
		}
	}
	for _, unwanted := range []string{"window.noise", "menu"} {
		if strings.Contains(result, unwanted) {
			t.Errorf("distilled content retained noise %q", unwanted)
		}
	}
}
