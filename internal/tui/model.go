package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"cockpit/internal/app"
	"cockpit/internal/config"
	"cockpit/internal/engine"
	"cockpit/internal/notify"
	"cockpit/internal/summary"
	"cockpit/internal/wezterm"
)

type Model struct {
	ctx        context.Context
	cfg        config.Config
	store      *app.Store
	summarizer *summary.CodexSummarizer

	tasks       []app.Task
	cursor      int
	width       int
	height      int
	errs        []string
	hidden      int
	statusLine  string
	ready       bool
	detail      bool
	summarizing map[string]bool
	seenStatus  map[string]app.Status
	demo        bool
}

type snapshotMsg struct {
	snapshot app.Snapshot
	err      error
}

type summaryMsg struct {
	taskID  string
	summary string
	err     error
}

type tickMsg struct{}

var (
	titleStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	subtitleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	mutedStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errorStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	workingStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	waitStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	idleStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	linkStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	attention        = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	panelStyle       = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("8")).Padding(0, 1)
	headerPanelStyle = lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(lipgloss.Color("4")).Padding(0, 1)
	metricStyle      = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("8")).Padding(0, 1)
	rowStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	selectedRowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
	tableHeadStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true)
)

type taskStats struct {
	total     int
	bound     int
	unbound   int
	attention int
	waiting   int
	working   int
	idle      int
	completed int
}

func NewModel(ctx context.Context, cfg config.Config, store *app.Store) Model {
	return Model{
		ctx:         ctx,
		cfg:         cfg,
		store:       store,
		summarizer:  summary.NewCodexSummarizer(cfg, store),
		summarizing: map[string]bool{},
		seenStatus:  map[string]app.Status{},
		statusLine:  "loading sessions...",
	}
}

func NewDemoModel() Model {
	return Model{
		ctx:         context.Background(),
		tasks:       demoTasks(),
		cursor:      0,
		hidden:      9,
		statusLine:  "demo data · terminal cockpit layout",
		ready:       true,
		detail:      true,
		summarizing: map[string]bool{},
		seenStatus:  map[string]app.Status{},
		demo:        true,
	}
}

func (m Model) Init() tea.Cmd {
	if m.demo {
		return nil
	}
	return tea.Batch(loadSnapshot(m.ctx, m.cfg, m.store), tick())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tickMsg:
		if m.demo {
			return m, nil
		}
		return m, tea.Batch(loadSnapshot(m.ctx, m.cfg, m.store), tick())
	case snapshotMsg:
		if msg.err != nil {
			m.statusLine = msg.err.Error()
		}
		m.errs = msg.snapshot.Errors
		m.tasks, m.hidden = visibleTasks(msg.snapshot.Tasks)
		if m.cursor >= len(m.tasks) {
			m.cursor = len(m.tasks) - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.ready = true
		m.statusLine = fmt.Sprintf("%d tasks · %d hidden · refreshed %s", len(m.tasks), m.hidden, time.Now().Format("15:04:05"))
		var cmds []tea.Cmd
		for _, task := range m.tasks {
			previous, seen := m.seenStatus[task.ID]
			if seen {
				notify.MaybeNotify(m.ctx, task, previous)
			}
			m.seenStatus[task.ID] = task.Status
		}
		if cmd := m.nextSummaryCmd(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	case summaryMsg:
		delete(m.summarizing, msg.taskID)
		for i := range m.tasks {
			if m.tasks[i].ID == msg.taskID {
				if msg.err != nil {
					m.tasks[i].Summary = "summary error: " + msg.err.Error()
				} else {
					m.tasks[i].Summary = msg.summary
					m.tasks[i].SummaryAt = time.Now()
				}
			}
		}
		if msg.err != nil {
			m.statusLine = msg.err.Error()
		} else {
			m.statusLine = "summary refreshed"
		}
		return m, m.nextSummaryCmd()
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "down", "j":
			if m.cursor < len(m.tasks)-1 {
				m.cursor++
			}
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "enter":
			if m.demo {
				m.statusLine = "demo: Enter would activate the bound WezTerm pane"
				return m, nil
			}
			return m, m.openSelected()
		case "s":
			if m.demo {
				m.statusLine = "demo: s would refresh the selected summary"
				return m, nil
			}
			return m, m.forceSummarySelected()
		case "a":
			if m.demo {
				m.statusLine = "demo: a would archive the selected task"
				return m, nil
			}
			return m, m.archiveSelected()
		case "i":
			if m.demo {
				m.statusLine = "demo: i would ignore the selected task"
				return m, nil
			}
			return m, m.ignoreSelected()
		case "l", " ":
			m.detail = !m.detail
		}
	}
	return m, nil
}

