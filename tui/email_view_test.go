package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/fetcher"
)

func TestEmailViewUpdate(t *testing.T) {
	emailWithAttachments := fetcher.Email{
		From:    "test@example.com",
		Subject: "Test Email with Attachments",
		Body:    "This is the body.",
		Date:    time.Now(),
		Attachments: []fetcher.Attachment{
			{Filename: "attachment1.txt", Data: []byte("attachment1")},
			{Filename: "attachment2.txt", Data: []byte("attachment2")},
		},
	}

	emailWithoutAttachments := fetcher.Email{
		From:    "test@example.com",
		Subject: "Test Email without Attachments",
		Body:    "This is the body.",
		Date:    time.Now(),
	}

	t.Run("Focus on attachments", func(t *testing.T) {
		emailView := NewEmailView(emailWithAttachments, 0, 80, 24, MailboxInbox, false)
		if emailView.focusOnAttachments {
			t.Error("focusOnAttachments should be initially false")
		}

		// Tab to focus on attachments
		model, _ := emailView.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		emailView = model.(*EmailView)

		if !emailView.focusOnAttachments {
			t.Error("focusOnAttachments should be true after tabbing")
		}

		// Tab back to body
		model, _ = emailView.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		emailView = model.(*EmailView)
		if emailView.focusOnAttachments {
			t.Error("focusOnAttachments should be false after tabbing again")
		}
	})

	t.Run("No focus on attachments when there are none", func(t *testing.T) {
		emailView := NewEmailView(emailWithoutAttachments, 0, 80, 24, MailboxInbox, false)
		if emailView.focusOnAttachments {
			t.Error("focusOnAttachments should be initially false")
		}
		// Tab
		model, _ := emailView.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		emailView = model.(*EmailView)
		if emailView.focusOnAttachments {
			t.Error("focusOnAttachments should remain false when there are no attachments")
		}
	})

	t.Run("Navigate attachments", func(t *testing.T) {
		emailView := NewEmailView(emailWithAttachments, 0, 80, 24, MailboxInbox, false)
		// Focus on attachments
		model, _ := emailView.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		emailView = model.(*EmailView)

		if emailView.attachmentCursor != 0 {
			t.Errorf("Initial attachmentCursor should be 0, got %d", emailView.attachmentCursor)
		}

		// Move down
		model, _ = emailView.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		emailView = model.(*EmailView)
		if emailView.attachmentCursor != 1 {
			t.Errorf("After one down arrow, attachmentCursor should be 1, got %d", emailView.attachmentCursor)
		}

		// Move down again (should wrap to the first attachment)
		model, _ = emailView.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		emailView = model.(*EmailView)
		if emailView.attachmentCursor != 0 {
			t.Errorf("attachmentCursor should wrap to the start of the list, got %d", emailView.attachmentCursor)
		}

		// Move up (should wrap to the last attachment)
		model, _ = emailView.Update(tea.KeyPressMsg{Code: tea.KeyUp})
		emailView = model.(*EmailView)
		if emailView.attachmentCursor != 1 {
			t.Errorf("After one up arrow from the first item, attachmentCursor should be 1, got %d", emailView.attachmentCursor)
		}
	})

	t.Run("Attachment navigation does not scroll body", func(t *testing.T) {
		emailView := NewEmailView(emailWithAttachments, 0, 80, 24, MailboxInbox, false)
		emailView.viewport.SetHeight(2)
		emailView.viewport.SetContent("line 1\nline 2\nline 3\nline 4\nline 5\n")
		emailView.viewport.SetYOffset(1)

		model, _ := emailView.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		emailView = model.(*EmailView)
		if !emailView.focusOnAttachments {
			t.Fatal("focusOnAttachments should be true after tabbing")
		}

		before := emailView.viewport.YOffset()
		model, _ = emailView.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		emailView = model.(*EmailView)
		if got := emailView.viewport.YOffset(); got != before {
			t.Fatalf("attachment navigation should not scroll the email body, got offset %d want %d", got, before)
		}
	})

	t.Run("Download attachment", func(t *testing.T) {
		emailView := NewEmailView(emailWithAttachments, 0, 80, 24, MailboxInbox, false)
		// Focus on attachments
		model, _ := emailView.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		emailView = model.(*EmailView)

		// Move to the second attachment
		model, _ = emailView.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		emailView = model.(*EmailView)

		// Press enter
		_, cmd := emailView.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("Expected a command, but got nil")
		}

		msg := cmd()
		downloadMsg, ok := msg.(DownloadAttachmentMsg)
		if !ok {
			t.Fatalf("Expected a DownloadAttachmentMsg, but got %T", msg)
		}
		if downloadMsg.Filename != "attachment2.txt" {
			t.Errorf("Expected to download 'attachment2.txt', but got '%s'", downloadMsg.Filename)
		}
		if downloadMsg.Mailbox != MailboxInbox {
			t.Errorf("Expected mailbox to be MailboxInbox, got %s", downloadMsg.Mailbox)
		}
	})

	t.Run("Reply to email", func(t *testing.T) {
		emailView := NewEmailView(emailWithAttachments, 0, 80, 24, MailboxInbox, false)

		_, cmd := emailView.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
		if cmd == nil {
			t.Fatal("Expected a command, but got nil")
		}

		msg := cmd()
		replyMsg, ok := msg.(ReplyToEmailMsg)
		if !ok {
			t.Fatalf("Expected a ReplyToEmailMsg, but got %T", msg)
		}
		if replyMsg.Email.Subject != emailWithAttachments.Subject {
			t.Errorf("Expected reply to have subject '%s', but got '%s'", emailWithAttachments.Subject, replyMsg.Email.Subject)
		}
	})
}

