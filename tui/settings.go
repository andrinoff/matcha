package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/config"
)

var (
	accountItemStyle         = lipgloss.NewStyle().PaddingLeft(2)
	selectedAccountItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("42"))
	accountEmailStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	dangerStyle              = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

type SettingsState int

const (
	SettingsMain SettingsState = iota
	SettingsAccounts
	SettingsMailingLists
)

// Settings displays the settings screen.
type Settings struct {
	cfg              *config.Config
	state            SettingsState
	cursor           int
	confirmingDelete bool
	width            int
	height           int
}

// NewSettings creates a new settings model.
func NewSettings(cfg *config.Config) *Settings {
	if cfg == nil {
		cfg = &config.Config{}
	}
	return &Settings{
		cfg:    cfg,
		state:  SettingsMain,
		cursor: 0,
	}
}

// Init initializes the settings model.
func (m *Settings) Init() tea.Cmd {
	return nil
}

// Update handles messages for the settings model.
func (m *Settings) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		if m.state == SettingsMain {
			return m.updateMain(msg)
		} else if m.state == SettingsMailingLists {
			return m.updateMailingLists(msg)
		} else {
			return m.updateAccounts(msg)
		}
	}
	return m, nil
}

func (m *Settings) updateMain(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		// Options: 0: Email Accounts, 1: Image Display, 2: Edit Signature, 3: Contextual Tips, 4: Mailing Lists
		if m.cursor < 4 {
			m.cursor++
		}
	case "enter":
		switch m.cursor {
		case 0: // Email Accounts
			m.state = SettingsAccounts
			m.cursor = 0
			return m, nil
		case 1: // Image Display
			m.cfg.DisableImages = !m.cfg.DisableImages
			// Save config immediately
			_ = config.SaveConfig(m.cfg)
			return m, nil
		case 2: // Edit Signature
			return m, func() tea.Msg { return GoToSignatureEditorMsg{} }
		case 3: // Contextual Tips
			m.cfg.HideTips = !m.cfg.HideTips
			// Save config immediately
			_ = config.SaveConfig(m.cfg)
			return m, nil
		case 4: // Mailing Lists
			m.state = SettingsMailingLists
			m.cursor = 0
			return m, nil
		}
	case "esc":
		return m, func() tea.Msg { return GoToChoiceMenuMsg{} }
	}
	return m, nil
}

