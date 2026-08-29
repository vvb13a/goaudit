package tui

import (
	"fmt"
	"strings"

	"github.com/vvb13a/goaudit/config"
	"github.com/vvb13a/goaudit/data"
	"github.com/vvb13a/goaudit/db"
	"github.com/vvb13a/goaudit/engine"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type ViewState int

const (
	ViewInspectionsTab ViewState = iota
	ViewChecklistsTab
	ViewPlansTab
	ViewChecklistForm
	ViewPlanForm
	ViewPromptURL
	ViewInspecting
	ViewIssues
	ViewIssueDetail
	ViewConfirmDelete
)

type pendingDelete struct {
	itemType string
	id       string
	name     string
	index    int
	returnTo ViewState
}

type Model struct {
	state             ViewState
	database          *db.DB
	fetcher           *engine.Fetcher
	cfg               *config.Config
	allChecks         []data.Check
	inspections       []*data.Inspection
	checklists        []*data.Checklist
	plans             []*data.Plan
	defaultChecklist  *data.Checklist
	selectedInsp      *data.Inspection
	currentIssues     []data.Issue
	selectedIssue     *data.Issue
	pendingDel        pendingDelete
	inspectionsTable  table.Model
	checklistsTable   table.Model
	plansTable        table.Model
	issuesTable       table.Model
	textInput         textinput.Model
	spinner           spinner.Model
	progressBar       progress.Model
	progressChan      chan tea.Msg
	checklistForm     CreateChecklistForm
	planForm          CreatePlanForm
	statusMsg         string
	inspectTitle      string
	currentInspectURL string
	completedCount    int
	totalCount        int
}

func New(
	database *db.DB,
	fetcher *engine.Fetcher,
	cfg *config.Config,
	allChecks []data.Check,
	inspections []*data.Inspection,
	checklists []*data.Checklist,
	plans []*data.Plan,
) Model {
	var defaultChk *data.Checklist
	for _, c := range checklists {
		if c.IsDefault {
			defaultChk = c
			break
		}
	}

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(50),
	)

	return Model{
		state:            ViewInspectionsTab,
		database:         database,
		fetcher:          fetcher,
		cfg:              cfg,
		allChecks:        allChecks,
		inspections:      inspections,
		checklists:       checklists,
		plans:            plans,
		defaultChecklist: defaultChk,
		inspectionsTable: BuildInspectionsTable(inspections),
		checklistsTable:  BuildChecklistsTable(checklists),
		plansTable:       BuildPlansTable(plans),
		textInput:        NewURLInput(),
		spinner:          s,
		progressBar:      p,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}

		// Global Tab Switching across Tab 1, 2, 3
		if m.state == ViewInspectionsTab || m.state == ViewChecklistsTab || m.state == ViewPlansTab {
			switch msg.String() {
			case "tab":
				switch m.state {
				case ViewInspectionsTab:
					m.state = ViewChecklistsTab
				case ViewChecklistsTab:
					m.state = ViewPlansTab
				case ViewPlansTab:
					m.state = ViewInspectionsTab
				}
				m.statusMsg = ""
				return m, nil

			case "shift+tab", "backtab":
				switch m.state {
				case ViewInspectionsTab:
					m.state = ViewPlansTab
				case ViewChecklistsTab:
					m.state = ViewInspectionsTab
				case ViewPlansTab:
					m.state = ViewChecklistsTab
				}
				m.statusMsg = ""
				return m, nil

			case "1":
				m.state = ViewInspectionsTab
				m.statusMsg = ""
				return m, nil
			case "2":
				m.state = ViewChecklistsTab
				m.statusMsg = ""
				return m, nil
			case "3":
				m.state = ViewPlansTab
				m.statusMsg = ""
				return m, nil
			}
		}

		switch m.state {
		// -------------------------------------------------------------
		// TAB 1: INSPECTIONS
		// -------------------------------------------------------------
		case ViewInspectionsTab:
			switch msg.String() {
			case "q":
				return m, tea.Quit

			case "n":
				m.state = ViewPromptURL
				m.textInput.Reset()
				m.textInput.Focus()
				m.statusMsg = ""
				return m, textinput.Blink

			case "r":
				if len(m.inspections) > 0 {
					idx := m.inspectionsTable.Cursor()
					if idx < len(m.inspections) {
						insp := m.inspections[idx]
						var urls []string
						for _, rep := range insp.Reports {
							urls = append(urls, rep.URL)
						}
						return m.startInspectionRun(insp.PlanName, urls)
					}
				}
				return m, nil

			case "d", "x":
				if len(m.inspections) > 0 {
					idx := m.inspectionsTable.Cursor()
					if idx < len(m.inspections) {
						insp := m.inspections[idx]
						m.pendingDel = pendingDelete{
							itemType: "inspection",
							id:       insp.ID,
							name:     insp.PlanName,
							index:    idx,
							returnTo: ViewInspectionsTab,
						}
						m.state = ViewConfirmDelete
					}
				}
				return m, nil

			case "right", "enter", "l":
				if len(m.inspections) > 0 {
					idx := m.inspectionsTable.Cursor()
					if idx < len(m.inspections) {
						m.selectedInsp = m.inspections[idx]
						m.currentIssues = GetSortedIssues(m.selectedInsp)
						m.issuesTable = BuildIssuesTable(m.currentIssues)
						m.state = ViewIssues
					}
				}
				return m, nil
			}

		// -------------------------------------------------------------
		// TAB 2: CHECKLISTS
		// -------------------------------------------------------------
		case ViewChecklistsTab:
			switch msg.String() {
			case "q":
				return m, tea.Quit

			case "n", "c":
				m.checklistForm = NewCreateChecklistForm(m.allChecks)
				m.state = ViewChecklistForm
				m.statusMsg = ""
				return m, nil

			case "right", "enter", "l", "e":
				// Open Checklist in Edit mode
				if len(m.checklists) > 0 {
					idx := m.checklistsTable.Cursor()
					if idx < len(m.checklists) {
						m.checklistForm = NewEditChecklistForm(m.allChecks, m.checklists[idx])
						m.state = ViewChecklistForm
						m.statusMsg = ""
					}
				}
				return m, nil

			case " ":
				// Toggle as default checklist
				if len(m.checklists) > 0 {
					idx := m.checklistsTable.Cursor()
					if idx < len(m.checklists) {
						selected := m.checklists[idx]
						_ = m.database.SetDefaultChecklist(selected.ID)
						m.checklists, _ = m.database.ListChecklists()
						for _, c := range m.checklists {
							if c.IsDefault {
								m.defaultChecklist = c
								break
							}
						}
						m.checklistsTable = BuildChecklistsTable(m.checklists)
						m.statusMsg = SuccessStyle.Render(fmt.Sprintf("Default checklist set to '%s'", selected.Name))
					}
				}
				return m, nil

			case "d", "x":
				if len(m.checklists) > 0 {
					idx := m.checklistsTable.Cursor()
					if idx < len(m.checklists) {
						chk := m.checklists[idx]
						m.pendingDel = pendingDelete{
							itemType: "checklist",
							id:       chk.ID,
							name:     chk.Name,
							index:    idx,
							returnTo: ViewChecklistsTab,
						}
						m.state = ViewConfirmDelete
					}
				}
				return m, nil
			}

		// -------------------------------------------------------------
		// TAB 3: PLANS
		// -------------------------------------------------------------
		case ViewPlansTab:
			switch msg.String() {
			case "q":
				return m, tea.Quit

			case "r":
				// 'r' strictly runs the plan
				if len(m.plans) > 0 {
					idx := m.plansTable.Cursor()
					if idx < len(m.plans) && len(m.plans[idx].URLs) > 0 {
						targetPlan := m.plans[idx]
						return m.startInspectionRun(targetPlan.Name, targetPlan.URLs)
					}
				}
				return m, nil

			case "right", "enter", "l", "e":
				// '→' or 'Enter' opens the plan in Edit mode
				if len(m.plans) > 0 {
					idx := m.plansTable.Cursor()
					if idx < len(m.plans) {
						m.planForm = NewEditPlanForm(m.plans[idx])
						m.state = ViewPlanForm
						m.statusMsg = ""
					}
				}
				return m, nil

			case "n", "c":
				m.planForm = NewCreatePlanForm()
				m.state = ViewPlanForm
				m.statusMsg = ""
				return m, nil

			case "d", "x":
				if len(m.plans) > 0 {
					idx := m.plansTable.Cursor()
					if idx < len(m.plans) {
						p := m.plans[idx]
						m.pendingDel = pendingDelete{
							itemType: "plan",
							id:       p.ID,
							name:     p.Name,
							index:    idx,
							returnTo: ViewPlansTab,
						}
						m.state = ViewConfirmDelete
					}
				}
				return m, nil
			}

		// -------------------------------------------------------------
		// CONFIRM DELETION MODAL
		// -------------------------------------------------------------
		case ViewConfirmDelete:
			switch msg.String() {
			case "y", "Y":
				switch m.pendingDel.itemType {
				case "inspection":
					_ = m.database.DeleteInspection(m.pendingDel.id)
					m.inspections, _ = m.database.ListInspections()
					m.inspectionsTable = BuildInspectionsTable(m.inspections)
					m.statusMsg = SuccessStyle.Render(fmt.Sprintf("🗑️ Deleted inspection '%s'", m.pendingDel.name))

				case "checklist":
					_ = m.database.DeleteChecklist(m.pendingDel.id)
					m.checklists, _ = m.database.ListChecklists()
					m.checklistsTable = BuildChecklistsTable(m.checklists)
					m.statusMsg = SuccessStyle.Render(fmt.Sprintf("🗑️ Deleted checklist '%s'", m.pendingDel.name))

				case "plan":
					_ = m.database.DeletePlan(m.pendingDel.id)
					m.plans, _ = m.database.ListPlans()
					m.plansTable = BuildPlansTable(m.plans)
					m.statusMsg = SuccessStyle.Render(fmt.Sprintf("🗑️ Deleted plan '%s'", m.pendingDel.name))
				}

				m.state = m.pendingDel.returnTo
				return m, nil

			case "n", "N", "esc", "left", "h":
				m.state = m.pendingDel.returnTo
				m.statusMsg = ""
				return m, nil
			}

		// -------------------------------------------------------------
		// FORM: CHECKLIST (CREATE / EDIT)
		// -------------------------------------------------------------
		case ViewChecklistForm:
			switch msg.Type {
			case tea.KeyEsc:
				m.state = ViewChecklistsTab
				return m, nil

			case tea.KeyTab, tea.KeyShiftTab:
				m.checklistForm.InName = !m.checklistForm.InName
				if m.checklistForm.InName {
					m.checklistForm.NameInput.Focus()
				} else {
					m.checklistForm.NameInput.Blur()
				}
				return m, nil

			case tea.KeyUp:
				if !m.checklistForm.InName && m.checklistForm.Cursor > 0 {
					m.checklistForm.Cursor--
				}
				return m, nil

			case tea.KeyDown:
				if !m.checklistForm.InName && m.checklistForm.Cursor < len(m.checklistForm.Options)-1 {
					m.checklistForm.Cursor++
				}
				return m, nil

			case tea.KeySpace:
				if !m.checklistForm.InName {
					m.checklistForm.Options[m.checklistForm.Cursor].Selected = !m.checklistForm.Options[m.checklistForm.Cursor].Selected
				}
				return m, nil

			case tea.KeyCtrlS, tea.KeyEnter:
				name := strings.TrimSpace(m.checklistForm.NameInput.Value())
				if name == "" {
					return m, nil
				}
				var chosen []string
				for _, opt := range m.checklistForm.Options {
					if opt.Selected {
						chosen = append(chosen, opt.Check.Name())
					}
				}

				chk := &data.Checklist{
					ID:         m.checklistForm.ID,
					Name:       name,
					CheckNames: chosen,
					IsDefault:  m.checklistForm.ID == "" || (m.defaultChecklist != nil && m.defaultChecklist.ID == m.checklistForm.ID),
				}
				_ = m.database.SaveChecklist(chk)
				m.checklists, _ = m.database.ListChecklists()
				m.checklistsTable = BuildChecklistsTable(m.checklists)
				m.state = ViewChecklistsTab

				if m.checklistForm.ID != "" {
					m.statusMsg = SuccessStyle.Render(fmt.Sprintf("Checklist '%s' updated!", name))
				} else {
					m.statusMsg = SuccessStyle.Render(fmt.Sprintf("Checklist '%s' created & set default!", name))
				}
				return m, nil
			}

			if m.checklistForm.InName {
				m.checklistForm.NameInput, cmd = m.checklistForm.NameInput.Update(msg)
				return m, cmd
			}

		// -------------------------------------------------------------
		// FORM: PLAN (CREATE / EDIT)
		// -------------------------------------------------------------
		case ViewPlanForm:
			switch msg.Type {
			case tea.KeyEsc:
				m.state = ViewPlansTab
				return m, nil

			case tea.KeyTab, tea.KeyShiftTab:
				m.planForm.InName = !m.planForm.InName
				if m.planForm.InName {
					m.planForm.NameInput.Focus()
					m.planForm.URLsArea.Blur()
				} else {
					m.planForm.NameInput.Blur()
					m.planForm.URLsArea.Focus()
				}
				return m, nil

			case tea.KeyCtrlS:
				name := strings.TrimSpace(m.planForm.NameInput.Value())
				if name == "" {
					return m, nil
				}
				var cleanURLs []string
				for _, line := range strings.Split(m.planForm.URLsArea.Value(), "\n") {
					u := strings.TrimSpace(line)
					if u != "" {
						if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
							u = "https://" + u
						}
						cleanURLs = append(cleanURLs, u)
					}
				}

				plan := &data.Plan{
					ID:   m.planForm.ID,
					Name: name,
					URLs: cleanURLs,
				}
				_ = m.database.SavePlan(plan)
				m.plans, _ = m.database.ListPlans()
				m.plansTable = BuildPlansTable(m.plans)
				m.state = ViewPlansTab

				if m.planForm.ID != "" {
					m.statusMsg = SuccessStyle.Render(fmt.Sprintf("Plan '%s' updated with %d URLs!", name, len(cleanURLs)))
				} else {
					m.statusMsg = SuccessStyle.Render(fmt.Sprintf("Plan '%s' created with %d URLs!", name, len(cleanURLs)))
				}
				return m, nil
			}

			if m.planForm.InName {
				m.planForm.NameInput, cmd = m.planForm.NameInput.Update(msg)
			} else {
				m.planForm.URLsArea, cmd = m.planForm.URLsArea.Update(msg)
			}
			return m, cmd

		// -------------------------------------------------------------
		// PROMPT URL
		// -------------------------------------------------------------
		case ViewPromptURL:
			switch msg.Type {
			case tea.KeyEsc:
				m.state = ViewInspectionsTab
				return m, nil
			case tea.KeyEnter:
				targetURL := strings.TrimSpace(m.textInput.Value())
				if targetURL == "" {
					return m, nil
				}
				if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
					targetURL = "https://" + targetURL
				}
				return m.startInspectionRun(targetURL, []string{targetURL})
			}
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd

		// -------------------------------------------------------------
		// ISSUES VIEW
		// -------------------------------------------------------------
		case ViewIssues:
			switch msg.String() {
			case "q":
				return m, tea.Quit
			case "right", "enter", "l":
				if len(m.currentIssues) > 0 {
					idx := m.issuesTable.Cursor()
					if idx < len(m.currentIssues) {
						m.selectedIssue = &m.currentIssues[idx]
						m.state = ViewIssueDetail
					}
				}
				return m, nil
			case "left", "esc", "h":
				m.state = ViewInspectionsTab
				return m, nil
			}

		case ViewIssueDetail:
			switch msg.String() {
			case "q":
				return m, tea.Quit
			case "left", "esc", "h":
				m.state = ViewIssues
				return m, nil
			}
		}

	// -------------------------------------------------------------
	// PROGRESS STREAMING & RESULTS
	// -------------------------------------------------------------
	case ProgressMsg:
		m.currentInspectURL = msg.CurrentURL
		m.completedCount = msg.Completed
		m.totalCount = msg.Total

		var progressCmd tea.Cmd
		if msg.Total > 0 {
			percent := float64(msg.Completed) / float64(msg.Total)
			progressCmd = m.progressBar.SetPercent(percent)
		}

		return m, tea.Batch(
			progressCmd,
			WaitForProgress(m.progressChan),
		)

	case progress.FrameMsg:
		progressModel, cmd := m.progressBar.Update(msg)
		m.progressBar = progressModel.(progress.Model)
		return m, cmd

	case InspectionResultMsg:
		if msg.Err != nil {
			m.statusMsg = ErrorStyle.Render(fmt.Sprintf("❌ Inspection failed: %v", msg.Err))
			m.state = ViewInspectionsTab
			return m, nil
		}

		_ = m.database.SaveInspection(msg.Inspection)
		m.inspections, _ = m.database.ListInspections()
		m.inspectionsTable = BuildInspectionsTable(m.inspections)
		m.inspectionsTable.SetCursor(0)
		m.state = ViewInspectionsTab
		m.statusMsg = SuccessStyle.Render(fmt.Sprintf("✅ Finished inspection for '%s' (%d endpoints in %v)", msg.Inspection.PlanName, msg.Inspection.TotalEndpoints, msg.Inspection.Duration))
		return m, nil
	}

	switch m.state {
	case ViewInspectionsTab:
		m.inspectionsTable, cmd = m.inspectionsTable.Update(msg)
	case ViewChecklistsTab:
		m.checklistsTable, cmd = m.checklistsTable.Update(msg)
	case ViewPlansTab:
		m.plansTable, cmd = m.plansTable.Update(msg)
	case ViewPromptURL:
		m.textInput, cmd = m.textInput.Update(msg)
	case ViewInspecting:
		m.spinner, cmd = m.spinner.Update(msg)
	case ViewIssues:
		m.issuesTable, cmd = m.issuesTable.Update(msg)
	}

	return m, cmd
}

