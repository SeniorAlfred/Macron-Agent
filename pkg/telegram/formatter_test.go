package telegram

import (
	"strings"
	"testing"
)

func TestEscapeHTML(t *testing.T) {
	if got := EscapeHTML(`<b>a & b</b>`); got != "&lt;b&gt;a &amp; b&lt;/b&gt;" {
		t.Fatalf("EscapeHTML: %q", got)
	}
}

func TestMarkdownToHTMLKeepsTheBasics(t *testing.T) {
	html := MarkdownToHTML("# Title\n\n**bold** and `code` and [link](https://example.com)")
	for _, want := range []string{"<b>", "bold", "<code>code</code>", "https://example.com"} {
		if !strings.Contains(html, want) {
			t.Fatalf("expected %q in %q", want, html)
		}
	}
	if !ValidHTML(html) {
		t.Fatalf("the formatter produced invalid HTML: %q", html)
	}
}

func TestSplitHTMLStaysUnderTheLimit(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		sb.WriteString("یک خط فارسی برای آزمون تقسیم پیام\n")
	}
	html := MarkdownToHTML(sb.String())
	parts := SplitHTML(html, 300)
	if len(parts) < 2 {
		t.Fatalf("a long message should be split, got %d parts", len(parts))
	}
	for _, p := range parts {
		if n := VisibleRunes(p); n > 320 {
			t.Fatalf("a part is too long: %d runes", n)
		}
		if !ValidHTML(p) {
			t.Fatalf("a part is broken HTML: %q", p)
		}
	}
}

func TestRTLDetection(t *testing.T) {
	if !IsMostlyRTL("سلام خوبی؟ این یک پیام فارسی است") {
		t.Fatal("a Persian sentence should be detected as RTL")
	}
	if IsMostlyRTL("this is an English sentence") {
		t.Fatal("an English sentence is not RTL")
	}
	marked := MarkRTL("سلام hello")
	if UnmarkRTL(marked) != "سلام hello" {
		t.Fatalf("the RTL marks left something behind: %q", UnmarkRTL(marked))
	}
}

func TestLookupHelpers(t *testing.T) {
	if dc := FileDC(""); dc != 0 {
		t.Fatalf("an empty file id has no data centre, got %d", dc)
	}
	if got := DCLocation(2); got == "" {
		t.Fatal("every known data centre should have a location")
	}
	if got := DCLocation(999); got != "" {
		t.Fatal("an unknown data centre should still say something")
	}
}

func TestFoldLongKeepsItReadable(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 40; i++ {
		sb.WriteString("## بخش\n\nمتن طولانی فارسی برای آزمون جمع‌شدن پیام.\n\n")
	}
	long := sb.String()
	out := FoldLong(long, FoldOptions{})
	if out == "" {
		t.Fatal("FoldLong returned nothing")
	}
	if VisibleRunes(out) >= VisibleRunes(long) {
		t.Fatalf("a long document should shrink: %d >= %d", VisibleRunes(out), VisibleRunes(long))
	}
	if !strings.Contains(out, "##") {
		t.Fatalf("the headings should survive: %q", out[:min(len(out), 120)])
	}
}
