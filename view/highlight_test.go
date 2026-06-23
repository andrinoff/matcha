package view

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/theme"
)

func TestHighlightCodeGo(t *testing.T) {
	theme.ActiveTheme = theme.Matcha
	code := "func main() {\n\tfmt.Println(\"hello\")\n\treturn 42\n}"
	out := highlightCode(code, "go")
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("expected ANSI escapes, got plain: %q", out)
	}
	if !strings.Contains(out, "func") {
		t.Errorf("output missing 'func'")
	}
}

func TestHighlightCodeMultipleLanguages(t *testing.T) {
	theme.ActiveTheme = theme.Matcha
	cases := []struct {
		lang string
		code string
		want string // a substring that should appear (after ANSI strip) in the output
	}{
		{"python", "def foo():\n    return 1", "def"},
		{"javascript", "function foo() { return 1; }", "function"},
		{"rust", "fn main() { let x = 1; }", "fn"},
		{"c", "int main() { return 0; }", "int"},
		{"java", "public class Foo {}", "public"},
		{"ruby", "def foo\n  1\nend", "def"},
		{"bash", "if [ -f x ]; then echo hi; fi", "echo"},
		{"sql", "SELECT * FROM users", "SELECT"},
		{"json", `{"key": "value"}`, "key"},
		{"yaml", "key: value\nnum: 42", "key"},
		{"html", `<div class="x">hi</div>`, "div"},
		{"css", ".x { color: red; }", "color"},
		{"markdown", "# Title\n\n**bold**", "Title"},
	}
	for _, tc := range cases {
		t.Run(tc.lang, func(t *testing.T) {
			out := highlightCode(tc.code, tc.lang)
			if !strings.Contains(out, "\x1b[") {
				t.Errorf("language %q: expected ANSI escapes, got plain: %q", tc.lang, out)
			}
			stripped := stripANSITest(out)
			if !strings.Contains(stripped, tc.want) {
				t.Errorf("language %q: output missing %q in %q", tc.lang, tc.want, stripped)
			}
		})
	}
}

func TestHighlightCodeUnknownLang(t *testing.T) {
	theme.ActiveTheme = theme.Matcha
	code := "some weird text"
	out := highlightCode(code, "klingon")
	if out != code {
		t.Errorf("unknown lang should return code unchanged, got %q", out)
	}
}

func TestHighlightCodeEmpty(t *testing.T) {
	out := highlightCode("", "go")
	if out != "" {
		t.Errorf("empty code should return empty, got %q", out)
	}
}

func TestHighlightCodeEmptyLang(t *testing.T) {
	theme.ActiveTheme = theme.Matcha
	code := "plain code without language"
	out := highlightCode(code, "")
	if out != code {
		t.Errorf("empty lang should return code unchanged, got %q", out)
	}
}

func TestHighlightCodeLangAlias(t *testing.T) {
	theme.ActiveTheme = theme.Matcha
	// "py" should resolve to "python" and produce highlighting
	out := highlightCode("def foo(): pass", "py")
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("alias 'py' should produce highlighting, got: %q", out)
	}
}

