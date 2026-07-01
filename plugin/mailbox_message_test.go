package plugin

import (
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

func TestLuaMailboxCreate_QueuesPendingOp(t *testing.T) {
	m := NewManager()
	defer m.Close()
	m.currentPlugin = "test_archiver"

	L := m.LuaState()
	err := L.DoString(`
		local matcha = require("matcha")
		matcha.mailbox.create("acct-1", "Archive/2024")
	`)
	if err != nil {
		t.Fatalf("lua error: %v", err)
	}

	ops := m.TakePendingCreateFolderOps()
	if len(ops) != 1 {
		t.Fatalf("expected 1 pending create-folder op, got %d", len(ops))
	}
	op := ops[0]
	if op.AccountID != "acct-1" {
		t.Fatalf("expected account_id %q, got %q", "acct-1", op.AccountID)
	}
	if op.FolderPath != "Archive/2024" {
		t.Fatalf("expected folder_path %q, got %q", "Archive/2024", op.FolderPath)
	}
	if op.PluginName != "test_archiver" {
		t.Fatalf("expected plugin name %q, got %q", "test_archiver", op.PluginName)
	}

	// Verify the queue is drained.
	if ops2 := m.TakePendingCreateFolderOps(); ops2 != nil {
		t.Fatalf("expected nil after drain, got %d ops", len(ops2))
	}
}

func TestLuaMessageMove_QueuesPendingOp(t *testing.T) {
	m := NewManager()
	defer m.Close()
	m.currentPlugin = "test_archiver"

	L := m.LuaState()
	err := L.DoString(`
		local matcha = require("matcha")
		matcha.message.move("acct-1", 42, "INBOX", "Archive/2024")
	`)
	if err != nil {
		t.Fatalf("lua error: %v", err)
	}

	ops := m.TakePendingMoveOps()
	if len(ops) != 1 {
		t.Fatalf("expected 1 pending move op, got %d", len(ops))
	}
	op := ops[0]
	if op.AccountID != "acct-1" {
		t.Fatalf("expected account_id %q, got %q", "acct-1", op.AccountID)
	}
	if op.UID != 42 {
		t.Fatalf("expected uid 42, got %d", op.UID)
	}
	if op.SrcFolder != "INBOX" {
		t.Fatalf("expected src_folder %q, got %q", "INBOX", op.SrcFolder)
	}
	if op.DstFolder != "Archive/2024" {
		t.Fatalf("expected dst_folder %q, got %q", "Archive/2024", op.DstFolder)
	}
	if op.PluginName != "test_archiver" {
		t.Fatalf("expected plugin name %q, got %q", "test_archiver", op.PluginName)
	}
}

func TestLuaMessageMove_RequiresAllArgs(t *testing.T) {
	m := NewManager()
	defer m.Close()

	L := m.LuaState()
	// Missing the destination folder argument should raise a Lua error.
	err := L.DoString(`
		local matcha = require("matcha")
		matcha.message.move("acct-1", 42, "INBOX")
	`)
	if err == nil {
		t.Fatal("expected error for missing destination folder argument")
	}
}

func TestTakePendingMoveOps_NilWhenEmpty(t *testing.T) {
	m := NewManager()
	defer m.Close()

	if ops := m.TakePendingMoveOps(); ops != nil {
		t.Fatalf("expected nil on fresh manager, got %d ops", len(ops))
	}
}

func TestTakePendingCreateFolderOps_NilWhenEmpty(t *testing.T) {
	m := NewManager()
	defer m.Close()

	if ops := m.TakePendingCreateFolderOps(); ops != nil {
		t.Fatalf("expected nil on fresh manager, got %d ops", len(ops))
	}
}

func TestCallNewEmailHook_DispatchesToCallback(t *testing.T) {
	m := NewManager()
	defer m.Close()
	m.currentPlugin = "test_archiver"

	L := m.LuaState()
	// Register a callback that stashes the message id and date into globals.
	err := L.DoString(`
		local matcha = require("matcha")
		matcha.on("on_new_email", function(msg)
			_received_id = msg.id
			_received_uid = msg.uid
			_received_date = msg.date_received
			_received_account = msg.account_id
			_received_folder = msg.folder
			_received_from = msg.from
			_received_subject = msg.subject
			_received_is_read = msg.is_read
		end)
	`)
	if err != nil {
		t.Fatalf("lua error registering hook: %v", err)
	}

	date := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	m.CallNewEmailHook(NewEmailInfo{
		UID:          99,
		AccountID:    "acct-1",
		Folder:       "INBOX",
		From:         "sender@example.com",
		Subject:      "Test subject",
		DateReceived: date,
		IsRead:       false,
	})

	if got := L.GetGlobal("_received_id"); got.String() != "acct-1:99" {
		t.Fatalf("expected id %q, got %q", "acct-1:99", got.String())
	}
	if got := L.GetGlobal("_received_uid"); got.(lua.LNumber) != 99 {
		t.Fatalf("expected uid 99, got %v", got)
	}
	if got := L.GetGlobal("_received_date"); got.String() != date.Format(time.RFC3339) {
		t.Fatalf("expected date %q, got %q", date.Format(time.RFC3339), got.String())
	}
	if got := L.GetGlobal("_received_account"); got.String() != "acct-1" {
		t.Fatalf("expected account_id %q, got %q", "acct-1", got.String())
	}
	if got := L.GetGlobal("_received_folder"); got.String() != "INBOX" {
		t.Fatalf("expected folder %q, got %q", "INBOX", got.String())
	}
	if got := L.GetGlobal("_received_from"); got.String() != "sender@example.com" {
		t.Fatalf("expected from %q, got %q", "sender@example.com", got.String())
	}
	if got := L.GetGlobal("_received_subject"); got.String() != "Test subject" {
		t.Fatalf("expected subject %q, got %q", "Test subject", got.String())
	}
	if got := L.GetGlobal("_received_is_read"); got != lua.LFalse {
		t.Fatalf("expected is_read false, got %v", got)
	}
}

func TestCallNewEmailHook_NoCallbacksIsNoOp(t *testing.T) {
	m := NewManager()
	defer m.Close()

	// No callbacks registered — should not panic.
	m.CallNewEmailHook(NewEmailInfo{
		UID:          1,
		AccountID:    "acct-1",
		Folder:       "INBOX",
		From:         "x@example.com",
		Subject:      "x",
		DateReceived: time.Now(),
	})
}

func TestCallNewEmailHook_CallbackCanQueueMoveOp(t *testing.T) {
	m := NewManager()
	defer m.Close()
	m.currentPlugin = "auto_archiver"

	L := m.LuaState()
	// The callback parses the year from date_received and queues a move.
	err := L.DoString(`
		local matcha = require("matcha")
		matcha.on("on_new_email", function(msg)
			local year = msg.date_received:sub(1, 4)
			local archive_folder = "Archive/" .. year
			matcha.mailbox.create(msg.account_id, archive_folder)
			matcha.message.move(msg.account_id, msg.uid, msg.folder, archive_folder)
		end)
	`)
	if err != nil {
		t.Fatalf("lua error: %v", err)
	}

	date := time.Date(2023, 3, 10, 8, 0, 0, 0, time.UTC)
	m.CallNewEmailHook(NewEmailInfo{
		UID:          55,
		AccountID:    "acct-1",
		Folder:       "INBOX",
		From:         "sender@example.com",
		Subject:      "Old email",
		DateReceived: date,
	})

	// Verify the callback queued both a folder creation and a move.
	createOps := m.TakePendingCreateFolderOps()
	if len(createOps) != 1 {
		t.Fatalf("expected 1 create-folder op, got %d", len(createOps))
	}
	if createOps[0].FolderPath != "Archive/2023" {
		t.Fatalf("expected folder %q, got %q", "Archive/2023", createOps[0].FolderPath)
	}

	moveOps := m.TakePendingMoveOps()
	if len(moveOps) != 1 {
		t.Fatalf("expected 1 move op, got %d", len(moveOps))
	}
	if moveOps[0].DstFolder != "Archive/2023" {
		t.Fatalf("expected dst folder %q, got %q", "Archive/2023", moveOps[0].DstFolder)
	}
	if moveOps[0].UID != 55 {
		t.Fatalf("expected uid 55, got %d", moveOps[0].UID)
	}
}