func (m Model) View() string {
	if !m.ready {
		return m.renderLoading()
	}
	var b strings.Builder
	stats := collectStats(m.tasks)
	b.WriteString(m.renderHeader(stats))
	b.WriteString("\n")
	b.WriteString(m.renderMetrics(stats))
	b.WriteString("\n\n")
	if len(m.tasks) == 0 {
		b.WriteString(panelStyle.Width(m.panelWidth()).Render(mutedStyle.Render("No active Claude/Codex sessions found.")))
		b.WriteString("\n")
		b.WriteString(m.renderFooter())
		return b.String()
	}
	if m.detail && m.cursor >= 0 && m.cursor < len(m.tasks) && m.contentWidth() >= 118 {
		b.WriteString(m.renderSplit())
	} else {
		b.WriteString(m.renderBoard())
	}
	if m.detail && m.cursor >= 0 && m.cursor < len(m.tasks) && m.contentWidth() < 118 {
		b.WriteString("\n")
		b.WriteString(m.renderDetailPanel(m.tasks[m.cursor]))
	}
	b.WriteString("\n")
	if len(m.errs) > 0 {
		b.WriteString(m.renderWarnings())
		b.WriteString("\n")
	}
	b.WriteString(m.renderFooter())
	return b.String()
}

func (m Model) renderLoading() string {
	width := m.panelWidth()
	body := strings.Join([]string{
		titleStyle.Render("COCKPIT"),
		mutedStyle.Render("scanning agent sessions, panes, and logs"),
		"",
		mutedStyle.Render(m.statusLine),
	}, "\n")
	return headerPanelStyle.Width(width).Render(body)
}

func (m Model) renderHeader(stats taskStats) string {
	width := m.panelWidth()
	left := titleStyle.Render("COCKPIT") + " " + subtitleStyle.Render("agent flight deck")
	right := mutedStyle.Render(time.Now().Format("15:04:05")) + "  " + mutedStyle.Render(fmt.Sprintf("%d visible / %d hidden", stats.total, m.hidden))
	gap := width - lipgloss.Width(left) - lipgloss.Width(right) - 4
	if gap < 1 {
		gap = 1
	}
	line := truncate(left+strings.Repeat(" ", gap)+right, width)
	return headerPanelStyle.Width(width).Render(line)
}

func (m Model) renderMetrics(stats taskStats) string {
	width := m.contentWidth()
	cells := []string{
		metricCell("ATTN", stats.attention, app.StatusNeedsAttention),
		metricCell("WAIT", stats.waiting, app.StatusWaiting),
		metricCell("WORK", stats.working, app.StatusWorking),
		metricCell("IDLE", stats.idle, app.StatusIdle),
		metricCell("BOUND", stats.bound, app.StatusCompleted),
		metricCell("UNBOUND", stats.unbound, app.StatusUnbound),
	}
	if width < 100 {
		return mutedStyle.Render(fmt.Sprintf("ATTN %d | WAIT %d | WORK %d | IDLE %d | BOUND %d | UNBOUND %d", stats.attention, stats.waiting, stats.working, stats.idle, stats.bound, stats.unbound))
	}
	joined := lipgloss.JoinHorizontal(lipgloss.Top, cells...)
	if lipgloss.Width(joined) > width {
		return strings.Join([]string{
			lipgloss.JoinHorizontal(lipgloss.Top, cells[:3]...),
			lipgloss.JoinHorizontal(lipgloss.Top, cells[3:]...),
		}, "\n")
	}
	return joined
}

func (m Model) renderBoard() string {
	return m.renderBoardFor(m.contentWidth())
}

func (m Model) renderSplit() string {
	width := m.contentWidth()
	leftWidth := int(float64(width) * 0.58)
	rightWidth := width - leftWidth - 4
	if rightWidth < 42 {
		return m.renderBoard()
	}
	left := m.renderBoardFor(leftWidth)
	right := m.renderDetailPanelFor(m.tasks[m.cursor], rightWidth)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
}