func TestEmailViewMailtoClick(t *testing.T) {
	origTerm := os.Getenv("TERM")
	origTermProgram := os.Getenv("TERM_PROGRAM")
	t.Cleanup(func() {
		os.Setenv("TERM", origTerm)
		os.Setenv("TERM_PROGRAM", origTermProgram)
	})
	os.Setenv("TERM", "xterm-kitty")
	os.Setenv("TERM_PROGRAM", "")

	email := fetcher.Email{
		From:         "sender@example.com",
		To:           []string{"recipient@example.com"},
		Subject:      "Email with mailto link",
		Body:         `<a href="mailto:contact@example.com">contact@example.com</a>`,
		BodyMIMEType: "text/html",
		Date:         time.Now(),
		AccountID:    "test-acct",
	}
	emailView := NewEmailView(email, 0, 80, 24, MailboxInbox, false)

	if len(emailView.mailtoLinks) != 1 {
		t.Fatalf("expected 1 stored mailto link, got %d", len(emailView.mailtoLinks))
	}
	if emailView.mailtoLinks[0].VisibleText != "contact@example.com" {
		t.Fatalf("unexpected VisibleText: %q", emailView.mailtoLinks[0].VisibleText)
	}

	// Find the line containing the link's visible text in the rendered viewport.
	content := emailView.viewport.GetContent()
	lines := strings.Split(content, "\n")
	linkLine := -1
	linkCol := -1
	searchText := emailView.mailtoLinks[0].VisibleText
	for i, line := range lines {
		plain := stripANSIFromLine(line)
		if idx := strings.Index(plain, searchText); idx >= 0 {
			linkLine = i
			linkCol = idx
			break
		}
	}
	if linkLine < 0 {
		t.Fatalf("could not find mailto link text in rendered body:\n%s", content)
	}

	// Map content line + visible column to screen coordinates.
	headerHeight := emailView.renderedHeaderHeight()
	clickY := headerHeight + 1 + linkLine
	clickX := linkCol

	model, cmd := emailView.Update(tea.MouseClickMsg{
		X:      clickX,
		Y:      clickY,
		Button: tea.MouseLeft,
	})
	if cmd == nil {
		t.Fatal("Expected a command for mailto click, got nil")
	}
	msg := cmd()
	openMsg, ok := msg.(OpenMailtoMsg)
	if !ok {
		t.Fatalf("Expected OpenMailtoMsg, got %T", msg)
	}
	if openMsg.URL != "mailto:contact@example.com" {
		t.Errorf("URL = %q, want %q", openMsg.URL, "mailto:contact@example.com")
	}

	if _, ok := model.(*EmailView); !ok {
		t.Errorf("Expected model to remain *EmailView, got %T", model)
	}
}

func TestEmailViewMailtoClickOutsideLink(t *testing.T) {
	origTerm := os.Getenv("TERM")
	t.Cleanup(func() { os.Setenv("TERM", origTerm) })
	os.Setenv("TERM", "xterm-kitty")

	email := fetcher.Email{
		From:         "sender@example.com",
		To:           []string{"recipient@example.com"},
		Subject:      "Email without mailto link",
		Body:         `<a href="https://example.com">visit site</a>`,
		BodyMIMEType: "text/html",
		Date:         time.Now(),
	}
	emailView := NewEmailView(email, 0, 80, 24, MailboxInbox, false)

	if len(emailView.mailtoLinks) != 0 {
		t.Fatalf("expected 0 mailto links, got %d", len(emailView.mailtoLinks))
	}

	headerHeight := emailView.renderedHeaderHeight()
	clickY := headerHeight + 1
	clickX := 0

	_, cmd := emailView.Update(tea.MouseClickMsg{
		X:      clickX,
		Y:      clickY,
		Button: tea.MouseLeft,
	})
	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(OpenMailtoMsg); ok {
			t.Fatal("Should not emit OpenMailtoMsg when email has no mailto links")
		}
	}
}

// stripANSIFromLine removes ANSI escape sequences for test text matching.
func stripANSIFromLine(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == 0x1b {
			i++
			for i < len(s) && s[i] != 'm' && s[i] != 'H' && s[i] != 'G' {
				i++
			}
			if i < len(s) {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
