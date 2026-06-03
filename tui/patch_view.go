package tui

import (
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	mailpatch "github.com/floatpane/go-mailpatch"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/gitmail"
	"github.com/floatpane/matcha/theme"
)

// detectPatch reconstructs a minimal RFC 5322 message from a plain-text email
// and parses it as a git format-patch. It returns the parsed patch and the raw
// bytes (for applying) when the message carries a diff; otherwise ok is false.
func detectPatch(e fetcher.Email) (patch *mailpatch.Patch, raw []byte, ok bool) {
	if e.BodyMIMEType == "text/html" {
		return nil, nil, false
	}
	raw = reconstructPatchMessage(e)
	parsed, err := mailpatch.ParseBytes(raw)
	if err != nil || !parsed.HasDiff() {
		return nil, nil, false
	}
	return parsed, raw, true
}

// reconstructPatchMessage rebuilds the headers go-mailpatch needs (From,
// Subject) around the raw body, so the subject's "[PATCH n/m]" prefix and the
// author are parsed alongside the diff.
func reconstructPatchMessage(e fetcher.Email) []byte {
	var b strings.Builder
	b.WriteString("From: ")
	b.WriteString(e.From)
	b.WriteString("\nSubject: ")
	b.WriteString(e.Subject)
	b.WriteString("\n\n")
	b.WriteString(e.Body)
	return []byte(b.String())
}

// renderPatch produces a colored, scrollable rendering of a parsed patch: a
// summary banner, the commit message, then each file's hunks with additions in
// green and deletions in red.
func renderPatch(p *mailpatch.Patch) string {
	add := lipgloss.NewStyle().Foreground(theme.ActiveTheme.Tip)
	del := lipgloss.NewStyle().Foreground(theme.ActiveTheme.Danger)
	hunkStyle := lipgloss.NewStyle().Foreground(theme.ActiveTheme.Accent)
	fileHdr := lipgloss.NewStyle().Bold(true).Foreground(theme.ActiveTheme.Accent)
	meta := lipgloss.NewStyle().Foreground(theme.ActiveTheme.MutedText)

	var b strings.Builder

	if p.Series.Total > 0 {
		fmt.Fprintf(&b, "%s\n", meta.Render(
			fmt.Sprintf("Patch %d/%d (v%d)", p.Series.Index, p.Series.Total, p.Series.Version)))
	}
	fmt.Fprintf(&b, "%s\n\n", meta.Render(
		fmt.Sprintf("%d file(s) changed, +%d -%d", p.Stat.FilesChanged, p.Stat.Additions, p.Stat.Deletions)))

	if msg := strings.TrimSpace(p.Body); msg != "" {
		b.WriteString(msg)
		b.WriteString("\n\n")
	}

	for _, f := range p.Files {
		fmt.Fprintf(&b, "%s\n", fileHdr.Render(fileHeading(f)))
		if f.IsBinary {
			b.WriteString(meta.Render("  (binary file)"))
			b.WriteString("\n\n")
			continue
		}
		for _, h := range f.Hunks {
			head := strings.TrimRight(fmt.Sprintf("@@ -%d,%d +%d,%d @@ %s",
				h.OldStart, h.OldLines, h.NewStart, h.NewLines, h.Section), " ")
			fmt.Fprintf(&b, "%s\n", hunkStyle.Render(head))
			for _, ln := range h.Lines {
				switch ln.Kind {
				case mailpatch.Add:
					b.WriteString(add.Render("+" + ln.Text))
				case mailpatch.Delete:
					b.WriteString(del.Render("-" + ln.Text))
				case mailpatch.Context:
					b.WriteString(" " + ln.Text)
				}
				b.WriteByte('\n')
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func fileHeading(f mailpatch.FileChange) string {
	switch f.Type {
	case mailpatch.Renamed, mailpatch.Copied:
		return fmt.Sprintf("%s: %s -> %s", f.Type, f.OldPath, f.NewPath)
	case mailpatch.Added, mailpatch.Deleted, mailpatch.Modified:
		return fmt.Sprintf("%s: %s", f.Type, f.Path())
	default:
		return fmt.Sprintf("%s: %s", f.Type, f.Path())
	}
}

// applyOpenPatch applies the patch to the current working directory and returns
// a human-readable status line for the help bar.
func applyOpenPatch(raw []byte) string {
	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}
	summary, err := gitmail.Apply(raw, ".", gitmail.Options{})
	if err != nil {
		return "✗ apply failed: " + err.Error()
	}
	return fmt.Sprintf("✓ applied %d file(s) into %s", len(summary.Files), wd)
}
