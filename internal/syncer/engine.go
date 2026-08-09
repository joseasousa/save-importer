package syncer

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Engine struct {
	Config Config
	Index  Index
}

func (e *Engine) Validate(direction Direction) error {
	if e.Config.Userdata == "" {
		return errors.New("diretório userdata não configurado")
	}
	if st, err := os.Stat(e.Config.Userdata); err != nil || !st.IsDir() {
		return fmt.Errorf("cartão 2/userdata indisponível: %s", e.Config.Userdata)
	}
	source, _ := e.roots(direction, Save)
	if st, err := os.Stat(source); err != nil || !st.IsDir() {
		return fmt.Errorf("origem não encontrada: %s", source)
	}
	return nil
}

func (e *Engine) roots(direction Direction, kind Kind) (string, string) {
	u := e.Config.Userdata
	mu := filepath.Join(u, "MUOS", "save", map[Kind]string{Save: "file", State: "state"}[kind])
	kn := e.knulliSaveRoot(kind)
	if direction == MuOSToKnulli {
		return mu, kn
	}
	return kn, mu
}

// knulliSaveRoot honors a generated RetroArch directory when it points inside
// userdata. Knulli normally generates per-system paths below /userdata/saves;
// the standard base remains the safe fallback for standalone emulators.
func (e *Engine) knulliSaveRoot(kind Kind) string {
	fallback := filepath.Join(e.Config.Userdata, "saves")
	configRoot := filepath.Join(e.Config.Userdata, "system", "configs")
	setting := "savefile_directory"
	if kind == State {
		setting = "savestate_directory"
	}
	var found string
	_ = filepath.WalkDir(configRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return fs.SkipAll
		}
		if entry.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".cfg" && ext != ".conf" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		for _, line := range strings.Split(string(data), "\n") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 || strings.TrimSpace(parts[0]) != setting {
				continue
			}
			value := strings.Trim(strings.TrimSpace(parts[1]), "\"")
			if strings.HasPrefix(value, "/userdata/saves") {
				rel := strings.TrimPrefix(value, "/userdata/")
				candidate := filepath.Clean(filepath.Join(e.Config.Userdata, filepath.FromSlash(rel)))
				// A core-specific subdirectory cannot safely become the global
				// root because the system name is appended later.
				if candidate == fallback {
					found = candidate
				}
			}
		}
		return nil
	})
	if found != "" {
		return found
	}
	return fallback
}

func (e *Engine) Scan(direction Direction, full bool) ([]Candidate, error) {
	if err := e.Validate(direction); err != nil {
		return nil, err
	}
	romSystems, err := scanROMs(filepath.Join(e.Config.Userdata, "roms"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	var out []Candidate
	for _, kind := range []Kind{Save, State} {
		sourceRoot, destRoot := e.roots(direction, kind)
		if _, err := os.Stat(sourceRoot); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(sourceRoot, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(sourceRoot, path)
			key := string(direction) + ":" + string(kind) + ":" + filepath.ToSlash(rel)
			if !full {
				if old, ok := e.Index.Files[key]; ok && old.Size == info.Size() && old.ModifiedNS == info.ModTime().UnixNano() {
					return nil
				}
			}
			hash, err := hashFile(path)
			if err != nil {
				return err
			}
			if !full {
				if old, ok := e.Index.Files[key]; ok && old.Hash == hash {
					return nil
				}
			}
			systems := e.matchSystems(rel, romSystems)
			c := Candidate{Source: path, Relative: rel, Kind: kind, Size: info.Size(), Modified: info.ModTime(), Hash: hash, Options: systems}
			if len(systems) == 1 {
				c.System = systems[0]
			} else {
				c.Ambiguous = true
			}
			if c.System != "" {
				c.Destination = destinationPath(direction, destRoot, c.System, rel)
				_, c.Conflict = statExists(c.Destination)
			}
			out = append(out, c)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Relative < out[j].Relative })
	return out, nil
}

func destinationPath(direction Direction, root, system, rel string) string {
	name := filepath.Base(rel)
	if direction == MuOSToKnulli {
		return filepath.Join(root, system, name)
	}
	// Keep the system folder in muOS so reverse imports remain deterministic.
	return filepath.Join(root, system, name)
}

func (e *Engine) Assign(c Candidate, direction Direction, system string) Candidate {
	_, root := e.roots(direction, c.Kind)
	c.System, c.Ambiguous = system, false
	c.Destination = destinationPath(direction, root, system, c.Relative)
	_, c.Conflict = statExists(c.Destination)
	return c
}

func (e *Engine) matchSystems(rel string, roms map[string]map[string]bool) []string {
	normal := normalize(rel)
	stem := normalize(saveStem(filepath.Base(rel)))
	var romMatches []string
	for system, names := range roms {
		if names[stem] {
			romMatches = append(romMatches, system)
		}
	}
	if len(romMatches) > 0 {
		sort.Strings(romMatches)
		return romMatches
	}
	set := map[string]bool{}
	for system, aliases := range e.Config.SystemAliases {
		if pathHasAlias(normal, system) {
			set[system] = true
		}
		for _, alias := range aliases {
			if pathHasAlias(normal, alias) {
				set[system] = true
			}
		}
	}
	result := make([]string, 0, len(set))
	for s := range set {
		result = append(result, s)
	}
	sort.Strings(result)
	return result
}

func pathHasAlias(normalPath, alias string) bool {
	a := normalize(alias)
	if len(a) >= 4 {
		return strings.Contains(normalPath, a)
	}
	for _, part := range strings.Split(normalPath, "/") {
		if part == a {
			return true
		}
	}
	return false
}

func scanROMs(root string) (map[string]map[string]bool, error) {
	out := map[string]map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) < 2 {
			return nil
		}
		system := parts[0]
		if out[system] == nil {
			out[system] = map[string]bool{}
		}
		out[system][normalize(romStem(d.Name()))] = true
		return nil
	})
	return out, err
}

