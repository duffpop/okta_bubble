package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/okta/okta-sdk-golang/v2/okta"
)

const (
	oktaOrgURLEnv   = "OKTA_ORG_URL"
	oktaAPITokenEnv = "OKTA_API_TOKEN"
)

type oktaUser struct {
	Login string `json:"login"`
}

func main() {
	oktaOrgURL := os.Getenv(oktaOrgURLEnv)
	oktaAPIToken := os.Getenv(oktaAPITokenEnv)

	if oktaOrgURL == "" || oktaAPIToken == "" {
		log.Fatalf("Environment variables %s and %s must be set", oktaOrgURLEnv, oktaAPITokenEnv)
	}

	p, err := newProgram(oktaOrgURL, oktaAPIToken)
	if err != nil {
		log.Fatalf("Failed to initialise program: %v", err)
	}

	// quiet renderer (avoids debug info)
	prog := tea.NewProgram(p, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := prog.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
	}
}

type appState int

const (
	listView appState = iota
	userDetailView
	createUserView
)

type model struct {
	list         list.Model
	err          error
	oktaUsers    []oktaUser
	isLoading    bool
	selectedUser *okta.User
	viewport     viewport.Model
	client       *okta.Client
	state        appState
	createForm   createUserForm
}

type createUserForm struct {
	inputs       []textinput.Model
	focused      int
	fieldNames   []string
	required     []bool
	values       map[string]string
	creating     bool
}

func newProgram(oktaOrgURL, oktaAPIToken string) (*model, error) {
	_, client, err := okta.NewClient(
		context.TODO(),
		okta.WithOrgUrl(oktaOrgURL),
		okta.WithToken(oktaAPIToken),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Okta client: %w", err)
	}

	m := &model{
		list:       list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0),
		isLoading:  true,
		client:     client,
		viewport:   viewport.New(0, 0),
		state:      listView,
		createForm: newCreateUserForm(),
	}

	// fetch okta users and init list with them
	if err := m.loadOktaUsers(); err != nil {
		return nil, fmt.Errorf("failed to load Okta users: %w", err)
	}
	m.isLoading = false

	return m, nil
}

func newCreateUserForm() createUserForm {
	fieldNames := []string{"login", "firstName", "lastName", "email", "displayName", "middleName", "title", "department"}
	required := []bool{true, true, true, true, false, false, false, false}
	inputs := make([]textinput.Model, len(fieldNames))
	
	for i := range inputs {
		inputs[i] = textinput.New()
		inputs[i].Placeholder = fieldNames[i]
		if required[i] {
			inputs[i].Placeholder += " (required)"
		}
		inputs[i].CharLimit = 256
		inputs[i].Width = 50
	}
	
	inputs[0].Focus()
	
	return createUserForm{
		inputs:     inputs,
		focused:    0,
		fieldNames: fieldNames,
		required:   required,
		values:     make(map[string]string),
		creating:   false,
	}
}

func (m *model) Init() tea.Cmd {
	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := appStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
		m.viewport.Width = msg.Width - h
		m.viewport.Height = msg.Height - v
		
	case tea.KeyMsg:
		switch m.state {
		case listView:
			return m.updateListView(msg)
		case userDetailView:
			return m.updateUserDetailView(msg)
		case createUserView:
			return m.updateCreateUserView(msg)
		}
		
	case *okta.User:
		m.selectedUser = msg
		m.state = userDetailView
		m.viewport.SetContent(formatUserProfile(msg))
		return m, nil
		
	case userCreatedMsg:
		m.state = listView
		m.createForm = newCreateUserForm()
		return m, m.reloadUsers()
		
	case error:
		m.err = msg
		return m, nil
	}
	
	var cmd tea.Cmd
	if m.state == listView {
		m.list, cmd = m.list.Update(msg)
	}
	return m, cmd
}

type userCreatedMsg struct{}

func (m *model) updateListView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		i, ok := m.list.SelectedItem().(oktaUser)
		if ok {
			return m, m.fetchUserProfile(i.Login)
		}
	case "n", "ctrl+n":
		m.state = createUserView
		return m, nil
	case "q", "ctrl+c":
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *model) updateUserDetailView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.selectedUser = nil
		m.state = listView
		return m, nil
	case "q", "ctrl+c":
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m *model) updateCreateUserView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = listView
		m.createForm = newCreateUserForm()
		return m, nil
	case "tab", "down":
		m.createForm.inputs[m.createForm.focused].Blur()
		m.createForm.focused = (m.createForm.focused + 1) % len(m.createForm.inputs)
		m.createForm.inputs[m.createForm.focused].Focus()
		return m, nil
	case "shift+tab", "up":
		m.createForm.inputs[m.createForm.focused].Blur()
		m.createForm.focused = (m.createForm.focused - 1 + len(m.createForm.inputs)) % len(m.createForm.inputs)
		m.createForm.inputs[m.createForm.focused].Focus()
		return m, nil
	case "enter":
		if m.canCreateUser() && !m.createForm.creating {
			m.createForm.creating = true
			return m, m.createUser()
		}
	case "q", "ctrl+c":
		return m, tea.Quit
	}
	
	var cmd tea.Cmd
	m.createForm.inputs[m.createForm.focused], cmd = m.createForm.inputs[m.createForm.focused].Update(msg)
	return m, cmd
}

func (m *model) View() string {
	switch m.state {
	case listView:
		return appStyle.Render(m.list.View() + "\n\nPress 'n' to create new user, 'enter' to view details, 'q' to quit")
	case userDetailView:
		return m.viewport.View()
	case createUserView:
		return m.renderCreateUserForm()
	}
	return ""
}

func (m *model) canCreateUser() bool {
	for i, required := range m.createForm.required {
		if required && strings.TrimSpace(m.createForm.inputs[i].Value()) == "" {
			return false
		}
	}
	return true
}

func (m *model) renderCreateUserForm() string {
	var b strings.Builder
	b.WriteString("Create New User\n\n")
	
	for i, input := range m.createForm.inputs {
		fieldName := m.createForm.fieldNames[i]
		if m.createForm.required[i] {
			fieldName += " *"
		}
		b.WriteString(fmt.Sprintf("%s:\n", fieldName))
		b.WriteString(input.View())
		b.WriteString("\n\n")
	}
	
	if m.createForm.creating {
		b.WriteString("Creating user...")
	} else if m.canCreateUser() {
		b.WriteString("Press 'enter' to create user")
	} else {
		b.WriteString("Please fill in all required fields (*)")
	}
	
	b.WriteString("\n\nPress 'esc' to cancel, 'tab'/'down' to next field, 'shift+tab'/'up' to previous field")
	
	return appStyle.Render(b.String())
}

func (m *model) createUser() tea.Cmd {
	return func() tea.Msg {
		profile := make(map[string]interface{})
		
		for i, fieldName := range m.createForm.fieldNames {
			value := strings.TrimSpace(m.createForm.inputs[i].Value())
			if value != "" {
				profile[fieldName] = value
			}
		}
		
		userProfile := okta.UserProfile(profile)
		createUserRequest := okta.CreateUserRequest{
			Profile: &userProfile,
		}
		
		_, _, err := m.client.User.CreateUser(context.TODO(), createUserRequest, nil)
		if err != nil {
			return err
		}
		
		return userCreatedMsg{}
	}
}

func (m *model) reloadUsers() tea.Cmd {
	return func() tea.Msg {
		if err := m.loadOktaUsers(); err != nil {
			return err
		}
		return nil
	}
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

func (u oktaUser) FilterValue() string {
	return u.Login
}

func (u oktaUser) Title() string { return u.Login }

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