func TestNormalizeLang(t *testing.T) {
	cases := map[string]string{
		"Go":       "go",
		"Python":   "python",
		"py":       "python",
		"JS":       "javascript",
		"js":       "javascript",
		"ts":       "typescript",
		"sh":       "bash",
		"  Rust  ": "rust",
		"rs":       "rust",
		"rb":       "ruby",
		"yml":      "yaml",
		"":         "",
		"unknown":  "unknown",
	}
	for in, want := range cases {
		if got := normalizeLang(in); got != want {
			t.Errorf("normalizeLang(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderCodeBlockWithLang(t *testing.T) {
	theme.ActiveTheme = theme.Matcha
	out := renderCodeBlock("func main() {}", "go")
	stripped := stripANSITest(out)
	if !strings.Contains(stripped, "GO") {
		t.Errorf("code block with lang should show 'GO' label, got: %q", stripped)
	}
	if !strings.Contains(stripped, "func main() {}") {
		t.Errorf("code block should contain the code text, got: %q", stripped)
	}
}

func TestRenderCodeBlockWithoutLang(t *testing.T) {
	theme.ActiveTheme = theme.Matcha
	out := renderCodeBlock("plain text code", "")
	stripped := stripANSITest(out)
	if !strings.Contains(stripped, "plain text code") {
		t.Errorf("code block without lang should still contain code, got: %q", stripped)
	}
}

func TestRenderCodeBlockEmpty(t *testing.T) {
	out := renderCodeBlock("", "go")
	if out != "" {
		t.Errorf("empty code block should return empty string, got: %q", out)
	}
}

func TestProcessBodyMarkdownCodeBlock(t *testing.T) {
	theme.ActiveTheme = theme.Matcha
	body := "Intro text\n\n```go\nfunc main() {\n\treturn 42\n}\n```\n\nOutro text"
	h1 := lipgloss.NewStyle().Bold(true).Foreground(theme.ActiveTheme.Accent)
	h2 := lipgloss.NewStyle().Bold(true).Foreground(theme.ActiveTheme.Secondary)
	bodyStyle := lipgloss.NewStyle()

	out, placements, _, err := ProcessBody(body, BodyMIMETypePlain, h1, h2, bodyStyle, true)
	if err != nil {
		t.Fatalf("ProcessBody failed: %v", err)
	}
	if len(placements) != 0 {
		t.Errorf("expected no image placements, got %d", len(placements))
	}

	stripped := stripANSITest(out)
	if !strings.Contains(stripped, "GO") {
		t.Errorf("markdown code block should show 'GO' language label, got: %q", stripped)
	}
	if !strings.Contains(stripped, "func main()") {
		t.Errorf("markdown code block should contain the code, got: %q", stripped)
	}
	if !strings.Contains(stripped, "Intro text") {
		t.Errorf("text before code block should be preserved, got: %q", stripped)
	}
	if !strings.Contains(stripped, "Outro text") {
		t.Errorf("text after code block should be preserved, got: %q", stripped)
	}
}

func TestProcessBodyHTMLCodeBlock(t *testing.T) {
	theme.ActiveTheme = theme.Matcha
	htmlBody := `<p>Before</p><pre><code class="language-python">def hello():
    return 1</code></pre><p>After</p>`
	h1 := lipgloss.NewStyle().Bold(true).Foreground(theme.ActiveTheme.Accent)
	h2 := lipgloss.NewStyle().Bold(true).Foreground(theme.ActiveTheme.Secondary)
	bodyStyle := lipgloss.NewStyle()

	out, _, _, err := ProcessBody(htmlBody, BodyMIMETypeHTML, h1, h2, bodyStyle, true)
	if err != nil {
		t.Fatalf("ProcessBody failed: %v", err)
	}

	stripped := stripANSITest(out)
	if !strings.Contains(stripped, "PYTHON") {
		t.Errorf("HTML code block should show 'PYTHON' language label, got: %q", stripped)
	}
	if !strings.Contains(stripped, "def hello()") {
		t.Errorf("HTML code block should contain the code, got: %q", stripped)
	}
}

func TestProcessBodyCodeBlockNoLanguage(t *testing.T) {
	theme.ActiveTheme = theme.Matcha
	body := "Text\n\n```\nplain code\n```\n\nMore"
	h1 := lipgloss.NewStyle().Bold(true).Foreground(theme.ActiveTheme.Accent)
	h2 := lipgloss.NewStyle().Bold(true).Foreground(theme.ActiveTheme.Secondary)
	bodyStyle := lipgloss.NewStyle()

	out, _, _, err := ProcessBody(body, BodyMIMETypePlain, h1, h2, bodyStyle, true)
	if err != nil {
		t.Fatalf("ProcessBody failed: %v", err)
	}

	stripped := stripANSITest(out)
	if !strings.Contains(stripped, "plain code") {
		t.Errorf("code block without language should still contain code, got: %q", stripped)
	}
}

func TestProcessBodyMultipleCodeBlocks(t *testing.T) {
	theme.ActiveTheme = theme.Matcha
	body := "```go\nfunc a() {}\n```\n\nmiddle\n\n```python\ndef b(): pass\n```\n"
	h1 := lipgloss.NewStyle().Bold(true).Foreground(theme.ActiveTheme.Accent)
	h2 := lipgloss.NewStyle().Bold(true).Foreground(theme.ActiveTheme.Secondary)
	bodyStyle := lipgloss.NewStyle()

	out, _, _, err := ProcessBody(body, BodyMIMETypePlain, h1, h2, bodyStyle, true)
	if err != nil {
		t.Fatalf("ProcessBody failed: %v", err)
	}

	stripped := stripANSITest(out)
	if !strings.Contains(stripped, "GO") {
		t.Errorf("first code block should show 'GO' label, got: %q", stripped)
	}
	if !strings.Contains(stripped, "PYTHON") {
		t.Errorf("second code block should show 'PYTHON' label, got: %q", stripped)
	}
	if !strings.Contains(stripped, "middle") {
		t.Errorf("text between code blocks should be preserved, got: %q", stripped)
	}
}

// stripANSITest removes ANSI escape sequences from a string for test assertions.
func stripANSITest(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' {
			// Skip escape sequence: ESC [ ... letter
			i++
			for i < len(s) && s[i] != 'm' && s[i] != 'H' && s[i] != 'G' {
				i++
			}
			if i < len(s) {
				i++ // skip the terminating letter
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
