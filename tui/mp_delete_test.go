package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/plugins"
)

func mpEntries(n int, installedName string) []plugins.PluginEntry {
	out := make([]plugins.PluginEntry, n)
	for i := range n {
		name := installedName
		if i != 0 {
			name = "other" + string(rune('a'+i))
		}
		out[i] = plugins.PluginEntry{Name: name, Title: "Plugin " + name, Description: "desc", File: name + ".lua"}
	}
	return out
}

func TestMarketplaceDeleteFlow(t *testing.T) {
	RebuildStyles()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".config", "matcha", "plugins")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	pluginName := "test-delete-flow"
	dest := filepath.Join(dir, pluginName+".lua")
	if err := os.WriteFile(dest, []byte("-- test"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(dest)

	mp := Marketplace{
		installed:  map[string]bool{pluginName: true},
		standalone: false,
		width:      80,
		height:     30,
		state:      marketplaceReady,
		entries:    mpEntries(5, pluginName),
		cursor:     0,
	}

	deleteKey := config.Keybinds.Inbox.Delete
	model, _ := mp.Update(tea.KeyPressMsg{Code: rune(deleteKey[0]), Text: deleteKey})
	mp = model.(Marketplace)

	if !mp.confirmingDelete {
		t.Fatalf("expected confirmingDelete=true after pressing %q", deleteKey)
	}
	if mp.deleteTarget != pluginName {
		t.Fatalf("expected deleteTarget=%q, got %q", pluginName, mp.deleteTarget)
	}

	model, cmd := mp.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	mp = model.(Marketplace)

	if mp.confirmingDelete {
		t.Fatal("expected confirmingDelete=false after confirming")
	}
	if cmd == nil {
		t.Fatal("expected a command to be returned for deletion")
	}

	msg := cmd()
	dmsg, ok := msg.(PluginDeletedMsg)
	if !ok {
		t.Fatalf("expected PluginDeletedMsg, got %T", msg)
	}
	if dmsg.Err != nil {
		t.Fatalf("unexpected delete error: %v", dmsg.Err)
	}

	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("expected plugin file to be deleted, stat err=%v", err)
	}

	model, _ = mp.Update(dmsg)
	mp = model.(Marketplace)
	if mp.installed[pluginName] {
		t.Fatal("expected plugin removed from installed map after deletion")
	}
}

func TestMarketplaceDeleteCancel(t *testing.T) {
	RebuildStyles()
	mp := Marketplace{
		installed:        map[string]bool{"plug": true},
		standalone:       false,
		width:            80,
		height:           30,
		state:            marketplaceReady,
		entries:          mpEntries(3, "plug"),
		cursor:           0,
		confirmingDelete: true,
		deleteTarget:     "plug",
	}

	model, _ := mp.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	mp = model.(Marketplace)
	if mp.confirmingDelete {
		t.Fatal("expected confirmingDelete=false after pressing n")
	}
	if mp.deleteTarget != "" {
		t.Fatal("expected deleteTarget cleared after cancel")
	}

	mp.confirmingDelete = true
	mp.deleteTarget = "plug"
	cancelKey := config.Keybinds.Global.Cancel
	model, _ = mp.Update(tea.KeyPressMsg{Text: cancelKey})
	mp = model.(Marketplace)
	if mp.confirmingDelete {
		t.Fatalf("expected confirmingDelete=false after pressing cancel key %q", cancelKey)
	}
}

func TestMarketplaceDeleteOnlyOnInstalled(t *testing.T) {
	RebuildStyles()
	mp := Marketplace{
		installed:  map[string]bool{},
		standalone: false,
		width:      80,
		height:     30,
		state:      marketplaceReady,
		entries:    mpEntries(3, "plug"),
		cursor:     0,
	}
	deleteKey := config.Keybinds.Inbox.Delete
	model, _ := mp.Update(tea.KeyPressMsg{Code: rune(deleteKey[0]), Text: deleteKey})
	mp = model.(Marketplace)
	if mp.confirmingDelete {
		t.Fatal("should not prompt delete for an uninstalled plugin")
	}
}

func TestMarketplaceDeleteDialogRenders(t *testing.T) {
	RebuildStyles()
	mp := Marketplace{
		installed:        map[string]bool{"plug": true},
		standalone:       false,
		width:            80,
		height:           30,
		state:            marketplaceReady,
		entries:          mpEntries(3, "plug"),
		cursor:           0,
		confirmingDelete: true,
		deleteTarget:     "plug",
	}
	rendered := mp.View().Content
	if !strings.Contains(rendered, "plug") {
		t.Error("confirmation dialog should show the plugin name")
	}
	if !strings.Contains(rendered, "(y/n)") {
		t.Error("confirmation dialog should show (y/n) prompt")
	}
}