func (m Model) renderBoardFor(width int) string {
	var b strings.Builder
	header := m.renderTableHeader()
	b.WriteString(panelStyle.Width(max(36, width-4)).Render(header))
	b.WriteString("\n")
	currentGroup := app.Status("")
	for i, task := range m.tasks {
		if task.Status != currentGroup {
			currentGroup = task.Status
			b.WriteString(groupHeader(task.Status, countStatus(m.tasks, task.Status), width))
			b.WriteString("\n")
		}
		b.WriteString(m.renderTaskFor(i, task, width))
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) renderTableHeader() string {
	return tableHeadStyle.Render(fmt.Sprintf("  %-6s %-7s %-22s %-10s %-12s %s", "STATE", "AGENT", "PROJECT", "ACTIVITY", "LINK", "SUMMARY"))
}

func (m Model) renderTask(i int, task app.Task) string {
	return m.renderTaskFor(i, task, m.contentWidth())
}

func (m Model) renderTaskFor(i int, task app.Task, width int) string {
	state := padCell(statusTag(task.Status), 6)
	agent := strings.ToUpper(string(task.Session.Agent))
	repoWidth := 18
	linkWidth := 10
	if width > 112 {
		repoWidth = 22
		linkWidth = 12
	}
	repo := padCell(truncate(task.RepoName(), repoWidth), repoWidth)
	age := "?"
	if !task.Session.LastEventAt.IsZero() {
		age = shortDuration(time.Since(task.Session.LastEventAt)) + " ago"
	}
	age = padCell(age, 10)
	link := padCell(bindingLabel(task), linkWidth)
	summaryText := task.Summary
	if m.summarizing[task.ID] {
		summaryText = "summarizing..."
	}
	if summaryText == "" {
		summaryText = "summary pending"
	}
	prefix := fmt.Sprintf("%s %s %s %s %s %s ", selector(i == m.cursor), state, padCell(agent, 7), repo, age, link)
	summaryWidth := max(24, width-lipgloss.Width(prefix)-2)
	line := prefix + truncate(summaryText, summaryWidth)
	if i == m.cursor {
		return selectedRowStyle.Render(line)
	}
	return rowStyle.Render(line)
}

func (m Model) renderDetailPanel(task app.Task) string {
	return m.renderDetailPanelFor(task, m.panelWidth())
}

func (m Model) renderDetailPanelFor(task app.Task, width int) string {
	var b strings.Builder
	inner := max(36, width-4)
	valueWidth := max(10, inner-14)
	b.WriteString(titleStyle.Render("SELECTED TASK"))
	b.WriteString("  ")
	b.WriteString(statusTag(task.Status))
	b.WriteString("\n")
	b.WriteString(detailKV("agent", string(task.Session.Agent), valueWidth))
	b.WriteString(detailKV("project", task.RepoName(), valueWidth))
	b.WriteString(detailKV("session", task.IDShort(), valueWidth))
	b.WriteString(detailKV("link", bindingLabel(task), valueWidth))
	if task.Session.CWD != "" {
		b.WriteString(detailKV("cwd", task.Session.CWD, valueWidth))
	}
	if task.AttentionReason != "" {
		b.WriteString(detailKV("alert", task.AttentionReason, valueWidth))
	}
	if len(task.Binding.Reasons) > 0 {
		b.WriteString(detailKV("bind", strings.Join(task.Binding.Reasons, ", "), valueWidth))
	}
	b.WriteString("\n")
	b.WriteString(tableHeadStyle.Render("RECENT EVENTS"))
	b.WriteString("\n")
	start := 0
	if len(task.Session.Events) > 8 {
		start = len(task.Session.Events) - 8
	}
	for _, event := range task.Session.Events[start:] {
		text := event.Text
		if event.Detail != "" {
			text += " | " + event.Detail
		}
		textWidth := max(10, inner-22)
		b.WriteString(eventLine(event.At, event.Type, text, textWidth))
	}
	return panelStyle.Width(inner).Render(b.String())
}

func (m Model) renderWarnings() string {
	width := m.panelWidth()
	return panelStyle.Width(width).BorderForeground(lipgloss.Color("9")).Render(errorStyle.Render(strings.Join(m.errs, " · ")))
}

func (m Model) renderFooter() string {
	keys := "Enter open | s summary | l detail | a archive | i ignore | q quit"
	return mutedStyle.Render(truncate(m.statusLine+"  |  "+keys, m.contentWidth()))
}

func (m *Model) nextSummaryCmd() tea.Cmd {
	for i := range m.tasks {
		task := m.tasks[i]
		if task.Session.Internal || task.Archived || task.Ignored {
			continue
		}
		if task.Binding.Pane == nil && task.Status != app.StatusNeedsAttention && task.Status != app.StatusWaiting {
			continue
		}
		if m.summarizing[task.ID] {
			continue
		}
		if !m.summarizer.NeedsRefresh(task) {
			continue
		}
		m.summarizing[task.ID] = true
		return summarizeCmd(m.ctx, m.summarizer, task)
	}
	return nil
}

func (m Model) openSelected() tea.Cmd {
	if len(m.tasks) == 0 || m.cursor >= len(m.tasks) {
		return nil
	}
	task := m.tasks[m.cursor]
	if task.Binding.Pane == nil {
		m.statusLine = "selected task is unbound"
		return nil
	}
	paneID := strconv.Itoa(task.Binding.Pane.PaneID)
	return func() tea.Msg {
		err := wezterm.ActivatePane(m.ctx, m.cfg.WeztermBin, paneID)
		if err != nil {
			return summaryMsg{taskID: task.ID, err: err}
		}
		return summaryMsg{taskID: task.ID, summary: task.Summary}
	}
}

func (m Model) forceSummarySelected() tea.Cmd {
	if len(m.tasks) == 0 || m.cursor >= len(m.tasks) {
		return nil
	}
	task := m.tasks[m.cursor]
	return summarizeCmd(m.ctx, m.summarizer, task)
}

func (m Model) archiveSelected() tea.Cmd {
	if len(m.tasks) == 0 || m.cursor >= len(m.tasks) {
		return nil
	}
	task := m.tasks[m.cursor]
	return func() tea.Msg {
		if err := m.store.SetArchived(task.ID, true); err != nil {
			return snapshotMsg{err: err}
		}
		snap, err := engine.BuildSnapshot(m.ctx, m.cfg, m.store)
		return snapshotMsg{snapshot: snap, err: err}
	}
}

func (m Model) ignoreSelected() tea.Cmd {
	if len(m.tasks) == 0 || m.cursor >= len(m.tasks) {
		return nil
	}
	task := m.tasks[m.cursor]
	return func() tea.Msg {
		if err := m.store.SetIgnored(task.ID, true); err != nil {
			return snapshotMsg{err: err}
		}
		snap, err := engine.BuildSnapshot(m.ctx, m.cfg, m.store)
		return snapshotMsg{snapshot: snap, err: err}
	}
}

func loadSnapshot(ctx context.Context, cfg config.Config, store *app.Store) tea.Cmd {
	return func() tea.Msg {
		snap, err := engine.BuildSnapshot(ctx, cfg, store)
		return snapshotMsg{snapshot: snap, err: err}
	}
}

func summarizeCmd(ctx context.Context, summarizer *summary.CodexSummarizer, task app.Task) tea.Cmd {
	return func() tea.Msg {
		runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		err := summarizer.Refresh(runCtx, &task)
		return summaryMsg{taskID: task.ID, summary: task.Summary, err: err}
	}
}

func tick() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return tickMsg{} })
}

