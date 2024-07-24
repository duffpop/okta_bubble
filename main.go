package main

import (
	"context"
	"fmt"
	"log"

	"github.com/charmbracelet/bubbles/list"
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
	if err, done := tea.NewProgram(p).Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		return
	} else if done != nil {
		fmt.Println("Program exited without error")
	}
}

type model struct {
	list      list.Model
	err       error
	oktaUsers []oktaUser
	isLoading bool
}

func newProgram() (*model, error) {
	m := &model{
		list:      list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0),
		isLoading: true,
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
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		h, v := appStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *model) View() string {
	return appStyle.Render(m.list.View())
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
	ctx, client, err := okta.NewClient(
		context.TODO(),
		okta.WithOrgUrl(oktaOrgURL),
		okta.WithToken(oktaAPIToken),
	)

	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}

	fmt.Printf("Context: %+v\n Client: %+v\n", ctx, client)
	users, _, err := client.User.ListUsers(context.TODO(), nil)
	if err != nil {
		return fmt.Errorf("failed to list Okta users: %w", err)
	}

	m.oktaUsers = make([]oktaUser, len(users))
	for i, user := range users {
		// ... existing code ...
		if user.Profile != nil {
			if login, ok := (*user.Profile)["login"].(string); ok {
				m.oktaUsers[i] = oktaUser{
					Login: login,
				}
			} else {
				// Handle the case where "login" is not found or not a string
				m.oktaUsers[i] = oktaUser{
					Login: "unknown",
				}
			}
		} else {
			// Handle the case where user.Profile is nil
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
