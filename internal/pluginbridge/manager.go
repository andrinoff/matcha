package pluginbridge

import (
	"errors"
	"log"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/backend"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/plugin"
	"github.com/floatpane/matcha/tui"
	lua "github.com/yuin/gopher-lua"
)

// Store is the interface required by the plugin bridge for marking read/unread
// state and updating local email stores.
type Store interface {
	MarkEmailAsReadInStores(uid uint32, accountID string)
	MarkEmailAsUnreadInStores(uid uint32, accountID string)
}

// FlagCmdBuilder creates the backend commands that apply plugin flag ops.
type FlagCmdBuilder interface {
	MarkEmailAsReadCmd(account *config.Account, uid uint32, accountID string, folderName string) tea.Cmd
	MarkEmailAsUnreadCmd(account *config.Account, uid uint32, accountID string, folderName string) tea.Cmd
}

// MoveCmdBuilder creates the backend commands that apply plugin message-move
// ops. The commands run in separate goroutines (Bubble Tea's command channel)
// so the TUI render loop is never blocked by slow IMAP MOVE calls.
type MoveCmdBuilder interface {
	MoveEmailCmd(account *config.Account, uid uint32, accountID, srcFolder, dstFolder, pluginName string) tea.Cmd
}

// MailboxCmdBuilder creates the backend commands that apply plugin folder-
// creation ops. Like move commands, these run asynchronously.
type MailboxCmdBuilder interface {
	CreateFolderCmd(account *config.Account, folderPath, accountID, pluginName string) tea.Cmd
}

type defaultFlagCmdBuilder struct{}

func (defaultFlagCmdBuilder) MarkEmailAsReadCmd(account *config.Account, uid uint32, accountID string, folderName string) tea.Cmd {
	return func() tea.Msg {
		err := fetcher.MarkEmailAsReadInMailbox(account, folderName, uid)
		return tui.EmailMarkedReadMsg{UID: uid, AccountID: accountID, Err: err}
	}
}

func (defaultFlagCmdBuilder) MarkEmailAsUnreadCmd(account *config.Account, uid uint32, accountID string, folderName string) tea.Cmd {
	return func() tea.Msg {
		err := fetcher.MarkEmailAsUnreadInMailbox(account, folderName, uid)
		return tui.EmailMarkedUnreadMsg{UID: uid, AccountID: accountID, Err: err}
	}
}

type defaultMoveCmdBuilder struct{}

func (defaultMoveCmdBuilder) MoveEmailCmd(account *config.Account, uid uint32, accountID, srcFolder, dstFolder, pluginName string) tea.Cmd {
	return func() tea.Msg {
		err := fetcher.MoveEmailToFolder(account, uid, srcFolder, dstFolder)
		return tui.PluginEmailMovedMsg{
			UID:          uid,
			AccountID:    accountID,
			SourceFolder: srcFolder,
			DestFolder:   dstFolder,
			Err:          err,
			PluginName:   pluginName,
		}
	}
}

type defaultMailboxCmdBuilder struct{}

func (defaultMailboxCmdBuilder) CreateFolderCmd(account *config.Account, folderPath, accountID, pluginName string) tea.Cmd {
	return func() tea.Msg {
		err := fetcher.CreateFolder(account, folderPath)
		// Treat "already exists" as a non-error for plugin ergonomics.
		if errors.Is(err, backend.ErrFolderExists) {
			log.Printf("[plugin:%s] folder %q already exists, treating as success", pluginName, folderPath)
			err = nil
		}
		return tui.PluginFolderCreatedMsg{
			AccountID:  accountID,
			FolderPath: folderPath,
			Err:        err,
			PluginName: pluginName,
		}
	}
}

// Manager orchestrates plugin interactions for the main TUI model.
type Manager struct {
	plugins           *plugin.Manager
	store             Store
	cfg               *config.Config
	folderInbox       *tui.FolderInbox
	cmdBuilder        FlagCmdBuilder
	moveCmdBuilder    MoveCmdBuilder
	mailboxCmdBuilder MailboxCmdBuilder
}

func NewManager(plugins *plugin.Manager, store Store, cfg *config.Config, folderInbox *tui.FolderInbox) *Manager {
	return NewManagerWithCmdBuilder(plugins, store, cfg, folderInbox, defaultFlagCmdBuilder{})
}

