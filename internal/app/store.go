package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cockpit/internal/config"
)

type Store struct {
	cfg config.Config
}

type userState struct {
	Archived       map[string]bool   `json:"archived"`
	Ignored        map[string]bool   `json:"ignored"`
	Statuses       map[string]Status `json:"statuses,omitempty"`
	ManualBindings map[string]int    `json:"manual_bindings,omitempty"`
	MissionNames   map[string]string `json:"mission_names,omitempty"`
}

type SummaryCache struct {
	TaskID      string    `json:"task_id"`
	SessionID   string    `json:"session_id"`
	Summary     string    `json:"summary"`
	GeneratedAt time.Time `json:"generated_at"`
	InputHash   string    `json:"input_hash"`
	Raw         string    `json:"raw,omitempty"`
}

func NewStore(cfg config.Config) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(cfg.StateDir, "summaries"), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(cfg.StateDir, "debriefs"), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.InternalRunDir, 0o755); err != nil {
		return nil, err
	}
	return &Store{cfg: cfg}, nil
}

func (s *Store) LoadUserState() userState {
	state := userState{
		Archived:       map[string]bool{},
		Ignored:        map[string]bool{},
		Statuses:       map[string]Status{},
		ManualBindings: map[string]int{},
		MissionNames:   map[string]string{},
	}
	data, err := os.ReadFile(s.userStatePath())
	if err != nil {
		return state
	}
	_ = json.Unmarshal(data, &state)
	if state.Archived == nil {
		state.Archived = map[string]bool{}
	}
	if state.Ignored == nil {
		state.Ignored = map[string]bool{}
	}
	if state.Statuses == nil {
		state.Statuses = map[string]Status{}
	}
	if state.ManualBindings == nil {
		state.ManualBindings = map[string]int{}
	}
	if state.MissionNames == nil {
		state.MissionNames = map[string]string{}
	}
	return state
}

func (s *Store) SetArchived(taskID string, value bool) error {
	state := s.LoadUserState()
	state.Archived[taskID] = value
	return s.saveUserState(state)
}

func (s *Store) SetIgnored(taskID string, value bool) error {
	state := s.LoadUserState()
	state.Ignored[taskID] = value
	return s.saveUserState(state)
}

func (s *Store) SaveStatus(taskID string, status Status) error {
	state := s.LoadUserState()
	state.Statuses[taskID] = status
	return s.saveUserState(state)
}

func (s *Store) PreviousStatuses() map[string]Status {
	return s.LoadUserState().Statuses
}

func (s *Store) SetManualBinding(taskID string, paneID int) error {
	state := s.LoadUserState()
	state.ManualBindings[taskID] = paneID
	return s.saveUserState(state)
}

func (s *Store) ClearManualBinding(taskID string) error {
	state := s.LoadUserState()
	delete(state.ManualBindings, taskID)
	return s.saveUserState(state)
}

func (s *Store) SetMissionName(missionID string, name string) error {
	state := s.LoadUserState()
	missionID = strings.TrimSpace(missionID)
	name = strings.TrimSpace(name)
	if missionID == "" {
		return nil
	}
	if name == "" {
		delete(state.MissionNames, missionID)
	} else {
		state.MissionNames[missionID] = name
	}
	return s.saveUserState(state)
}

func (s *Store) saveUserState(state userState) error {
	if err := os.MkdirAll(s.cfg.StateDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.userStatePath(), data, 0o644)
}

func (s *Store) userStatePath() string {
	return filepath.Join(s.cfg.StateDir, "tasks.json")
}

func (s *Store) LoadSummary(taskID string) (SummaryCache, bool) {
	var cache SummaryCache
	data, err := os.ReadFile(s.summaryPath(taskID))
	if err != nil {
		return cache, false
	}
	if err := json.Unmarshal(data, &cache); err != nil {
		return cache, false
	}
	return cache, true
}

func (s *Store) SaveSummary(cache SummaryCache) error {
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.summaryPath(cache.TaskID), data, 0o644)
}