func romStem(name string) string {
	for {
		ext := filepath.Ext(name)
		if ext == "" {
			return name
		}
		name = strings.TrimSuffix(name, ext)
		if !archiveExt(ext) {
			return name
		}
	}
}
func archiveExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".zip", ".7z", ".gz", ".chd", ".iso", ".cue":
		return true
	}
	return false
}
func saveStem(name string) string {
	lower := strings.ToLower(name)
	for _, marker := range []string{".state", ".srm", ".sav", ".eep", ".fla", ".rtc", ".mcr", ".st"} {
		if i := strings.LastIndex(lower, marker); i > 0 {
			return name[:i]
		}
	}
	return strings.TrimSuffix(name, filepath.Ext(name))
}
func normalize(s string) string {
	s = strings.ToLower(filepath.ToSlash(s))
	replacer := strings.NewReplacer(" ", "", "_", "", "-", "", ".", "")
	return replacer.Replace(s)
}

func (e *Engine) Copy(c Candidate, direction Direction, action ConflictAction) Result {
	r := Result{Source: c.Source, Destination: c.Destination, Action: action}
	if c.Ambiguous || c.Destination == "" {
		r.Err = errors.New("sistema de destino não definido")
		return r
	}
	if c.Conflict && action == KeepDestination {
		return r
	}
	if c.Conflict && action == PreserveBoth {
		c.Destination = uniquePath(c.Destination)
		r.Destination = c.Destination
	}
	if err := ensureSpace(filepath.Dir(c.Destination), c.Size); err != nil {
		r.Err = err
		return r
	}
	if err := copyAtomicVerified(c.Source, c.Destination, c.Hash); err != nil {
		r.Err = err
		return r
	}
	r.Bytes, r.Hash = c.Size, c.Hash
	info, _ := os.Stat(c.Source)
	key := string(direction) + ":" + string(c.Kind) + ":" + filepath.ToSlash(c.Relative)
	e.Index.Files[key] = IndexRecord{Size: c.Size, ModifiedNS: info.ModTime().UnixNano(), Hash: c.Hash, CopiedAt: time.Now()}
	return r
}

func copyAtomicVerified(source, destination, expectedHash string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	tmp := destination + ".importing"
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	syncErr := out.Sync()
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if syncErr != nil {
		_ = os.Remove(tmp)
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	h, err := hashFile(tmp)
	if err != nil || h != expectedHash {
		_ = os.Remove(tmp)
		if err != nil {
			return err
		}
		return errors.New("falha na validação SHA-256")
	}
	if err := os.Rename(tmp, destination); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func statExists(path string) (os.FileInfo, bool) { st, err := os.Stat(path); return st, err == nil }
func uniquePath(path string) string {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 1; ; i++ {
		p := fmt.Sprintf("%s.muos-%d%s", base, i, ext)
		if _, ok := statExists(p); !ok {
			return p
		}
	}
}
