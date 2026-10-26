package migrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type memoryStore struct {
	applied map[int64]string
	calls   []string
}

func (s *memoryStore) Applied(context.Context) (map[int64]string, error) {
	out := map[int64]string{}
	for k, v := range s.applied {
		out[k] = v
	}
	return out, nil
}
func (s *memoryStore) Apply(_ context.Context, m Migration) error {
	if _, ok := s.applied[m.Version]; ok {
		return nil
	}
	s.calls = append(s.calls, "up:"+m.Name)
	s.applied[m.Version] = m.Name
	return nil
}
func (s *memoryStore) Rollback(_ context.Context, m Migration) error {
	if _, ok := s.applied[m.Version]; !ok {
		return nil
	}
	s.calls = append(s.calls, "down:"+m.Name)
	delete(s.applied, m.Version)
	return nil
}

func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, m := range []Migration{{1, "first", "SELECT 1;", "SELECT 2;"}, {2, "second", "SELECT 3;", "SELECT 4;"}} {
		if err := os.WriteFile(filepath.Join(dir, filepathName(m, "up")), []byte(m.Up), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, filepathName(m, "down")), []byte(m.Down), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
func filepathName(m Migration, direction string) string {
	return fmt.Sprintf("%06d_%s.%s.sql", m.Version, m.Name, direction)
}

func TestRunnerOrderingAndDuplicateDetection(t *testing.T) {
	dir := fixture(t)
	s := &memoryStore{applied: map[int64]string{}}
	r := Runner{Dir: dir, Store: s}
	if err := r.Up(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"up:first", "up:second"}
	if !reflect.DeepEqual(s.calls, want) {
		t.Fatalf("calls=%v want %v", s.calls, want)
	}
	if err := r.Up(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.calls, want) {
		t.Fatalf("repeated up executed migrations: %v", s.calls)
	}
}

func TestRunnerRollbackLatestFirst(t *testing.T) {
	dir := fixture(t)
	s := &memoryStore{applied: map[int64]string{1: "first", 2: "second"}}
	r := Runner{Dir: dir, Store: s}
	if err := r.Down(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.calls, []string{"down:second"}) {
		t.Fatalf("calls=%v", s.calls)
	}
	if _, ok := s.applied[1]; !ok {
		t.Fatal("older migration was rolled back")
	}
	if _, ok := s.applied[2]; ok {
		t.Fatal("latest migration remains applied")
	}
}

func TestLoadRejectsIncompleteAndDuplicateVersions(t *testing.T) {
	t.Run("incomplete pair", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "000001_one.up.sql"), []byte("SELECT 1"), 0600)
		if _, err := Load(dir); err == nil {
			t.Fatal("expected missing down migration error")
		}
	})
	t.Run("same version different name", func(t *testing.T) {
		dir := t.TempDir()
		for _, n := range []string{"one", "two"} {
			_ = os.WriteFile(filepath.Join(dir, "000001_"+n+".up.sql"), []byte("SELECT 1"), 0600)
			_ = os.WriteFile(filepath.Join(dir, "000001_"+n+".down.sql"), []byte("SELECT 1"), 0600)
		}
		if _, err := Load(dir); err == nil {
			t.Fatal("expected duplicate version error")
		}
	})
}

func TestValidateAppliedRejectsUnknownDatabaseVersion(t *testing.T) {
	err := validateApplied([]Migration{{Version: 1, Name: "one"}}, map[int64]string{2: "missing"})
	if err == nil {
		t.Fatal("expected unknown migration error")
	}
}

func TestNonTransactionalDirective(t *testing.T) {
	if !IsNonTransactional("-- migrate:no-transaction\nCREATE INDEX CONCURRENTLY x ON t (id)") {
		t.Fatal("directive was not recognized")
	}
	if IsNonTransactional("SELECT 1") {
		t.Fatal("ordinary SQL incorrectly marked non-transactional")
	}
}
