package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/config"
)

func (m *Settings) updateContacts(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.contactsConfirming {
		switch msg.String() {
		case "y", "Y":
			if m.contactsCursor < len(m.contactsList) {
				_ = config.DeleteContact(m.contactsList[m.contactsCursor].Email)
				m.contactsList = config.GetNamedContacts()
				if m.contactsCursor >= len(m.contactsList) && m.contactsCursor > 0 {
					m.contactsCursor--
				}
				m.contactsConfirming = false
			}
		case "n", "N", "esc":
			m.contactsConfirming = false
		}
		return m, nil
	}

	itemCount := len(m.contactsList) + 1 // +1 for "Add New Contact"

	switch msg.String() {
	case "up", "k":
		m.contactsCursor = (m.contactsCursor - 1 + itemCount) % itemCount
	case keyDown, "j":
		m.contactsCursor = (m.contactsCursor + 1) % itemCount
	case "d":
		if m.contactsCursor < len(m.contactsList) && len(m.contactsList) > 0 {
			m.contactsConfirming = true
		}
	case "e":
		if m.contactsCursor < len(m.contactsList) {
			c := m.contactsList[m.contactsCursor]
			return m, func() tea.Msg {
				return GoToEditContactMsg{
					OriginalEmail: c.Email,
					Name:          c.Name,
					Email:         c.Email,
				}
			}
		}
	case keyEnter:
		if m.contactsCursor == len(m.contactsList) {
			return m, func() tea.Msg { return GoToAddContactMsg{} }
		}
		if m.contactsCursor < len(m.contactsList) {
			c := m.contactsList[m.contactsCursor]
			return m, func() tea.Msg {
				return GoToEditContactMsg{
					OriginalEmail: c.Email,
					Name:          c.Name,
					Email:         c.Email,
				}
			}
		}
	}
	return m, nil
}

func (m *Settings) viewContacts() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(t("settings_contacts.title")) + "\n\n")

	if len(m.contactsList) == 0 {
		b.WriteString(accountEmailStyle.Render("  "+t("settings_contacts.no_contacts")) + "\n\n")
	}

	for i, c := range m.contactsList {
		line := fmt.Sprintf("%s  %s", c.Name, accountEmailStyle.Render("<"+c.Email+">"))
		selected := m.contactsCursor == i
		cursor := m.contentCursor(selected)
		style := m.contentItemStyle(selected)
		b.WriteString(style.Render(cursor+line) + "\n")
	}

	selected := m.contactsCursor == len(m.contactsList)
	cursor := m.contentCursor(selected)
	style := m.contentItemStyle(selected)
	b.WriteString(style.Render(cursor+t("settings_contacts.add_contact")) + "\n\n")

	b.WriteString(helpStyle.Render(t("settings_contacts.help")))

	if m.contactsConfirming && m.contactsCursor < len(m.contactsList) {
		contactName := m.contactsList[m.contactsCursor].Name
		dialog := DialogBoxStyle.Render(
			lipgloss.JoinVertical(lipgloss.Center,
				dangerStyle.Render(t("settings_contacts.delete_confirm")),
				accountEmailStyle.Render(contactName),
				HelpStyle.Render("\n(y/n)"),
			),
		)
		b.WriteString("\n\n" + dialog)
	}

	return b.String()
}
