package saac

import (
	"path/filepath"
	"testing"
)

func TestCreateVerify(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "saac.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	acc, err := s.Create("alice", "pass123")
	if err != nil {
		t.Fatal(err)
	}
	if acc.Name != "alice" || acc.Salt == "" {
		t.Fatalf("bad acc: %+v", acc)
	}

	if _, err := s.Verify("alice", "pass123"); err != nil {
		t.Fatalf("verify ok got: %v", err)
	}
	if _, err := s.Verify("alice", "wrong"); err != ErrBadPwd {
		t.Fatalf("want ErrBadPwd got: %v", err)
	}
	if _, err := s.Verify("nobody", "pass"); err != ErrNotFound {
		t.Fatalf("want ErrNotFound got: %v", err)
	}
	if _, err := s.Create("alice", "x"); err != ErrDup {
		t.Fatalf("want ErrDup got: %v", err)
	}
}

func TestVerifyMD5(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.Create("m", "1234"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Verify("m", "81dc9bdb52d04dc20036dbd8313ed055"); err != nil {
		t.Fatalf("md5 lower: %v", err)
	}
	if _, err := st.Verify("m", "81DC9BDB52D04DC20036DBD8313ED055"); err != nil {
		t.Fatalf("md5 upper: %v", err)
	}
	if _, err := st.Verify("m", "00dc9bdb52d04dc20036dbd8313ed055"); err != ErrBadPwd {
		t.Fatalf("bad md5 accepted: %v", err)
	}
}
