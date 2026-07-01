package plugin

import (
	lua "github.com/yuin/gopher-lua"
)

// matcha.mailbox.create(account_id, folder_path) — queue a folder-creation
// operation. The orchestrator dispatches the actual backend call
// asynchronously after the Lua callback returns, so the TUI render loop is
// never blocked by a slow IMAP CREATE command.
//
// The operation is idempotent from the plugin's perspective: if the folder
// already exists on the server, the queued op succeeds silently (no error is
// surfaced to the plugin).
//
// Example:
//
//	matcha.mailbox.create("acct-1", "Archive/2024")
func (m *Manager) luaMailboxCreate(L *lua.LState) int { //nolint:gocritic
	accountID := L.CheckString(1)
	folderPath := L.CheckString(2)

	m.pendingCreateFolderOps = append(m.pendingCreateFolderOps, CreateFolderOp{
		AccountID:  accountID,
		FolderPath: folderPath,
		PluginName: m.currentPlugin,
	})
	return 0
}

// matcha.message.move(account_id, uid, source_folder, destination_folder) —
// queue a message-move operation. The orchestrator dispatches the actual
// backend call asynchronously after the Lua callback returns, so the TUI
// render loop is never blocked by a slow IMAP MOVE command.
//
// The uid is the numeric UID as exposed in the email table's "uid" field.
//
// Example:
//
//	matcha.message.move(email.account_id, email.uid, email.folder, "Archive/2024")
func (m *Manager) luaMessageMove(L *lua.LState) int { //nolint:gocritic
	accountID := L.CheckString(1)
	uid := uint32(L.CheckInt(2))
	srcFolder := L.CheckString(3)
	dstFolder := L.CheckString(4)

	m.pendingMoveOps = append(m.pendingMoveOps, MoveOp{
		UID:        uid,
		AccountID:  accountID,
		SrcFolder:  srcFolder,
		DstFolder:  dstFolder,
		PluginName: m.currentPlugin,
	})
	return 0
}
