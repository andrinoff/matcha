package tui

import (
	"strings"
	"testing"

	"github.com/floatpane/matcha/fetcher"
)

const patchBody = `commit message here

---
 greet.txt | 2 +-
 1 file changed, 1 insertion(+), 1 deletion(-)

diff --git a/greet.txt b/greet.txt
index 111..222 100644
--- a/greet.txt
+++ b/greet.txt
@@ -1,3 +1,3 @@
 hello
-world
+there
 bye
`

func patchEmail() fetcher.Email {
	return fetcher.Email{
		From:         "Ada <ada@example.com>",
		Subject:      "[PATCH 2/3] greet: change the world",
		Body:         patchBody,
		BodyMIMEType: "text/plain",
	}
}

func TestDetectPatch(t *testing.T) {
	p, raw, ok := detectPatch(patchEmail())
	if !ok {
		t.Fatal("detectPatch = false for a patch email")
	}
	if p.Subject != "greet: change the world" {
		t.Errorf("Subject = %q", p.Subject)
	}
	if p.Series.Index != 2 || p.Series.Total != 3 {
		t.Errorf("Series = %+v", p.Series)
	}
	if len(raw) == 0 {
		t.Error("raw is empty")
	}
}

func TestDetectPatchRejectsHTML(t *testing.T) {
	e := patchEmail()
	e.BodyMIMEType = "text/html"
	if _, _, ok := detectPatch(e); ok {
		t.Error("detectPatch should reject HTML bodies")
	}
}

func TestDetectPatchRejectsPlainEmail(t *testing.T) {
	e := fetcher.Email{
		From:         "Bob <bob@example.com>",
		Subject:      "lunch?",
		Body:         "want to grab lunch tomorrow?\n",
		BodyMIMEType: "text/plain",
	}
	if _, _, ok := detectPatch(e); ok {
		t.Error("detectPatch should reject a non-patch email")
	}
}

func TestRenderPatch(t *testing.T) {
	p, _, ok := detectPatch(patchEmail())
	if !ok {
		t.Fatal("detectPatch failed")
	}
	out := renderPatch(p)
	for _, want := range []string{"greet.txt", "world", "there", "Patch 2/3"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered patch missing %q", want)
		}
	}
}
