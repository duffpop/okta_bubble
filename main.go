package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/okta/okta-sdk-golang/v2/okta"
)

const (
	oktaOrgURL   = "https://dev-21460697.okta.com"              // Update with your Okta org URL
	oktaAPIToken = "002sKQ11oskWXaG150Q2-ichuW4lrO7WwxfWQbjUmM" // Update with a valid API token for your Okta org
)

type oktaUser struct {
	Login string `json:"login"`
}

func main() {
	p, err := newProgram()
	if err != nil {
		log.Fatalf("Failed to initialize program: %v", err)
	}

	// Create a new Program with the quiet renderer option
	prog := tea.NewProgram(p, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := prog.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
	}
}

type model struct {
	list         list.Model
	err          error
	oktaUsers    []oktaUser
	isLoading    bool
	selectedUser *okta.User
	viewport     viewport.Model
	client       *okta.Client
}

func newProgram() (*model, error) {
	_, client, err := okta.NewClient(
		context.TODO(),
		okta.WithOrgUrl(oktaOrgURL),
		okta.WithToken(oktaAPIToken),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Okta client: %w", err)
	}

	m := &model{
		list:      list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0),
		isLoading: true,
		client:    client,
		viewport:  viewport.New(0, 0),
	}

	// Fetch Okta users and initialize the model's list with them.
	if err := m.loadOktaUsers(); err != nil {
		return nil, fmt.Errorf("failed to load Okta users: %w", err)
	}
	m.isLoading = false

	return m, nil
}

func (m *model) Init() tea.Cmd {
	return nil // No initialization commands needed for this example.
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := appStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
		m.viewport.Width = msg.Width - h
		m.viewport.Height = msg.Height - v
	case tea.KeyMsg:
		if m.selectedUser != nil {
			if msg.String() == "esc" {
				m.selectedUser = nil
				return m, nil
			}
			// Handle viewport scrolling
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

		if msg.String() == "enter" {
			i, ok := m.list.SelectedItem().(oktaUser)
			if ok {
				return m, m.fetchUserProfile(i.Login)
			}
		}
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case *okta.User:
		m.selectedUser = msg
		m.viewport.SetContent(formatUserProfile(msg))
		return m, nil
	case error:
		m.err = msg
		return m, nil
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *model) View() string {
	if m.selectedUser != nil {
		return m.viewport.View()
	}
	return appStyle.Render(m.list.View())
}

func (m *model) fetchUserProfile(login string) tea.Cmd {
	return func() tea.Msg {
		user, _, err := m.client.User.GetUser(context.TODO(), login)
		if err != nil {
			return err
		}
		return user
	}
}

var appStyle = lipgloss.NewStyle().Padding(1, 2)

// func (m *model) View() string {
// 	if m.err != nil {
// 		return fmt.Sprintf("%s\n\nPress 'q' or Ctrl+C to quit.", m.err)
// 	}

// 	view := m.list.View()
// 	if m.isLoading {
// 		view += "\nLoading..."
// 	}
// 	return view
// }

// Add this method
func (u oktaUser) FilterValue() string {
	return u.Login
}

// Add this method
func (u oktaUser) Title() string { return u.Login }

// Add this method
func (u oktaUser) Description() string { return "" }

func (m *model) loadOktaUsers() error {
	users, _, err := m.client.User.ListUsers(context.TODO(), nil)
	if err != nil {
		return fmt.Errorf("failed to list Okta users: %w", err)
	}

	m.oktaUsers = make([]oktaUser, len(users))
	for i, user := range users {
		if user.Profile != nil {
			if login, ok := (*user.Profile)["login"].(string); ok {
				m.oktaUsers[i] = oktaUser{
					Login: login,
				}
			} else {
				m.oktaUsers[i] = oktaUser{
					Login: "unknown",
				}
			}
		} else {
			m.oktaUsers[i] = oktaUser{
				Login: "profile not available",
			}
		}
	}

	items := make([]list.Item, len(m.oktaUsers))
	for i, user := range m.oktaUsers {
		items[i] = user
	}
	m.list.SetItems(items)
	m.list.Title = "Okta Users"

	return nil
}

func formatUserProfile(user *okta.User) string {
	var sb strings.Builder
	if user.Profile != nil {
		if login, ok := (*user.Profile)["login"].(string); ok {
			sb.WriteString(fmt.Sprintf("User Profile for %s\n\n", login))
		} else {
			sb.WriteString("User Profile (login unknown)\n\n")
		}
		for k, v := range *user.Profile {
			sb.WriteString(fmt.Sprintf("%s: %v\n", k, v))
		}
	} else {
		sb.WriteString("User Profile not available\n")
	}
	sb.WriteString("\nPress 'esc' to go back to the user list.")
	return sb.String()
}
