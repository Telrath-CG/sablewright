package main

import "testing"

func projectNames(ps []Project) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Name
	}
	return out
}

// A project exists because minis name it, the way a brand exists because
// paints name it. Nothing is created first and nothing is left behind.
func TestProjectsRollUpWhatIsFiledUnderThem(t *testing.T) {
	s := &Store{data: Data{Models: []Model{
		{ID: 1, Name: "Plague Marines", Project: "Cadia Push", Status: "In Progress",
			Count: 10, Done: 6,
			Sessions: []Session{{ID: 1, Date: "2026-08-01", Minutes: 180, Notes: "x"}}},
		{ID: 2, Name: "Lord of Contagion", Project: "Cadia Push", Status: "Complete",
			Count: 1},
		{ID: 3, Name: "Poxwalkers", Project: "Cadia Push", Status: "Backlog", Count: 20},
		{ID: 4, Name: "Unrelated", Status: "Backlog", Count: 3},
	}}}

	got := s.Projects(day("2026-08-08"))

	if len(got) != 1 {
		t.Fatalf("projects = %v, want just the one in use", projectNames(got))
	}
	p := got[0]
	if p.Minis != 31 || p.Done != 7 {
		t.Errorf("rolled up %d of %d minis, want 7 of 31", p.Done, p.Minis)
	}
	if p.Entries != 3 {
		t.Errorf("entries = %d, want 3", p.Entries)
	}
	if p.Minutes != 180 || p.Sessions != 1 {
		t.Errorf("time = %d minutes over %d sessions, want 180 over 1", p.Minutes, p.Sessions)
	}
	// Next up skips what's finished, and leads with what's on the desk.
	if len(p.Next) != 2 || p.Next[0].Name != "Plague Marines" {
		t.Errorf("next up = %v, want the unfinished two, in progress first", p.Next)
	}
}

// A deadline is the reason to write a project down at all, so dated ones come
// first in the order they fall due. The countdown is worked out here so every
// screen agrees on what today is.
func TestProjectsLeadWithTheSoonestDeadline(t *testing.T) {
	s := &Store{data: Data{
		Projects: []ProjectMeta{
			{Name: "Autumn Doubles", Due: "2026-10-03"},
			{Name: "Cadia Push", Due: "2026-08-15", Notes: "list locked"},
		},
		Models: []Model{
			{ID: 1, Project: "Cadia Push", Status: "Backlog", Count: 1},
			{ID: 2, Project: "Autumn Doubles", Status: "Backlog", Count: 1},
			{ID: 3, Project: "Someday", Status: "Backlog", Count: 1},
		},
	}}

	got := s.Projects(day("2026-08-08"))

	if names := projectNames(got); len(names) != 3 ||
		names[0] != "Cadia Push" || names[2] != "Someday" {
		t.Fatalf("order = %v, want the dated ones first and the undated last", names)
	}
	if got[0].DaysLeft != 7 {
		t.Errorf("days left = %d, want 7", got[0].DaysLeft)
	}
	if got[2].DaysLeft != 0 || got[2].Due != "" {
		t.Errorf("an undated project reported %d days left", got[2].DaysLeft)
	}
}

// A deadline with nothing filed under it yet is a plan, and losing the card
// the moment it's written would be surprising.
func TestProjectsKeepAPlanWithNoMinisYet(t *testing.T) {
	s := &Store{data: Data{
		Projects: []ProjectMeta{{Name: "Next Year's Army", Due: "2027-01-01"}},
	}}

	got := s.Projects(day("2026-08-08"))

	if len(got) != 1 || got[0].Minis != 0 {
		t.Errorf("projects = %v, want the empty plan kept", got)
	}
}

// Free text is only workable if a typo can be fixed in one place.
func TestRenameProjectRetagsEveryMiniAndCarriesTheDeadline(t *testing.T) {
	s := newTempStore(t)
	s.data = Data{NextID: 100,
		Projects: []ProjectMeta{{Name: "Cadia Push", Due: "2026-08-15"}},
		Models: []Model{
			{ID: 1, Name: "A", Project: "Cadia Push", Status: "Backlog", Count: 1},
			{ID: 2, Name: "B", Project: "Cadia Push", Status: "Backlog", Count: 1},
			{ID: 3, Name: "C", Project: "Other", Status: "Backlog", Count: 1},
		}}

	moved, err := s.RenameProject("Cadia Push", "Cadian Push")
	if err != nil {
		t.Fatalf("RenameProject(): %v", err)
	}
	if moved != 2 {
		t.Errorf("moved %d minis, want 2", moved)
	}

	got := s.Projects(day("2026-08-08"))
	if got[0].Name != "Cadian Push" || got[0].Due != "2026-08-15" {
		t.Errorf("after the rename: %+v, want the new name keeping the deadline", got[0])
	}
	if s.data.Models[2].Project != "Other" {
		t.Error("a mini in another project was re-tagged too")
	}
}

// Ungrouping dissolves the project without touching the minis in it.
func TestRenameProjectToNothingUngroupsWithoutDeleting(t *testing.T) {
	s := newTempStore(t)
	s.data = Data{NextID: 100,
		Projects: []ProjectMeta{{Name: "Cadia Push", Due: "2026-08-15"}},
		Models: []Model{{ID: 1, Name: "A", Project: "Cadia Push",
			Status: "Backlog", Count: 1}}}

	if _, err := s.RenameProject("Cadia Push", ""); err != nil {
		t.Fatalf("RenameProject(): %v", err)
	}

	if got := s.Projects(day("2026-08-08")); len(got) != 0 {
		t.Errorf("projects = %v, want none left", projectNames(got))
	}
	if len(s.data.Models) != 1 || s.data.Models[0].Name != "A" {
		t.Error("ungrouping took the mini with it")
	}
}

// A project with neither a deadline nor notes has nothing worth storing: it
// still exists for as long as minis name it.
func TestSaveProjectDropsAnEmptyRecord(t *testing.T) {
	s := newTempStore(t)
	s.data = Data{NextID: 100, Projects: []ProjectMeta{{Name: "Cadia Push", Due: "2026-08-15"}}}

	if err := s.SaveProject(ProjectMeta{Name: "Cadia Push"}); err != nil {
		t.Fatalf("SaveProject(): %v", err)
	}

	if len(s.data.Projects) != 0 {
		t.Errorf("stored %+v, want the emptied record dropped", s.data.Projects)
	}
}
