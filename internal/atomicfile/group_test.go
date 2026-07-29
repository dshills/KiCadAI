package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGroupCommitAndRollbackRetainOneProjectState(t *testing.T) {
	t.Run("commit", func(t *testing.T) {
		root, first, second := oldProject(t)
		group, err := BeginGroup(replacementMutations(first, second), GroupOptions{Root: root})
		if err != nil {
			t.Fatal(err)
		}
		assertProject(t, first, second, "new-first", "new-second")
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(JournalRelativePath))); err != nil {
			t.Fatalf("journal was discarded before application validation: %v", err)
		}
		if err := group.Commit(); err != nil {
			t.Fatal(err)
		}
		assertProject(t, first, second, "new-first", "new-second")
		assertNoJournal(t, root)
	})

	t.Run("rollback", func(t *testing.T) {
		root, first, second := oldProject(t)
		group, err := BeginGroup(replacementMutations(first, second), GroupOptions{Root: root})
		if err != nil {
			t.Fatal(err)
		}
		if err := group.Rollback(); err != nil {
			t.Fatal(err)
		}
		assertProject(t, first, second, "old-first", "old-second")
		assertNoJournal(t, root)
	})
}

func TestGroupFaultRecoveryIsAllOldOrAllNew(t *testing.T) {
	tests := []struct {
		name       string
		transition Transition
		occurrence int
		wantFirst  string
		wantSecond string
		commit     bool
	}{
		{name: "staging", transition: TransitionStaging, occurrence: 1, wantFirst: "old-first", wantSecond: "old-second"},
		{name: "file_sync", transition: TransitionFileSync, occurrence: 1, wantFirst: "old-first", wantSecond: "old-second"},
		{name: "journal_sync", transition: TransitionJournalSync, occurrence: 1, wantFirst: "old-first", wantSecond: "old-second"},
		{name: "replacement", transition: TransitionReplacement, occurrence: 1, wantFirst: "old-first", wantSecond: "old-second"},
		{name: "directory_sync", transition: TransitionDirectorySync, occurrence: 1, wantFirst: "old-first", wantSecond: "old-second"},
		{name: "staged_validation", transition: TransitionValidation, occurrence: 1, wantFirst: "old-first", wantSecond: "old-second"},
		{name: "committed_validation", transition: TransitionValidation, occurrence: 2, wantFirst: "old-first", wantSecond: "old-second"},
		{name: "cleanup", transition: TransitionCleanup, occurrence: 1, wantFirst: "new-first", wantSecond: "new-second", commit: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, first, second := oldProject(t)
			count := 0
			options := GroupOptions{
				Root:            root,
				PreserveOnError: true,
				Fault: func(transition Transition) error {
					if transition != test.transition {
						return nil
					}
					count++
					if count == test.occurrence {
						return errors.New("injected fault")
					}
					return nil
				},
			}
			group, err := BeginGroup(replacementMutations(first, second), options)
			if test.commit {
				if err != nil {
					t.Fatalf("begin group: %v", err)
				}
				err = group.Commit()
			}
			if err == nil {
				t.Fatal("expected injected fault")
			}
			if err := RecoverGroup(root); err != nil {
				t.Fatalf("recover: %v", err)
			}
			assertProject(t, first, second, test.wantFirst, test.wantSecond)
			assertNoJournal(t, root)
		})
	}
}

func TestRollbackFailureIsVisibleAndRecoverable(t *testing.T) {
	root, first, second := oldProject(t)
	group, err := BeginGroup(replacementMutations(first, second), GroupOptions{
		Root: root,
		Fault: func(transition Transition) error {
			if transition == TransitionRollback {
				return errors.New("injected rollback failure")
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := group.Rollback(); err == nil {
		t.Fatal("expected rollback failure")
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(JournalRelativePath))); err != nil {
		t.Fatalf("rollback failure did not retain journal: %v", err)
	}
	if err := RecoverGroup(root); err != nil {
		t.Fatal(err)
	}
	assertProject(t, first, second, "old-first", "old-second")
	assertNoJournal(t, root)
}

func TestGroupDeleteRollsBackAndCommits(t *testing.T) {
	t.Run("rollback", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "stale.txt")
		if err := os.WriteFile(target, []byte("old"), 0o640); err != nil {
			t.Fatal(err)
		}
		group, err := BeginGroup([]Mutation{{Path: target, Delete: true}}, GroupOptions{Root: root})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatalf("delete was not applied: %v", err)
		}
		if err := group.Rollback(); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(target)
		if err != nil || string(data) != "old" {
			t.Fatalf("rollback data=%q err=%v", data, err)
		}
		info, err := os.Stat(target)
		if err != nil || info.Mode().Perm() != 0o640 {
			t.Fatalf("rollback mode=%v err=%v", info.Mode().Perm(), err)
		}
	})

	t.Run("commit", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "stale.txt")
		if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
		group, err := BeginGroup([]Mutation{{Path: target, Delete: true}}, GroupOptions{Root: root})
		if err != nil {
			t.Fatal(err)
		}
		if err := group.Commit(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatalf("committed delete remains: %v", err)
		}
	})
}

func TestGroupAdoptCurrentRetainsOriginalRollbackIdentity(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "board.kicad_pcb")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	group, err := BeginGroup([]Mutation{{Path: target, Data: []byte("generated"), Mode: 0o644}}, GroupOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("zone-refilled"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := group.AdoptCurrent(); err != nil {
		t.Fatal(err)
	}
	if err := group.Rollback(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "old" {
		t.Fatalf("rollback data=%q err=%v", data, err)
	}
}

func oldProject(t *testing.T) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	first := filepath.Join(root, "first.txt")
	second := filepath.Join(root, "second.txt")
	if err := os.WriteFile(first, []byte("old-first"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("old-second"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, first, second
}

func replacementMutations(first string, second string) []Mutation {
	return []Mutation{
		{Path: first, Data: []byte("new-first"), Mode: 0o600},
		{Path: second, Data: []byte("new-second"), Mode: 0o644},
	}
}

func assertProject(t *testing.T, first string, second string, wantFirst string, wantSecond string) {
	t.Helper()
	firstData, firstErr := os.ReadFile(first)
	secondData, secondErr := os.ReadFile(second)
	if firstErr != nil || secondErr != nil {
		t.Fatalf("read project: first=%v second=%v", firstErr, secondErr)
	}
	if string(firstData) != wantFirst || string(secondData) != wantSecond {
		t.Fatalf("project first=%q second=%q, want first=%q second=%q", firstData, secondData, wantFirst, wantSecond)
	}
}

func assertNoJournal(t *testing.T, root string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(JournalRelativePath))); !os.IsNotExist(err) {
		t.Fatalf("journal remains after resolved outcome: %v", err)
	}
}