func (m *Model) startInspectionRun(planName string, urls []string) (tea.Model, tea.Cmd) {
	m.inspectTitle = planName
	m.currentInspectURL = "Resolving targets..."
	m.completedCount = 0
	m.totalCount = len(urls)
	m.state = ViewInspecting
	m.statusMsg = ""

	_ = m.progressBar.SetPercent(0)
	m.progressChan = make(chan tea.Msg, 50)

	runner := m.buildActiveRunner()
	chkName := "Default Checklist"
	if m.defaultChecklist != nil {
		chkName = m.defaultChecklist.Name
	}

	return m, tea.Batch(
		m.spinner.Tick,
		RunPlanWithProgress(m.progressChan, runner, planName, urls, chkName),
		WaitForProgress(m.progressChan),
	)
}

func (m Model) buildActiveRunner() *engine.Runner {
	if m.defaultChecklist == nil || len(m.defaultChecklist.CheckNames) == 0 {
		return engine.NewRunner(m.fetcher, m.cfg, m.allChecks...)
	}

	var active []data.Check
	for _, c := range m.allChecks {
		if m.defaultChecklist.HasCheck(c.Name()) {
			active = append(active, c)
		}
	}
	return engine.NewRunner(m.fetcher, m.cfg, active...)
}

func (m Model) renderTabs() string {
	tab1 := InactiveTabStyle.Render("1: Inspections")
	tab2 := InactiveTabStyle.Render("2: Checklists")
	tab3 := InactiveTabStyle.Render("3: Plans")

	switch m.state {
	case ViewInspectionsTab:
		tab1 = ActiveTabStyle.Render("1: Inspections")
	case ViewChecklistsTab:
		tab2 = ActiveTabStyle.Render("2: Checklists")
	case ViewPlansTab:
		tab3 = ActiveTabStyle.Render("3: Plans")
	}

	chkNotice := ""
	if m.defaultChecklist != nil {
		chkNotice = HelpStyle.Render(fmt.Sprintf(" [Active Checklist: %s (%d checks)]", m.defaultChecklist.Name, len(m.defaultChecklist.CheckNames)))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, tab1, tab2, tab3, chkNotice)
}