func visibleTasks(tasks []app.Task) ([]app.Task, int) {
	out := tasks[:0]
	hidden := 0
	for _, task := range tasks {
		if task.Session.Internal || task.Archived || task.Ignored {
			hidden++
			continue
		}
		if shouldHideInactiveUnbound(task) {
			hidden++
			continue
		}
		out = append(out, task)
	}
	return out, hidden
}

func collectStats(tasks []app.Task) taskStats {
	stats := taskStats{total: len(tasks)}
	for _, task := range tasks {
		if task.Binding.Pane != nil {
			stats.bound++
		} else {
			stats.unbound++
		}
		switch task.Status {
		case app.StatusNeedsAttention:
			stats.attention++
		case app.StatusWaiting:
			stats.waiting++
		case app.StatusWorking:
			stats.working++
		case app.StatusIdle:
			stats.idle++
		case app.StatusCompleted:
			stats.completed++
		}
	}
	return stats
}

func countStatus(tasks []app.Task, status app.Status) int {
	count := 0
	for _, task := range tasks {
		if task.Status == status {
			count++
		}
	}
	return count
}

func metricCell(label string, value int, status app.Status) string {
	valueText := fmt.Sprintf("%02d", value)
	body := groupStyle(status).Bold(true).Render(label) + "\n" + titleStyle.Render(valueText)
	return metricStyle.Width(13).Render(body)
}

