// Package workspaces — workspace-scoped task list under <cwd>/.agents/tasks.json.
package workspaces

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

const (
	// TasksFile is the JSON filename under .agents/.
	TasksFile = "tasks.json"
	// tasksDir is the workspace metadata directory (same as projmap).
	tasksDir = ".agents"
)

// TaskStatus is the lifecycle of a workspace task.
type TaskStatus string

const (
	TaskTodo  TaskStatus = "todo"
	TaskDoing TaskStatus = "doing"
	TaskDone  TaskStatus = "done"
)

// Task is a lightweight workspace to-do item.
type Task struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Status    TaskStatus `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type tasksFile struct {
	Version int    `json:"version"`
	Tasks   []Task `json:"tasks"`
}

var tasksMu sync.Mutex

// TasksPath returns the absolute path of the tasks store for a workspace root.
func TasksPath(absCwd string) string {
	return filepath.Join(absCwd, tasksDir, TasksFile)
}

func ValidTaskStatus(s TaskStatus) bool {
	switch s {
	case TaskTodo, TaskDoing, TaskDone:
		return true
	default:
		return false
	}
}

func loadTasks(path string) (tasksFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return tasksFile{Version: 1, Tasks: []Task{}}, nil
		}
		return tasksFile{}, err
	}
	var f tasksFile
	if err := json.Unmarshal(b, &f); err != nil {
		return tasksFile{}, fmt.Errorf("tasks.json: %w", err)
	}
	if f.Version == 0 {
		f.Version = 1
	}
	if f.Tasks == nil {
		f.Tasks = []Task{}
	}
	return f, nil
}

func saveTasks(path string, f tasksFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f.Version = 1
	if f.Tasks == nil {
		f.Tasks = []Task{}
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
}

// ListTasks returns tasks for the workspace at absCwd (newest first).
func ListTasks(absCwd string) ([]Task, error) {
	tasksMu.Lock()
	defer tasksMu.Unlock()
	f, err := loadTasks(TasksPath(absCwd))
	if err != nil {
		return nil, err
	}
	// newest first
	out := make([]Task, len(f.Tasks))
	copy(out, f.Tasks)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// CreateTask appends a new todo with the given title.
func CreateTask(absCwd, title string) (*Task, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("title required")
	}
	tasksMu.Lock()
	defer tasksMu.Unlock()
	path := TasksPath(absCwd)
	f, err := loadTasks(path)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	t := Task{
		ID:        "task_" + ulid.Make().String(),
		Title:     title,
		Status:    TaskTodo,
		CreatedAt: now,
		UpdatedAt: now,
	}
	f.Tasks = append(f.Tasks, t)
	if err := saveTasks(path, f); err != nil {
		return nil, err
	}
	return &t, nil
}

// UpdateTask patches status and/or title for a task id.
func UpdateTask(absCwd, id string, status *TaskStatus, title *string) (*Task, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("id required")
	}
	if status != nil && !ValidTaskStatus(*status) {
		return nil, fmt.Errorf("status must be todo, doing, or done")
	}
	if title != nil {
		trimmed := strings.TrimSpace(*title)
		if trimmed == "" {
			return nil, fmt.Errorf("title required")
		}
		*title = trimmed
	}
	if status == nil && title == nil {
		return nil, fmt.Errorf("status or title required")
	}
	tasksMu.Lock()
	defer tasksMu.Unlock()
	path := TasksPath(absCwd)
	f, err := loadTasks(path)
	if err != nil {
		return nil, err
	}
	for i := range f.Tasks {
		if f.Tasks[i].ID != id {
			continue
		}
		if status != nil {
			f.Tasks[i].Status = *status
		}
		if title != nil {
			f.Tasks[i].Title = *title
		}
		f.Tasks[i].UpdatedAt = time.Now().UTC()
		if err := saveTasks(path, f); err != nil {
			return nil, err
		}
		t := f.Tasks[i]
		return &t, nil
	}
	return nil, fmt.Errorf("task not found")
}

// DeleteTask removes a task by id.
func DeleteTask(absCwd, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("id required")
	}
	tasksMu.Lock()
	defer tasksMu.Unlock()
	path := TasksPath(absCwd)
	f, err := loadTasks(path)
	if err != nil {
		return err
	}
	out := f.Tasks[:0]
	found := false
	for _, t := range f.Tasks {
		if t.ID == id {
			found = true
			continue
		}
		out = append(out, t)
	}
	if !found {
		return fmt.Errorf("task not found")
	}
	f.Tasks = out
	return saveTasks(path, f)
}
