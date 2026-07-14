package workspaces

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTasksCRUD(t *testing.T) {
	dir := t.TempDir()

	list, err := ListTasks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("want empty, got %d", len(list))
	}

	a, err := CreateTask(dir, "  first  ")
	if err != nil {
		t.Fatal(err)
	}
	if a.Title != "first" || a.Status != TaskTodo || a.ID == "" {
		t.Fatalf("bad create: %+v", a)
	}

	b, err := CreateTask(dir, "second")
	if err != nil {
		t.Fatal(err)
	}

	list, err = ListTasks(dir)
	if err != nil {
		t.Fatal(err)
	}
	// newest first
	if len(list) != 2 || list[0].ID != b.ID || list[1].ID != a.ID {
		t.Fatalf("list order: %+v", list)
	}

	// file lands under .agents/tasks.json
	if _, err := os.Stat(filepath.Join(dir, ".agents", TasksFile)); err != nil {
		t.Fatal(err)
	}

	st := TaskDoing
	u, err := UpdateTask(dir, a.ID, &st, nil)
	if err != nil {
		t.Fatal(err)
	}
	if u.Status != TaskDoing {
		t.Fatalf("status: %s", u.Status)
	}

	title := "renamed"
	u, err = UpdateTask(dir, a.ID, nil, &title)
	if err != nil {
		t.Fatal(err)
	}
	if u.Title != "renamed" || u.Status != TaskDoing {
		t.Fatalf("patch: %+v", u)
	}

	done := TaskDone
	if _, err := UpdateTask(dir, a.ID, &done, nil); err != nil {
		t.Fatal(err)
	}

	if err := DeleteTask(dir, a.ID); err != nil {
		t.Fatal(err)
	}
	list, _ = ListTasks(dir)
	if len(list) != 1 || list[0].ID != b.ID {
		t.Fatalf("after delete: %+v", list)
	}
	if err := DeleteTask(dir, a.ID); err == nil {
		t.Fatal("expected not found")
	}
}

func TestCreateTaskValidation(t *testing.T) {
	dir := t.TempDir()
	if _, err := CreateTask(dir, "   "); err == nil {
		t.Fatal("empty title should fail")
	}
	bad := TaskStatus("nope")
	if _, err := UpdateTask(dir, "x", &bad, nil); err == nil {
		t.Fatal("bad status should fail")
	}
}