func NewManagerWithCmdBuilder(plugins *plugin.Manager, store Store, cfg *config.Config, folderInbox *tui.FolderInbox, cmdBuilder FlagCmdBuilder) *Manager {
	if cmdBuilder == nil {
		cmdBuilder = defaultFlagCmdBuilder{}
	}
	return &Manager{
		plugins:           plugins,
		store:             store,
		cfg:               cfg,
		folderInbox:       folderInbox,
		cmdBuilder:        cmdBuilder,
		moveCmdBuilder:    defaultMoveCmdBuilder{},
		mailboxCmdBuilder: defaultMailboxCmdBuilder{},
	}
}

func (m *Manager) SetFolderInbox(folderInbox *tui.FolderInbox) {
	m.folderInbox = folderInbox
}

func (m *Manager) SetConfig(cfg *config.Config) {
	m.cfg = cfg
}

func (m *Manager) SetPlugins(plugins *plugin.Manager) {
	m.plugins = plugins
}

func (m *Manager) FlagCmds() []tea.Cmd {
	if m.plugins == nil {
		return nil
	}
	ops := m.plugins.TakePendingFlagOps()
	if len(ops) == 0 {
		return nil
	}
	var cmds []tea.Cmd
	for _, op := range ops {
		if m.cfg == nil {
			continue
		}
		account := m.cfg.GetAccountByID(op.AccountID)
		if account == nil {
			continue
		}
		if op.Read {
			m.store.MarkEmailAsReadInStores(op.UID, op.AccountID)
			cmds = append(cmds, m.cmdBuilder.MarkEmailAsReadCmd(account, op.UID, op.AccountID, op.Folder))
		} else {
			m.store.MarkEmailAsUnreadInStores(op.UID, op.AccountID)
			cmds = append(cmds, m.cmdBuilder.MarkEmailAsUnreadCmd(account, op.UID, op.AccountID, op.Folder))
		}
	}
	return cmds
}

// MoveCmds drains pending plugin move operations and returns async tea.Cmds
// for each. Each cmd runs the backend MOVE in a separate goroutine, so the
// TUI render loop is never blocked by slow IMAP/JMAP network calls.
func (m *Manager) MoveCmds() []tea.Cmd {
	if m.plugins == nil {
		return nil
	}
	ops := m.plugins.TakePendingMoveOps()
	if len(ops) == 0 {
		return nil
	}
	var cmds []tea.Cmd
	for _, op := range ops {
		if m.cfg == nil {
			continue
		}
		account := m.cfg.GetAccountByID(op.AccountID)
		if account == nil {
			log.Printf("[plugin:%s] move: account %q not found", op.PluginName, op.AccountID)
			continue
		}
		cmds = append(cmds, m.moveCmdBuilder.MoveEmailCmd(
			account, op.UID, op.AccountID, op.SrcFolder, op.DstFolder, op.PluginName,
		))
	}
	return cmds
}

// MailboxCmds drains pending plugin folder-creation operations and returns
// async tea.Cmds for each. Each cmd runs the backend CREATE in a separate
// goroutine, so the TUI render loop is never blocked.
func (m *Manager) MailboxCmds() []tea.Cmd {
	if m.plugins == nil {
		return nil
	}
	ops := m.plugins.TakePendingCreateFolderOps()
	if len(ops) == 0 {
		return nil
	}
	var cmds []tea.Cmd
	for _, op := range ops {
		if m.cfg == nil {
			continue
		}
		account := m.cfg.GetAccountByID(op.AccountID)
		if account == nil {
			log.Printf("[plugin:%s] create folder: account %q not found", op.PluginName, op.AccountID)
			continue
		}
		cmds = append(cmds, m.mailboxCmdBuilder.CreateFolderCmd(
			account, op.FolderPath, op.AccountID, op.PluginName,
		))
	}
	return cmds
}

func (m *Manager) NotifyCmd() tea.Cmd {
	if m.plugins == nil {
		return nil
	}
	if n, ok := m.plugins.TakePendingNotification(); ok {
		return func() tea.Msg {
			return tui.PluginNotifyMsg{
				Message:  n.Message,
				Title:    n.Title,
				Duration: n.Duration,
				Kind:     string(n.Kind),
				Closable: n.Closable,
			}
		}
	}
	return nil
}

func (m *Manager) SyncStatus(current tea.Model) {
	if m.plugins == nil {
		return
	}
	if m.folderInbox != nil {
		m.folderInbox.GetInbox().SetPluginStatus(m.plugins.StatusText(plugin.StatusInbox))
	}
	switch v := current.(type) {
	case *tui.Composer:
		v.SetPluginStatus(m.plugins.StatusText(plugin.StatusComposer))
	case *tui.EmailView:
		v.SetPluginStatus(m.plugins.StatusText(plugin.StatusEmailView))
	}
}

