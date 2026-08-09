package syncer

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}

func setup(t *testing.T) (string, *Engine) {
	t.Helper()
	root := t.TempDir()
	cfg := DefaultConfig(root)
	return root, &Engine{Config: cfg, Index: Index{Version: 1, Files: map[string]IndexRecord{}}}
}

func TestScanMapsSaveUsingROMName(t *testing.T) {
	root, e := setup(t)
	write(t, filepath.Join(root, "roms", "gba", "Pokemon Emerald.gba"), "rom")
	write(t, filepath.Join(root, "MUOS", "save", "file", "mGBA", "Pokemon Emerald.srm"), "save")
	got, err := e.Scan(MuOSToKnulli, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 candidate, got %d", len(got))
	}
	if got[0].System != "gba" || got[0].Ambiguous {
		t.Fatalf("unexpected mapping: %#v", got[0])
	}
	want := filepath.Join(root, "saves", "gba", "Pokemon Emerald.srm")
	if got[0].Destination != want {
		t.Fatalf("want %s got %s", want, got[0].Destination)
	}
}

func TestCopyReplaceIsVerifiedAndIncremental(t *testing.T) {
	root, e := setup(t)
	source := filepath.Join(root, "MUOS", "save", "file", "gba", "game.srm")
	write(t, source, "new-save")
	write(t, filepath.Join(root, "roms", "gba", "game.gba"), "rom")
	dest := filepath.Join(root, "saves", "gba", "game.srm")
	write(t, dest, "old-save")
	items, err := e.Scan(MuOSToKnulli, false)
	if err != nil {
		t.Fatal(err)
	}
	if !items[0].Conflict {
		t.Fatal("expected conflict")
	}
	r := e.Copy(items[0], MuOSToKnulli, Replace)
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	b, _ := os.ReadFile(dest)
	if string(b) != "new-save" {
		t.Fatalf("destination=%q", b)
	}
	items, err = e.Scan(MuOSToKnulli, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("incremental scan returned %d", len(items))
	}
}

func TestKeepNeverChangesDestination(t *testing.T) {
	root, e := setup(t)
	src := filepath.Join(root, "source.srm")
	dst := filepath.Join(root, "dest.srm")
	write(t, src, "source")
	write(t, dst, "destination")
	h, _ := hashFile(src)
	r := e.Copy(Candidate{Source: src, Destination: dst, Relative: "source.srm", System: "gba", Kind: Save, Size: 6, Hash: h, Conflict: true}, MuOSToKnulli, KeepDestination)
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	b, _ := os.ReadFile(dst)
	if string(b) != "destination" {
		t.Fatal("destination was modified")
	}
}

func TestPreserveBoth(t *testing.T) {
	root, e := setup(t)
	src := filepath.Join(root, "source.srm")
	dst := filepath.Join(root, "game.srm")
	write(t, src, "source")
	write(t, dst, "destination")
	h, _ := hashFile(src)
	r := e.Copy(Candidate{Source: src, Destination: dst, Relative: "source.srm", System: "gba", Kind: Save, Size: 6, Hash: h, Conflict: true}, MuOSToKnulli, PreserveBoth)
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	if r.Destination == dst {
		t.Fatal("expected alternate destination")
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatal("original destination missing")
	}
}

func TestCorruptIndexIsRebuilt(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "index.json")
	write(t, path, "not json")
	idx, err := LoadIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	if idx.Version != 1 || idx.Files == nil {
		t.Fatal("index not rebuilt")
	}
	if _, err := os.Stat(path + ".corrupt"); err != nil {
		t.Fatal("corrupt backup missing")
	}
}