func (m *Settings) updateAccounts(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.confirmingDelete {
		switch msg.String() {
		case "y", "Y":
			if m.cursor < len(m.cfg.Accounts) {
				accountID := m.cfg.Accounts[m.cursor].ID
				m.confirmingDelete = false
				return m, func() tea.Msg {
					return DeleteAccountMsg{AccountID: accountID}
				}
			}
		case "n", "N", "esc":
			m.confirmingDelete = false
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		// +1 for "Add Account" option
		if m.cursor < len(m.cfg.Accounts) {
			m.cursor++
		}
	case "d":
		// Delete selected account (not the "Add Account" option)
		if m.cursor < len(m.cfg.Accounts) && len(m.cfg.Accounts) > 0 {
			m.confirmingDelete = true
		}
	case "enter":
		// If cursor is on "Add Account"
		if m.cursor == len(m.cfg.Accounts) {
			return m, func() tea.Msg { return GoToAddAccountMsg{} }
		}
	case "esc":
		m.state = SettingsMain
		m.cursor = 0
		return m, nil
	}
	return m, nil
}

func (m *Settings) updateMailingLists(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.confirmingDelete {
		switch msg.String() {
		case "y", "Y":
			if m.cursor < len(m.cfg.MailingLists) {
				m.cfg.MailingLists = append(m.cfg.MailingLists[:m.cursor], m.cfg.MailingLists[m.cursor+1:]...)
				_ = config.SaveConfig(m.cfg)
				if m.cursor >= len(m.cfg.MailingLists) && m.cursor > 0 {
					m.cursor--
				}
				m.confirmingDelete = false
			}
		case "n", "N", "esc":
			m.confirmingDelete = false
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.cfg.MailingLists) {
			m.cursor++
		}
	case "d":
		if m.cursor < len(m.cfg.MailingLists) && len(m.cfg.MailingLists) > 0 {
			m.confirmingDelete = true
		}
	case "enter":
		if m.cursor == len(m.cfg.MailingLists) {
			return m, func() tea.Msg { return GoToAddMailingListMsg{} }
		}
	case "esc":
		m.state = SettingsMain
		m.cursor = 0
		return m, nil
	}
	return m, nil
}

// View renders the settings screen.
func (m *Settings) View() tea.View {
	if m.state == SettingsMain {
		return tea.NewView(m.viewMain())
	} else if m.state == SettingsMailingLists {
		return tea.NewView(m.viewMailingLists())
	}
	return tea.NewView(m.viewAccounts())
}

func (m *Settings) viewMain() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Settings") + "\n\n")

	// Option 0: Email Accounts
	if m.cursor == 0 {
		b.WriteString(selectedAccountItemStyle.Render("> Email Accounts"))
	} else {
		b.WriteString(accountItemStyle.Render("  Email Accounts"))
	}
	b.WriteString("\n")

	// Option 1: Image Display
	status := "ON"
	if m.cfg.DisableImages {
		status = "OFF"
	}
	text := fmt.Sprintf("Image Display: %s", status)
	if m.cursor == 1 {
		b.WriteString(selectedAccountItemStyle.Render("> " + text))
	} else {
		b.WriteString(accountItemStyle.Render("  " + text))
	}
	b.WriteString("\n")

	// Option 2: Edit Signature
	sigText := "Edit Signature"
	if config.HasSignature() {
		sigText = "Edit Signature (configured)"
	}
	if m.cursor == 2 {
		b.WriteString(selectedAccountItemStyle.Render("> " + sigText))
	} else {
		b.WriteString(accountItemStyle.Render("  " + sigText))
	}
	b.WriteString("\n")

	// Option 3: Contextual Tips
	tipsStatus := "ON"
	if m.cfg.HideTips {
		tipsStatus = "OFF"
	}
	tipsText := fmt.Sprintf("Contextual Tips: %s", tipsStatus)
	if m.cursor == 3 {
		b.WriteString(selectedAccountItemStyle.Render("> " + tipsText))
	} else {
		b.WriteString(accountItemStyle.Render("  " + tipsText))
	}
	b.WriteString("\n")

	// Option 4: Mailing Lists
	mailingListsText := "Mailing Lists"
	if m.cursor == 4 {
		b.WriteString(selectedAccountItemStyle.Render("> " + mailingListsText))
	} else {
		b.WriteString(accountItemStyle.Render("  " + mailingListsText))
	}
	b.WriteString("\n\n")

	if !m.cfg.HideTips {
		tip := ""
		switch m.cursor {
		case 0:
			tip = "Manage your connected email accounts."
		case 1:
			tip = "Toggle displaying images in emails."
		case 2:
			tip = "Configure the signature appended to your outgoing emails."
		case 3:
			tip = "Toggle displaying helpful contextual tips like this one."
		case 4:
			tip = "Manage groups of email addresses to quickly send to multiple people."
		}
		if tip != "" {
			b.WriteString(TipStyle.Render("Tip: "+tip) + "\n\n")
		}
	}

	mainContent := b.String()
	helpView := helpStyle.Render("↑/↓: navigate • enter: select/toggle • esc: back")

	if m.height > 0 {
		currentHeight := lipgloss.Height(docStyle.Render(mainContent + helpView))
		gap := m.height - currentHeight
		if gap > 0 {
			mainContent += strings.Repeat("\n", gap)
		}
	} else {
		mainContent += "\n\n"
	}

	return docStyle.Render(mainContent + helpView)
}