func (m *Manager) HandleKeyBinding(msg tea.KeyPressMsg, current tea.Model, pendingPrompt **plugin.PendingPrompt) tea.Cmd {
	if m.plugins == nil {
		return nil
	}

	keyStr := msg.String()

	var area string
	switch current.(type) {
	case *tui.Inbox:
		area = plugin.StatusInbox
	case *tui.FolderInbox:
		area = plugin.StatusInbox
	case *tui.EmailView:
		area = plugin.StatusEmailView
	case *tui.Composer:
		area = plugin.StatusComposer
	default:
		return nil
	}

	bindings := m.plugins.Bindings(area)
	for _, binding := range bindings {
		if binding.Key != keyStr {
			continue
		}

		switch v := current.(type) {
		case *tui.Inbox:
			if email := v.GetSelectedEmail(); email != nil {
				t := m.plugins.EmailToTable(email.UID, email.From, email.To, email.Subject, email.Date, email.IsRead, email.AccountID, "")
				m.plugins.CallKeyBinding(binding, t)
			} else {
				m.plugins.CallKeyBinding(binding)
			}
		case *tui.FolderInbox:
			if email := v.GetInbox().GetSelectedEmail(); email != nil {
				t := m.plugins.EmailToTable(email.UID, email.From, email.To, email.Subject, email.Date, email.IsRead, email.AccountID, v.GetCurrentFolder())
				m.plugins.CallKeyBinding(binding, t)
			} else {
				m.plugins.CallKeyBinding(binding)
			}
		case *tui.EmailView:
			email := v.GetEmail()
			t := m.plugins.EmailToTable(email.UID, email.From, email.To, email.Subject, email.Date, email.IsRead, email.AccountID, "")
			m.plugins.CallKeyBinding(binding, t)
		case *tui.Composer:
			L := m.plugins.LuaState()
			t := L.NewTable()
			t.RawSetString("body", lua.LString(v.GetBody()))
			t.RawSetString("body_len", lua.LNumber(len(v.GetBody())))
			t.RawSetString("subject", lua.LString(v.GetSubject()))
			t.RawSetString("to", lua.LString(v.GetTo()))
			t.RawSetString("cc", lua.LString(v.GetCc()))
			t.RawSetString("bcc", lua.LString(v.GetBcc()))
			m.plugins.CallKeyBinding(binding, t)
			m.ApplyFields(v)

			if p, ok := m.plugins.TakePendingPrompt(); ok {
				*pendingPrompt = p
				v.ShowPluginPrompt(p.Placeholder)
			}
		}

		m.SyncStatus(current)
		return tea.Batch(m.FlagCmds()...)
	}
	return nil
}

func (m *Manager) IsSearchOverlayOpen(current tea.Model) bool {
	switch v := current.(type) {
	case *tui.Inbox:
		return v.IsSearchOverlayOpen()
	case *tui.FolderInbox:
		return v.GetInbox().IsSearchOverlayOpen()
	}
	return false
}

func (m *Manager) SyncKeyBindings(current tea.Model) {
	if m.plugins == nil {
		return
	}

	toPluginKeyBindings := func(bindings []plugin.KeyBinding) []tui.PluginKeyBinding {
		result := make([]tui.PluginKeyBinding, len(bindings))
		for i, b := range bindings {
			result[i] = tui.PluginKeyBinding{Key: b.Key, Description: b.Description}
		}
		return result
	}

	if m.folderInbox != nil {
		m.folderInbox.GetInbox().SetPluginKeyBindings(toPluginKeyBindings(m.plugins.Bindings(plugin.StatusInbox)))
	}
	switch v := current.(type) {
	case *tui.Composer:
		v.SetPluginKeyBindings(toPluginKeyBindings(m.plugins.Bindings(plugin.StatusComposer)))
	case *tui.ReplySplitView:
		v.Composer().SetPluginKeyBindings(toPluginKeyBindings(m.plugins.Bindings(plugin.StatusComposer)))
	case *tui.EmailView:
		v.SetPluginKeyBindings(toPluginKeyBindings(m.plugins.Bindings(plugin.StatusEmailView)))
	}
}

func (m *Manager) ApplyFields(composer *tui.Composer) {
	if m.plugins == nil {
		return
	}
	fields := m.plugins.TakePendingFields()
	if fields == nil {
		return
	}
	for field, value := range fields {
		switch field {
		case "to":
			composer.SetTo(value)
		case "cc":
			composer.SetCc(value)
		case "bcc":
			composer.SetBcc(value)
		case "subject":
			composer.SetSubject(value)
		case "body":
			composer.SetBody(value)
		}
	}
}