func (m Model) View() string {
	switch m.state {
	case ViewInspectionsTab:
		return m.renderTabs() + "\n\n" + m.renderInspectionsTab()
	case ViewChecklistsTab:
		return m.renderTabs() + "\n\n" + m.renderChecklistsTab()
	case ViewPlansTab:
		return m.renderTabs() + "\n\n" + m.renderPlansTab()
	case ViewConfirmDelete:
		return m.renderConfirmDeleteView()
	case ViewChecklistForm:
		return m.renderChecklistFormView()
	case ViewPlanForm:
		return m.renderPlanFormView()
	case ViewPromptURL:
		return m.renderPromptView()
	case ViewInspecting:
		return m.renderInspectingView()
	case ViewIssues:
		return m.renderIssuesView()
	case ViewIssueDetail:
		return m.renderIssueDetailView()
	default:
		return ""
	}
}

func (m Model) renderConfirmDeleteView() string {
	var body strings.Builder

	boxStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#E03131")).
		Padding(1, 2)

	var card strings.Builder
	card.WriteString(ErrorStyle.Render("⚠️  Confirm Deletion") + "\n\n")
	card.WriteString(fmt.Sprintf("Are you sure you want to delete %s '%s'?\n", m.pendingDel.itemType, m.pendingDel.name))
	card.WriteString("This action cannot be undone.\n\n")
	card.WriteString(HelpStyle.Render("[y] Yes, Delete  •  [n / Esc / ←] Cancel"))

	body.WriteString(boxStyle.Render(card.String()))
	return body.String()
}

func (m Model) renderInspectingView() string {
	var body strings.Builder

	header := fmt.Sprintf("⏳ Inspecting Plan: %s", m.inspectTitle)
	body.WriteString(TitleStyle.Render(header))
	body.WriteString("\n\n")

	body.WriteString(fmt.Sprintf("%s Current Target: %s\n\n", m.spinner.View(), m.currentInspectURL))
	body.WriteString(m.progressBar.View())
	body.WriteString("\n\n")

	if m.totalCount > 0 {
		body.WriteString(HelpStyle.Render(fmt.Sprintf("Completed %d of %d endpoints", m.completedCount, m.totalCount)))
	} else {
		body.WriteString(HelpStyle.Render("Resolving targets..."))
	}

	return body.String()
}