func (m *Settings) viewAccounts() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Account Settings"))
	b.WriteString("\n\n")

	if len(m.cfg.Accounts) == 0 {
		b.WriteString(accountEmailStyle.Render("  No accounts configured.\n"))
		b.WriteString("\n")
	}

	for i, account := range m.cfg.Accounts {
		displayName := account.Email
		if account.Name != "" {
			displayName = fmt.Sprintf("%s (%s)", account.Name, account.FetchEmail)
		}

		providerInfo := account.ServiceProvider
		if account.ServiceProvider == "custom" {
			providerInfo = fmt.Sprintf("custom: %s", account.IMAPServer)
		}

		line := fmt.Sprintf("%s - %s", displayName, accountEmailStyle.Render(providerInfo))

		if m.cursor == i {
			b.WriteString(selectedAccountItemStyle.Render(fmt.Sprintf("> %s", line)))
		} else {
			b.WriteString(accountItemStyle.Render(fmt.Sprintf("  %s", line)))
		}
		b.WriteString("\n")
	}

	// Add Account option
	addAccountText := "Add New Account"
	if m.cursor == len(m.cfg.Accounts) {
		b.WriteString(selectedAccountItemStyle.Render(fmt.Sprintf("> %s", addAccountText)))
	} else {
		b.WriteString(accountItemStyle.Render(fmt.Sprintf("  %s", addAccountText)))
	}
	b.WriteString("\n")

	mainContent := b.String()
	helpView := helpStyle.Render("↑/↓: navigate • enter: select • d: delete account • esc: back")

	if m.height > 0 {
		currentHeight := lipgloss.Height(docStyle.Render(mainContent + helpView))
		gap := m.height - currentHeight
		if gap > 0 {
			mainContent += strings.Repeat("\n", gap)
		}
	} else {
		mainContent += "\n\n"
	}

	if m.confirmingDelete {
		accountName := m.cfg.Accounts[m.cursor].Email
		dialog := DialogBoxStyle.Render(
			lipgloss.JoinVertical(lipgloss.Center,
				dangerStyle.Render("Delete account?"),
				accountEmailStyle.Render(accountName),
				HelpStyle.Render("\n(y/n)"),
			),
		)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
	}

	return docStyle.Render(mainContent + helpView)
}

func (m *Settings) viewMailingLists() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Mailing Lists"))
	b.WriteString("\n\n")

	if len(m.cfg.MailingLists) == 0 {
		b.WriteString(accountEmailStyle.Render("  No mailing lists configured.\n"))
		b.WriteString("\n")
	}

	for i, list := range m.cfg.MailingLists {
		displayName := list.Name

		addrCount := fmt.Sprintf("%d address", len(list.Addresses))
		if len(list.Addresses) != 1 {
			addrCount += "es"
		}

		line := fmt.Sprintf("%s - %s", displayName, accountEmailStyle.Render(addrCount))

		if m.cursor == i {
			b.WriteString(selectedAccountItemStyle.Render(fmt.Sprintf("> %s", line)))
		} else {
			b.WriteString(accountItemStyle.Render(fmt.Sprintf("  %s", line)))
		}
		b.WriteString("\n")
	}

	// Add Mailing List option
	addListText := "Add New Mailing List"
	if m.cursor == len(m.cfg.MailingLists) {
		b.WriteString(selectedAccountItemStyle.Render(fmt.Sprintf("> %s", addListText)))
	} else {
		b.WriteString(accountItemStyle.Render(fmt.Sprintf("  %s", addListText)))
	}
	b.WriteString("\n")

	helpView := helpStyle.Render("↑/↓: navigate • enter: select • d: delete • esc: back")
	mainContent := b.String()

	if m.height > 0 {
		contentHeight := strings.Count(mainContent, "\n") + 1
		gap := m.height - contentHeight - 2 // -2 for margins
		if gap > 0 {
			mainContent += strings.Repeat("\n", gap)
		}
	} else {
		mainContent += "\n\n"
	}

	if m.confirmingDelete {
		listName := m.cfg.MailingLists[m.cursor].Name
		dialog := DialogBoxStyle.Render(
			lipgloss.JoinVertical(lipgloss.Center,
				dangerStyle.Render("Delete mailing list?"),
				accountEmailStyle.Render(listName),
				HelpStyle.Render("\n(y/n)"),
			),
		)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
	}

	return docStyle.Render(mainContent + helpView)
}

// UpdateConfig updates the configuration (used when accounts are deleted).
func (m *Settings) UpdateConfig(cfg *config.Config) {
	m.cfg = cfg
	if m.state == SettingsAccounts && m.cursor >= len(cfg.Accounts) {
		m.cursor = len(cfg.Accounts)
	}
}
