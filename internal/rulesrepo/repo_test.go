package rulesrepo

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadVersion_RejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	repo := New(dir)
	if err := repo.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	secret := filepath.Join(dir, "outside.json")
	if err := os.WriteFile(secret, []byte(`{"n9e":{"user_token":"leaked"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := repo.ReadVersion("../outside")
	if !errors.Is(err, ErrInvalidVersion) {
		t.Fatalf("expected ErrInvalidVersion, got %v", err)
	}

	_, _, err = repo.ReadVersion("")
	if !errors.Is(err, ErrInvalidVersion) {
		t.Fatalf("empty: expected ErrInvalidVersion, got %v", err)
	}

	_, _, err = repo.Rollback("..\\outside", "nope", "test")
	if !errors.Is(err, ErrInvalidVersion) {
		t.Fatalf("rollback: expected ErrInvalidVersion, got %v", err)
	}
}

func TestPublish_WritesOwnerOnlyFiles(t *testing.T) {
	dir := t.TempDir()
	repo := New(dir)
	ver, _, err := repo.Publish(RuleSet{}, "init", "test")
	if err != nil {
		t.Fatal(err)
	}

	st, err := os.Stat(repo.CurrentPath())
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("current.json mode=%o", st.Mode().Perm())
	}

	vp := filepath.Join(repo.VersionsDir(), ver+".json")
	st, err = os.Stat(vp)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("version file mode=%o", st.Mode().Perm())
	}

	_, hash, err := repo.ReadVersion(ver)
	if err != nil {
		t.Fatal(err)
	}
	if hash == "" {
		t.Fatal("expected version hash")
	}
}
