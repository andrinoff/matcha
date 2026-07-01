package plugin

import (
	"log"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// Hook event names.
const (
	HookStartup         = "startup"
	HookShutdown        = "shutdown"
	HookEmailReceived   = "email_received"
	HookNewEmail        = "on_new_email"
	HookEmailSendBefore = "email_send_before"
	HookEmailSendAfter  = "email_send_after"
	HookEmailViewed     = "email_viewed"
	HookFolderChanged   = "folder_changed"
	HookComposerUpdated = "composer_updated"
	HookEmailBodyRender = "email_body_render"
)

// Status area names.
const (
	StatusInbox     = "inbox"
	StatusComposer  = "composer"
	StatusEmailView = "email_view"
)

type registeredHook struct {
	fn     *lua.LFunction
	plugin string
}

// registerHook adds a callback for the given event.
func (m *Manager) registerHook(event string, fn *lua.LFunction) {
	m.hooks[event] = append(m.hooks[event], registeredHook{fn: fn, plugin: m.currentPlugin})
}

// CallHook invokes all callbacks registered for the given event.
func (m *Manager) CallHook(event string, args ...lua.LValue) {
	callbacks, ok := m.hooks[event]
	if !ok {
		return
	}

	previousPlugin := m.currentPlugin
	defer func() {
		m.currentPlugin = previousPlugin
	}()

	for _, hook := range callbacks {
		m.currentPlugin = hook.plugin
		if err := m.state.CallByParam(lua.P{
			Fn:      hook.fn,
			NRet:    0,
			Protect: true,
		}, args...); err != nil {
			log.Printf("plugin hook %q error: %v", event, err)
		}
	}
}

// NewEmailInfo carries the minimal metadata needed by the on_new_email hook.
// It intentionally excludes the email body to avoid loading large payloads
// into memory when plugins only need the date and identity for routing
// decisions (e.g. year-based archiving).
type NewEmailInfo struct {
	UID          uint32
	AccountID    string
	Folder       string
	From         string
	Subject      string
	DateReceived time.Time
	IsRead       bool
}

// CallNewEmailHook dispatches the on_new_email event to all registered
// callbacks. It builds a lightweight Lua table containing essential metadata
// (id, uid, date_received, account_id, folder, from, subject, is_read) without
// loading the email body. This is the primary entry point for auto-archiver
// and filtering plugins that need to act on incoming mail.
//
// The Lua table passed to the callback:
//
//	{
//	  id            = "acct-1:1234",   -- unique identifier string
//	  uid           = 1234,            -- numeric UID for move/delete ops
//	  date_received = "2024-06-15T...", -- RFC3339 timestamp
//	  account_id    = "acct-1",
//	  folder        = "INBOX",
//	  from          = "sender@example.com",
//	  subject       = "...",
//	  is_read       = false,
//	}
func (m *Manager) CallNewEmailHook(info NewEmailInfo) {
	callbacks, ok := m.hooks[HookNewEmail]
	if !ok || len(callbacks) == 0 {
		return
	}

	L := m.state
	t := L.NewTable()
	t.RawSetString("id", lua.LString(info.AccountID+":"+itoa(info.UID)))
	t.RawSetString("uid", lua.LNumber(info.UID))
	t.RawSetString("date_received", lua.LString(info.DateReceived.Format(time.RFC3339)))
	t.RawSetString("account_id", lua.LString(info.AccountID))
	t.RawSetString("folder", lua.LString(info.Folder))
	t.RawSetString("from", lua.LString(info.From))
	t.RawSetString("subject", lua.LString(info.Subject))
	t.RawSetString("is_read", lua.LBool(info.IsRead))

	previousPlugin := m.currentPlugin
	defer func() {
		m.currentPlugin = previousPlugin
	}()

	for _, hook := range callbacks {
		m.currentPlugin = hook.plugin
		if err := L.CallByParam(lua.P{
			Fn:      hook.fn,
			NRet:    0,
			Protect: true,
		}, t); err != nil {
			log.Printf("plugin hook %q error: %v", HookNewEmail, err)
		}
	}
}

// itoa converts a uint32 to its decimal string representation without
// importing strconv (which is not otherwise needed in this file).
func itoa(n uint32) string {
	if n == 0 {
		return "0"
	}
	var buf [10]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// CallSendHook calls a hook with email send metadata.
func (m *Manager) CallSendHook(event string, to, cc, subject, accountID string) {
	callbacks, ok := m.hooks[event]
	if !ok || len(callbacks) == 0 {
		return
	}

	L := m.state
	t := L.NewTable()
	t.RawSetString("to", lua.LString(to))
	t.RawSetString("cc", lua.LString(cc))
	t.RawSetString("subject", lua.LString(subject))
	t.RawSetString("account_id", lua.LString(accountID))

	previousPlugin := m.currentPlugin
	defer func() {
		m.currentPlugin = previousPlugin
	}()
	for _, hook := range callbacks {
		m.currentPlugin = hook.plugin
		if err := L.CallByParam(lua.P{
			Fn:      hook.fn,
			NRet:    0,
			Protect: true,
		}, t); err != nil {
			log.Printf("plugin hook %q error: %v", event, err)
		}
	}
}

// CallFolderHook calls a hook with a folder name.
func (m *Manager) CallFolderHook(event string, folderName string) {
	callbacks, ok := m.hooks[event]
	if !ok {
		return
	}

	previousPlugin := m.currentPlugin
	defer func() {
		m.currentPlugin = previousPlugin
	}()

	for _, hook := range callbacks {
		m.currentPlugin = hook.plugin
		if err := m.state.CallByParam(lua.P{
			Fn:      hook.fn,
			NRet:    0,
			Protect: true,
		}, lua.LString(folderName)); err != nil {
			log.Printf("plugin hook %q error: %v", event, err)
		}
	}
}

// CallComposerHook calls a hook with composer state info.
func (m *Manager) CallComposerHook(event string, body, subject, to, cc, bcc string) {
	callbacks, ok := m.hooks[event]
	if !ok || len(callbacks) == 0 {
		return
	}

	L := m.state
	t := L.NewTable()
	t.RawSetString("body_len", lua.LNumber(len(body)))
	t.RawSetString("body", lua.LString(body))
	t.RawSetString("subject", lua.LString(subject))
	t.RawSetString("to", lua.LString(to))
	t.RawSetString("cc", lua.LString(cc))
	t.RawSetString("bcc", lua.LString(bcc))

	previousPlugin := m.currentPlugin
	defer func() {
		m.currentPlugin = previousPlugin
	}()
	for _, hook := range callbacks {
		m.currentPlugin = hook.plugin
		if err := L.CallByParam(lua.P{
			Fn:      hook.fn,
			NRet:    0,
			Protect: true,
		}, t); err != nil {
			log.Printf("plugin hook %q error: %v", event, err)
		}
	}
}

// CallBodyRenderHook runs all email_body_render callbacks, threading the body
// string through each. Callbacks receive (email_table, rendered, raw):
//   - rendered: the current display string (ANSI-styled, post-HTML→terminal)
//   - raw: the original message body (HTML or plain text, same string fed to
//     the renderer) — useful for parsing the source instead of the rendered
//     output
//
// A callback may return a string to replace the rendered body, or nil to leave
// it unchanged. Non-string returns are ignored. Multiple callbacks chain in
// registration order; each subsequent callback sees the previous callback's
// rendered output, but always the same raw source.
func (m *Manager) CallBodyRenderHook(email *lua.LTable, rendered, raw string) string {
	callbacks, ok := m.hooks[HookEmailBodyRender]
	if !ok {
		return rendered
	}

	L := m.state
	previousPlugin := m.currentPlugin
	defer func() {
		m.currentPlugin = previousPlugin
	}()

	for _, hook := range callbacks {
		m.currentPlugin = hook.plugin
		if err := L.CallByParam(lua.P{
			Fn:      hook.fn,
			NRet:    1,
			Protect: true,
		}, email, lua.LString(rendered), lua.LString(raw)); err != nil {
			log.Printf("plugin hook %q error: %v", HookEmailBodyRender, err)
			continue
		}
		ret := L.Get(-1)
		L.Pop(1)
		if s, ok := ret.(lua.LString); ok {
			rendered = string(s)
		}
	}
	return rendered
}

// CallKeyBinding invokes a plugin key binding callback with the given arguments.
func (m *Manager) CallKeyBinding(binding KeyBinding, args ...lua.LValue) {
	previousPlugin := m.currentPlugin
	m.currentPlugin = binding.Plugin
	defer func() {
		m.currentPlugin = previousPlugin
	}()

	if err := m.state.CallByParam(lua.P{
		Fn:      binding.Fn,
		NRet:    0,
		Protect: true,
	}, args...); err != nil {
		log.Printf("plugin keybinding %q error: %v", binding.Key, err)
	}
}

// EmailToTable converts email fields into a Lua table.
func (m *Manager) EmailToTable(uid uint32, from string, to []string, subject string, date time.Time, isRead bool, accountID string, folder string) *lua.LTable {
	L := m.state

	t := L.NewTable()
	t.RawSetString("uid", lua.LNumber(uid))
	t.RawSetString("from", lua.LString(from))
	t.RawSetString("subject", lua.LString(subject))
	t.RawSetString("date", lua.LString(date.Format(time.RFC3339)))
	t.RawSetString("is_read", lua.LBool(isRead))
	t.RawSetString("account_id", lua.LString(accountID))
	t.RawSetString("folder", lua.LString(folder))

	toTable := L.NewTable()
	for i, addr := range to {
		toTable.RawSetInt(i+1, lua.LString(addr))
	}
	t.RawSetString("to", toTable)

	return t
}