func (s *Store) LoadDebrief(taskID string) (Debrief, bool) {
	var debrief Debrief
	data, err := os.ReadFile(s.debriefPath(taskID))
	if err != nil {
		return debrief, false
	}
	if err := json.Unmarshal(data, &debrief); err != nil {
		return debrief, false
	}
	return debrief, true
}

func (s *Store) SaveDebrief(debrief Debrief) error {
	if err := os.MkdirAll(filepath.Join(s.cfg.StateDir, "debriefs"), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(debrief, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.debriefPath(debrief.TaskID), data, 0o644)
}

func (s *Store) LoadDoneInbox() []DoneInboxItem {
	var items []DoneInboxItem
	data, err := os.ReadFile(s.doneInboxPath())
	if err != nil {
		return nil
	}
	_ = json.Unmarshal(data, &items)
	return items
}

func (s *Store) SyncDoneInbox(tasks []Task) ([]DoneInboxItem, error) {
	items := s.LoadDoneInbox()
	byTask := map[string]int{}
	for index, item := range items {
		byTask[item.TaskID] = index
	}
	changed := false
	for _, task := range tasks {
		if task.Session.Internal || task.Status != StatusCompleted {
			continue
		}
		item := doneItemFromTask(task)
		if index, ok := byTask[task.ID]; ok {
			item.Reviewed = items[index].Reviewed
			item.Archived = items[index].Archived
			items[index] = item
			changed = true
			continue
		}
		items = append(items, item)
		byTask[task.ID] = len(items) - 1
		changed = true
	}
	if changed {
		if err := s.saveDoneInbox(items); err != nil {
			return items, err
		}
	}
	return visibleDoneInbox(items), nil
}

func (s *Store) SetDoneReviewed(itemID string, value bool) error {
	items := s.LoadDoneInbox()
	for index := range items {
		if items[index].ID == itemID {
			items[index].Reviewed = value
			return s.saveDoneInbox(items)
		}
	}
	return nil
}

func (s *Store) ArchiveDoneItem(itemID string) error {
	items := s.LoadDoneInbox()
	for index := range items {
		if items[index].ID == itemID {
			items[index].Archived = true
			return s.saveDoneInbox(items)
		}
	}
	return nil
}

func (s *Store) saveDoneInbox(items []DoneInboxItem) error {
	if err := os.MkdirAll(s.cfg.StateDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.doneInboxPath(), data, 0o644)
}

func (s *Store) summaryPath(taskID string) string {
	safe := strings.NewReplacer("/", "_", ":", "_").Replace(taskID)
	return filepath.Join(s.cfg.StateDir, "summaries", safe+".json")
}

func (s *Store) debriefPath(taskID string) string {
	safe := strings.NewReplacer("/", "_", ":", "_").Replace(taskID)
	return filepath.Join(s.cfg.StateDir, "debriefs", safe+".json")
}

func (s *Store) doneInboxPath() string {
	return filepath.Join(s.cfg.StateDir, "done.json")
}

func (s *Store) Config() config.Config {
	return s.cfg
}

func doneItemFromTask(task Task) DoneInboxItem {
	completedAt := task.Session.LastEventAt
	if completedAt.IsZero() {
		completedAt = time.Now()
	}
	item := DoneInboxItem{
		ID:          strings.NewReplacer("/", "_", ":", "_").Replace(task.ID),
		TaskID:      task.ID,
		SessionID:   task.Session.ID,
		Agent:       task.Session.Agent,
		Project:     task.RepoName(),
		CWD:         task.Session.CWD,
		Title:       task.Session.Title,
		Status:      task.Status,
		Summary:     task.Summary,
		CompletedAt: completedAt,
		Diff:        task.Diff,
		Debrief:     task.Debrief,
	}
	if task.Binding.Pane != nil {
		paneID := task.Binding.Pane.PaneID
		item.PaneID = &paneID
	}
	return item
}

func visibleDoneInbox(items []DoneInboxItem) []DoneInboxItem {
	visible := make([]DoneInboxItem, 0, len(items))
	for _, item := range items {
		if !item.Archived {
			visible = append(visible, item)
		}
	}
	return visible
}

func (s *Store) DoneInbox() []DoneInboxItem {
	return visibleDoneInbox(s.LoadDoneInbox())
}