func detailKV(key, value string, width int) string {
	return fmt.Sprintf("%-8s %s\n", key, truncate(value, width))
}

func eventLine(at time.Time, eventType app.EventType, text string, width int) string {
	return fmt.Sprintf("%s  %-6s %s\n", mutedStyle.Render(at.Format("15:04")), mutedStyle.Render(eventTypeLabel(eventType)), truncate(text, width))
}

func eventTypeLabel(eventType app.EventType) string {
	switch eventType {
	case app.EventAssistant:
		return "asst"
	case app.EventUser:
		return "user"
	case app.EventTool:
		return "tool"
	case app.EventResult:
		return "result"
	case app.EventSystem:
		return "sys"
	case app.EventError:
		return "error"
	default:
		return string(eventType)
	}
}

func groupHeader(status app.Status, count int, width int) string {
	label := fmt.Sprintf(" %s / %d ", statusLabel(status), count)
	lineWidth := width - lipgloss.Width(label)
	if lineWidth < 0 {
		lineWidth = 0
	}
	return groupStyle(status).Bold(true).Render(label + strings.Repeat("─", lineWidth))
}

func selector(active bool) string {
	if active {
		return ">"
	}
	return " "
}

func bindingLabel(task app.Task) string {
	if task.Binding.Pane == nil {
		return "unbound"
	}
	if hasReason(task.Binding.Reasons, "manual attach") {
		return fmt.Sprintf("manual:%d", task.Binding.Pane.PaneID)
	}
	return fmt.Sprintf("pane:%d", task.Binding.Pane.PaneID)
}

func statusTag(status app.Status) string {
	label := "UNK"
	switch status {
	case app.StatusNeedsAttention:
		label = "ATTN"
	case app.StatusWaiting:
		label = "WAIT"
	case app.StatusWorking:
		label = "WORK"
	case app.StatusIdle:
		label = "IDLE"
	case app.StatusCompleted:
		label = "DONE"
	case app.StatusUnbound:
		label = "LINK"
	}
	return groupStyle(status).Bold(true).Render(label)
}

func hasReason(reasons []string, reason string) bool {
	for _, candidate := range reasons {
		if candidate == reason {
			return true
		}
	}
	return false
}

func shouldHideInactiveUnbound(task app.Task) bool {
	if task.Status != app.StatusUnbound || task.Binding.Pane != nil {
		return false
	}
	if task.Session.LastEventAt.IsZero() {
		return true
	}
	return time.Since(task.Session.LastEventAt) > 2*time.Hour
}

func statusLabel(status app.Status) string {
	switch status {
	case app.StatusNeedsAttention:
		return "NEEDS ATTENTION"
	case app.StatusWaiting:
		return "WAITING"
	case app.StatusWorking:
		return "WORKING"
	case app.StatusIdle:
		return "IDLE"
	case app.StatusCompleted:
		return "COMPLETED"
	case app.StatusUnbound:
		return "UNBOUND"
	default:
		return strings.ToUpper(string(status))
	}
}

func groupStyle(status app.Status) lipgloss.Style {
	switch status {
	case app.StatusNeedsAttention:
		return attention
	case app.StatusWaiting:
		return waitStyle
	case app.StatusWorking:
		return workingStyle
	case app.StatusIdle:
		return idleStyle
	case app.StatusCompleted:
		return linkStyle
	case app.StatusUnbound:
		return linkStyle
	default:
		return mutedStyle
	}
}

func (m Model) contentWidth() int {
	if m.width <= 0 {
		return 110
	}
	if m.width < 72 {
		return 72
	}
	return m.width - 2
}

func (m Model) panelWidth() int {
	return max(40, m.contentWidth()-4)
}

func padCell(text string, width int) string {
	current := lipgloss.Width(text)
	if current >= width {
		return truncate(text, width)
	}
	return text + strings.Repeat(" ", width-current)
}

func shortDuration(d time.Duration) string {
	if d < time.Minute {
		return d.Round(time.Second).String()
	}
	if d < time.Hour {
		return d.Round(time.Minute).String()
	}
	return d.Round(time.Hour).String()
}

func truncate(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(text) <= width {
		return text
	}
	runes := []rune(text)
	for len(runes) > 0 && lipgloss.Width(string(runes)) > width-1 {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
