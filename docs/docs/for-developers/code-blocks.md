# Code Blocks in Email View

Matcha renders fenced code blocks inside emails as styled terminal boxes with a language label and syntax highlighting. This works for both Markdown-formatted plain-text emails and HTML emails containing `<pre><code>` blocks.

## How It Looks

When you open an email containing a fenced code block, Matcha renders it in a rounded box. The language is shown as a highlighted label above the code, and the code itself is colorized:

```
 ╭──────────────────────────────────╮
 │  GO                              │
 │ func main() {                    │
 │     fmt.Println("hello")         │
 │     return 42                    │
 │ }                                │
 ╰──────────────────────────────────╯
```

Colors used for highlighting:

| Token type   | Example                 | Color source              |
|--------------|-------------------------|---------------------------|
| Keywords     | `func`, `return`, `if`  | Theme accent (bold)       |
| Strings      | `"hello"`, `'a'`        | Warm yellow `#E5C07B`     |
| Comments     | `// ...`, `/* ... */`   | Theme secondary (italic)  |
| Numbers      | `42`, `0xFF`            | Orange `#D19A66`          |
| Functions    | `main`, `Println`       | Blue `#61AFEF`            |
| Types        | `string`, `MyStruct`    | Cyan `#56B6C2`            |
| Punctuation  | `{ } ( ) ; ,`           | Theme subtle text         |
| Constants    | `true`, `false`, `nil`  | Purple `#C678DD`          |

## Supported Languages

The highlighter is a lightweight pure-Go regex tokenizer (no external dependencies) in `view/highlight.go`. It recognizes the following languages and common aliases:

| Language   | Aliases                                  |
|------------|------------------------------------------|
| Go         | `go`                                     |
| Python     | `python`, `py`                           |
| JavaScript | `javascript`, `js`, `jsx`                |
| TypeScript | `typescript`, `ts`, `tsx`                |
| Rust       | `rust`, `rs`                             |
| C / C++    | `c`, `cpp`, `c++`, `cc`, `cxx`, `h`, `hpp` |
| Java       | `java`, `kotlin`, `kt`, `scala`, `groovy` |
| Ruby       | `ruby`, `rb`                             |
| Bash       | `bash`, `sh`, `shell`, `zsh`             |
| HTML/XML   | `html`, `xml`, `svg`                     |
| CSS        | `css`, `scss`, `less`                    |
| JSON       | `json`                                   |
| YAML       | `yaml`, `yml`                            |
| SQL        | `sql`                                    |
| Markdown   | `markdown`, `md`                         |

Code blocks with an unrecognized or missing language are rendered in the same bordered box **without** a language label and **without** syntax highlighting — the raw text is preserved as-is.

## How It Works

The feature spans four layers of the codebase:

### 1. HTML Sanitizer (`internal/htmlsanitizer`)

The sanitizer policy was extended to allow the `class` attribute on `<pre>` and `<code>` elements, but only when it matches the pattern `language-[a-z0-9+#.]+`. This lets the language hint through while blocking any other class-based attacks.

### 2. C HTML Parser (`clib/htmlconv.c`)

A new element type `HELEM_CODE` (value `7`) was added to the parser. When the parser encounters a `<pre>` tag, it captures all text content into a dedicated buffer (preserving whitespace exactly) and emits an `HELEM_CODE` element on `</pre>`. The language is extracted from a `class="language-XXX"` attribute on either the `<pre>` or the inner `<code>` tag (md4c puts it on `<code>`).

The pure-Go fallback parser (`clib/htmlconv_nocgo.go`) was updated to emit `HELEM_CODE` for `<pre>` blocks via goquery.

### 3. Syntax Highlighter (`view/highlight.go`)

A self-contained regex-based tokenizer that:

1. Looks up rules for the language via `languageRules()` — each rule is a compiled regex paired with a `TokenKind`.
2. Finds all matches across all rules and resolves overlaps so earlier (higher-priority) rules win byte-by-byte.
3. Renders each contiguous run with a lipgloss style derived from the active theme.

Go's `regexp` package uses RE2, which does **not** support lookaheads (`(?=`). Function-call detection (e.g. highlighting `main` in `main()`) is handled with a capturing-group rule that matches `name(` and highlights only group 1 (the name), via the `rule.group` field.

### 4. Rendering (`view/html.go`)

The `renderHTMLToText` function handles `clib.HElemCode` by calling `renderCodeBlock()`, which:

- Trims leading/trailing blank lines (preserving internal indentation).
- Calls `highlightCode()` for colorization.
- Renders the language as an uppercase label with `codeLangStyle()`.
- Wraps everything in a rounded box via `codeBoxStyle()`.

## Markdown Fenced Code Blocks

Plain-text emails go through md4c (`clib/markdown.go`) which converts fenced code blocks:

````markdown
```go
func main() {
    fmt.Println("hello")
}
```
````

into `<pre><code class="language-go">…</code></pre>`, which then flows through the sanitizer and C parser as described above.

## Adding a New Language

To add support for a new language, add a `case` to the `switch` in `languageRules()` in `view/highlight.go`:

```go
case "php":
    return []rule{
        mustRule(`\/\/[^\n]*|\/\*[\s\S]*?\*\/`, tokComment),
        mustRule(`"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'`, tokString),
        mustRule(`\b(abstract|and|array|as|break|callable|case|catch|class|clone|const|continue|declare|default|do|echo|else|elseif|empty|enddeclare|endfor|endforeach|endif|endswitch|endwhile|extends|final|finally|fn|for|foreach|function|global|if|implements|include|include_once|instanceof|insteadof|interface|isset|list|match|namespace|new|or|print|private|protected|public|readonly|require|require_once|return|static|switch|throw|trait|try|unset|use|var|while|xor|yield)\b`, tokKeyword),
        mustRule(`\b(true|false|null)\b`, tokConstant),
        mustRule(`\b[A-Z][A-Za-z0-9_]*\b`, tokType),
        mustRule(`\b[0-9]+(\.[0-9]+)?\b`, tokNumber),
        funcRule(),
        mustRule(`[{}()\[\];,:.<>=+\-*/%&|^!@?]`, tokPunctuation),
    }
```

If the language has a common alias, also add it to `normalizeLang()`.

## Testing

Tests live in `view/highlight_test.go` and cover:

- Per-language highlighting (all 15 supported languages).
- Unknown language and empty language fallbacks.
- Language alias resolution.
- Full `ProcessBody` pipeline for Markdown and HTML code blocks.
- Multiple code blocks in a single email.
- The HTML sanitizer's `class` attribute handling (`internal/htmlsanitizer`).

Run them with:

```bash
go test ./view/ -run 'TestHighlight|TestRenderCodeBlock|TestProcessBody.*Code|TestNormalizeLang' -v
```
