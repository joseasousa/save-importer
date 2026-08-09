//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"
	"unicode"

	"github.com/jose/muos-save-importer/internal/syncer"
	"github.com/veandco/go-sdl2/sdl"
)

const width, height = 640, 480
const wrapWidth = 52

var appLog *os.File

type ui struct {
	win *sdl.Window
	ren *sdl.Renderer
}

func main() {
	userdata := envDefault("USERDATA", "/userdata")
	initLog(userdata)
	defer closeLog()
	defer func() {
		if r := recover(); r != nil {
			logf("panic: %v\n%s", r, debug.Stack())
			time.Sleep(3 * time.Second)
			os.Exit(2)
		}
	}()
	logf("starting app, USERDATA=%s", userdata)
	appDir := filepath.Join(userdata, "system", "muos-save-importer")
	cfgPath := filepath.Join(appDir, "systems.json")
	idxPath := filepath.Join(appDir, "index.json")
	cfg, err := syncer.LoadConfig(cfgPath, userdata)
	if err != nil {
		fatal(err)
	}
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		_ = syncer.SaveConfig(cfgPath, cfg)
	}
	idx, err := syncer.LoadIndex(idxPath)
	if err != nil {
		fatal(err)
	}
	engine := &syncer.Engine{Config: cfg, Index: idx}
	if err := sdl.Init(sdl.INIT_VIDEO | sdl.INIT_GAMECONTROLLER); err != nil {
		fatal(err)
	}
	defer sdl.Quit()
	logf("sdl initialized")
	var controllers []*sdl.GameController
	for joystick := 0; joystick < sdl.NumJoysticks(); joystick++ {
		if sdl.IsGameController(joystick) {
			if controller := sdl.GameControllerOpen(joystick); controller != nil {
				controllers = append(controllers, controller)
				defer controller.Close()
			}
		}
	}
	win, ren, err := sdl.CreateWindowAndRenderer(width, height, sdl.WINDOW_FULLSCREEN_DESKTOP)
	if err != nil {
		fatal(err)
	}
	defer win.Destroy()
	defer ren.Destroy()
	logf("window and renderer created")
	u := &ui{win: win, ren: ren}
	u.menu(engine, idxPath)
	logf("app exited normally")
}

func (u *ui) menu(e *syncer.Engine, idxPath string) {
	items := []string{"muOS → Knulli", "Knulli → muOS", "Histórico da última operação", "Refazer varredura completa", "Sair"}
	selected := 0
	full := false
	for {
		u.screen("Importador de saves", items, selected, "D-pad: navegar   A: confirmar   B: sair")
		switch waitKey() {
		case "up":
			selected = (selected + len(items) - 1) % len(items)
		case "down":
			selected = (selected + 1) % len(items)
		case "back":
			return
		case "ok":
			switch selected {
			case 0:
				u.run(e, syncer.MuOSToKnulli, idxPath, full)
				full = false
			case 1:
				u.run(e, syncer.KnulliToMuOS, idxPath, full)
				full = false
			case 2:
				u.history(e.Config.Userdata)
			case 3:
				full = true
				e.Index = syncer.Index{Version: 1, Files: map[string]syncer.IndexRecord{}}
				_ = syncer.SaveIndex(idxPath, e.Index)
				u.message("Índice limpo", "A próxima importação fará uma varredura completa.")
			case 4:
				return
			}
		}
	}
}

func (u *ui) run(e *syncer.Engine, d syncer.Direction, idxPath string, full bool) {
	logf("run direction=%s full=%v", d, full)
	if !u.confirm("Atenção aos save states", "Save states podem não funcionar entre núcleos ou versões diferentes. Os saves normais são mais portáveis. Continuar?") {
		logf("run cancelled before scan")
		return
	}
	u.message("Varredura", "Procurando saves novos ou modificados...")
	items, err := e.Scan(d, full)
	if err != nil {
		logf("scan error: %v", err)
		u.message("Não foi possível continuar", err.Error())
		return
	}
	logf("scan found %d candidate(s)", len(items))
	if len(items) == 0 {
		u.message("Tudo atualizado", "Nenhum save novo ou modificado foi encontrado.")
		return
	}
	var results []syncer.Result
	applyAll := syncer.ConflictAction("")
	for i, c := range items {
		if c.Ambiguous {
			opts := c.Options
			if len(opts) == 0 {
				opts = sortedSystems(e.Config.SystemAliases)
			}
			choice, ok := u.choose("Escolha o sistema: "+filepath.Base(c.Source), opts)
			if !ok {
				continue
			}
			c = e.Assign(c, d, choice)
		}
		action := syncer.Replace
		if c.Conflict {
			if applyAll != "" {
				action = applyAll
			} else {
				var ok bool
				action, applyAll, ok = u.conflict(c, i+1, len(items))
				if !ok {
					continue
				}
			}
		}
		u.message(fmt.Sprintf("Copiando %d/%d", i+1, len(items)), filepath.Base(c.Source))
		logf("copy %d/%d source=%s dest=%s action=%s", i+1, len(items), c.Source, c.Destination, action)
		results = append(results, e.Copy(c, d, action))
		if results[len(results)-1].Err != nil {
			logf("copy error: %v", results[len(results)-1].Err)
		}
	}
	if err := syncer.SaveIndex(idxPath, e.Index); err != nil {
		logf("save index error: %v", err)
	}
	report, reportErr := syncer.WriteReport(filepath.Join(e.Config.Userdata, "system", "logs", "muos-save-importer"), d, results)
	if reportErr != nil {
		logf("report error: %v", reportErr)
	}
	errs := 0
	for _, r := range results {
		if r.Err != nil {
			errs++
		}
	}
	msg := fmt.Sprintf("Processados: %d   Erros: %d", len(results), errs)
	if reportErr == nil {
		msg += "\nRelatório: " + report
	}
	u.message("Importação concluída", msg)
}

func (u *ui) confirm(title, message string) bool {
	items := []string{"Continuar", "Cancelar"}
	selected := 1
	for {
		lines := wrap(message, wrapWidth)
		u.screen(title, append(lines, items...), selected+len(lines), "A: confirmar   B: cancelar")
		switch waitKey() {
		case "up", "down":
			selected = 1 - selected
		case "ok":
			return selected == 0
		case "back":
			return false
		}
	}
}

func (u *ui) conflict(c syncer.Candidate, n, total int) (syncer.ConflictAction, syncer.ConflictAction, bool) {
	items := []string{"Manter destino", "Substituir pelo arquivo de origem", "Preservar ambos", "Cancelar este arquivo"}
	sel := 0
	for {
		u.screen(fmt.Sprintf("Conflito %d/%d", n, total), append([]string{filepath.Base(c.Source), fmt.Sprintf("Origem: %s | %s", human(c.Size), c.Modified.Format("02/01/2006 15:04")), ""}, items...), sel+3, "A: escolher   L1: aplicar escolha aos restantes   B: cancelar")
		switch waitKey() {
		case "up":
			sel = (sel + len(items) - 1) % len(items)
		case "down":
			sel = (sel + 1) % len(items)
		case "back":
			return "", "", false
		case "ok":
			if sel == 3 {
				return "", "", false
			}
			return []syncer.ConflictAction{syncer.KeepDestination, syncer.Replace, syncer.PreserveBoth}[sel], "", true
		case "all":
			if sel == 3 {
				return "", "", false
			}
			a := []syncer.ConflictAction{syncer.KeepDestination, syncer.Replace, syncer.PreserveBoth}[sel]
			return a, a, true
		}
	}
}

func (u *ui) choose(title string, items []string) (string, bool) {
	sel := 0
	for {
		u.screen(title, items, sel, "A: selecionar   B: ignorar")
		switch waitKey() {
		case "up":
			sel = (sel + len(items) - 1) % len(items)
		case "down":
			sel = (sel + 1) % len(items)
		case "ok":
			return items[sel], true
		case "back":
			return "", false
		}
	}
}
func (u *ui) history(userdata string) {
	dir := filepath.Join(userdata, "system", "logs", "muos-save-importer")
	entries, _ := os.ReadDir(dir)
	if len(entries) == 0 {
		u.message("Histórico", "Nenhuma importação registrada.")
		return
	}
	last := entries[len(entries)-1]
	b, err := os.ReadFile(filepath.Join(dir, last.Name()))
	if err != nil {
		u.message("Histórico", err.Error())
		return
	}
	text := string(b)
	if len(text) > 1500 {
		text = text[:1500] + "\n..."
	}
	u.message("Última operação", text)
}

func (u *ui) screen(title string, items []string, selected int, footer string) {
	u.ren.SetDrawColor(12, 18, 29, 255)
	u.ren.Clear()
	u.text(title, 28, 22, 3, sdl.Color{R: 91, G: 192, B: 235, A: 255})
	y := 75
	for i, item := range items {
		color := sdl.Color{R: 220, G: 225, B: 230, A: 255}
		if i == selected {
			u.ren.SetDrawColor(35, 74, 105, 255)
			u.ren.FillRect(&sdl.Rect{X: 20, Y: int32(y - 5), W: 600, H: 34})
			color = sdl.Color{R: 255, G: 214, B: 102, A: 255}
		}
		for _, line := range wrap(item, wrapWidth) {
			u.text(line, 32, y, 2, color)
			y += 24
		}
		y += 7
		if y > 420 {
			break
		}
	}
	u.text(footer, 20, 449, 2, sdl.Color{R: 145, G: 155, B: 165, A: 255})
	u.ren.Present()
}
func (u *ui) message(title, msg string) {
	u.screen(title, wrap(msg, wrapWidth), -1, "A/B: voltar")
	for {
		key := waitKey()
		if key == "ok" || key == "back" {
			return
		}
	}
}

func (u *ui) text(text string, x, y, scale int, color sdl.Color) {
	u.ren.SetDrawColor(color.R, color.G, color.B, color.A)
	cursor := x
	for _, ch := range displayText(text) {
		if ch == ' ' {
			cursor += 4 * scale
			continue
		}
		pattern, ok := bitmapFont[ch]
		if !ok {
			pattern = bitmapFont['?']
		}
		for row, bits := range pattern {
			for col, bit := range bits {
				if bit == '1' {
					u.ren.FillRect(&sdl.Rect{
						X: int32(cursor + col*scale),
						Y: int32(y + row*scale),
						W: int32(scale),
						H: int32(scale),
					})
				}
			}
		}
		cursor += 6 * scale
	}
}

func waitKey() string {
	for event := sdl.WaitEvent(); event != nil; event = sdl.WaitEvent() {
		switch value := event.(type) {
		case *sdl.QuitEvent:
			return "back"
		case *sdl.KeyboardEvent:
			if value.Type != sdl.KEYDOWN {
				continue
			}
			switch value.Keysym.Sym {
			case sdl.K_UP:
				return "up"
			case sdl.K_DOWN:
				return "down"
			case sdl.K_RETURN, sdl.K_SPACE:
				return "ok"
			case sdl.K_ESCAPE, sdl.K_BACKSPACE:
				return "back"
			case sdl.K_q:
				return "all"
			}
		case *sdl.ControllerButtonEvent:
			if value.Type != sdl.CONTROLLERBUTTONDOWN {
				continue
			}
			switch value.Button {
			case sdl.CONTROLLER_BUTTON_DPAD_UP:
				return "up"
			case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
				return "down"
			case sdl.CONTROLLER_BUTTON_A:
				return "ok"
			case sdl.CONTROLLER_BUTTON_B:
				return "back"
			case sdl.CONTROLLER_BUTTON_LEFTSHOULDER:
				return "all"
			}
		}
	}
	return "back"
}
func wrap(s string, n int) []string {
	var out []string
	for _, raw := range strings.Split(s, "\n") {
		words := strings.Fields(raw)
		line := ""
		for _, w := range words {
			if len(line)+len(w)+1 > n && line != "" {
				out = append(out, line)
				line = w
			} else if line == "" {
				line = w
			} else {
				line += " " + w
			}
		}
		out = append(out, line)
	}
	return out
}
func sortedSystems(m map[string][]string) []string {
	r := make([]string, 0, len(m))
	for s := range m {
		r = append(r, s)
	}
	for i := range r {
		for j := i + 1; j < len(r); j++ {
			if r[j] < r[i] {
				r[i], r[j] = r[j], r[i]
			}
		}
	}
	return r
}
func human(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1f KiB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1f MiB", float64(n)/(1024*1024))
}
func envDefault(k, v string) string {
	if x := os.Getenv(k); x != "" {
		return x
	}
	return v
}
func fatal(err error) {
	logf("fatal: %v", err)
	fmt.Fprintln(os.Stderr, err)
	time.Sleep(3 * time.Second)
	os.Exit(1)
}

func initLog(userdata string) {
	dir := filepath.Join(userdata, "system", "logs", "muos-save-importer")
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "log setup failed: %v\n", err)
		return
	}
	path := filepath.Join(dir, "app.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "log open failed: %v\n", err)
		return
	}
	appLog = f
}

func closeLog() {
	if appLog != nil {
		_ = appLog.Close()
	}
}

func logf(format string, args ...any) {
	line := fmt.Sprintf(time.Now().Format(time.RFC3339)+" "+format+"\n", args...)
	if appLog != nil {
		_, _ = appLog.WriteString(line)
	}
	_, _ = os.Stderr.WriteString(line)
}

func displayText(s string) string {
	replacer := strings.NewReplacer(
		"→", "->", "←", "<-", "↔", "<->",
		"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
		"Á", "A", "À", "A", "Â", "A", "Ã", "A", "Ä", "A",
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"É", "E", "È", "E", "Ê", "E", "Ë", "E",
		"í", "i", "ì", "i", "î", "i", "ï", "i",
		"Í", "I", "Ì", "I", "Î", "I", "Ï", "I",
		"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
		"Ó", "O", "Ò", "O", "Ô", "O", "Õ", "O", "Ö", "O",
		"ú", "u", "ù", "u", "û", "u", "ü", "u",
		"Ú", "U", "Ù", "U", "Û", "U", "Ü", "U",
		"ç", "c", "Ç", "C",
	)
	var b strings.Builder
	for _, r := range replacer.Replace(s) {
		if r <= unicode.MaxASCII {
			b.WriteRune(unicode.ToUpper(r))
		}
	}
	return b.String()
}

var bitmapFont = map[rune][7]string{
	' ':  {"00000", "00000", "00000", "00000", "00000", "00000", "00000"},
	'!':  {"00100", "00100", "00100", "00100", "00100", "00000", "00100"},
	'"':  {"01010", "01010", "01010", "00000", "00000", "00000", "00000"},
	'#':  {"01010", "01010", "11111", "01010", "11111", "01010", "01010"},
	'%':  {"11001", "11010", "00100", "01000", "10110", "00110", "00000"},
	'&':  {"01100", "10010", "10100", "01000", "10101", "10010", "01101"},
	'\'': {"00100", "00100", "01000", "00000", "00000", "00000", "00000"},
	'(':  {"00010", "00100", "01000", "01000", "01000", "00100", "00010"},
	')':  {"01000", "00100", "00010", "00010", "00010", "00100", "01000"},
	'*':  {"00000", "00100", "10101", "01110", "10101", "00100", "00000"},
	'+':  {"00000", "00100", "00100", "11111", "00100", "00100", "00000"},
	',':  {"00000", "00000", "00000", "00000", "00100", "00100", "01000"},
	'-':  {"00000", "00000", "00000", "11111", "00000", "00000", "00000"},
	'.':  {"00000", "00000", "00000", "00000", "00000", "01100", "01100"},
	'/':  {"00001", "00010", "00100", "01000", "10000", "00000", "00000"},
	'0':  {"01110", "10001", "10011", "10101", "11001", "10001", "01110"},
	'1':  {"00100", "01100", "00100", "00100", "00100", "00100", "01110"},
	'2':  {"01110", "10001", "00001", "00010", "00100", "01000", "11111"},
	'3':  {"11110", "00001", "00001", "01110", "00001", "00001", "11110"},
	'4':  {"00010", "00110", "01010", "10010", "11111", "00010", "00010"},
	'5':  {"11111", "10000", "10000", "11110", "00001", "00001", "11110"},
	'6':  {"00110", "01000", "10000", "11110", "10001", "10001", "01110"},
	'7':  {"11111", "00001", "00010", "00100", "01000", "01000", "01000"},
	'8':  {"01110", "10001", "10001", "01110", "10001", "10001", "01110"},
	'9':  {"01110", "10001", "10001", "01111", "00001", "00010", "11100"},
	':':  {"00000", "01100", "01100", "00000", "01100", "01100", "00000"},
	';':  {"00000", "01100", "01100", "00000", "01100", "00100", "01000"},
	'<':  {"00010", "00100", "01000", "10000", "01000", "00100", "00010"},
	'=':  {"00000", "00000", "11111", "00000", "11111", "00000", "00000"},
	'>':  {"01000", "00100", "00010", "00001", "00010", "00100", "01000"},
	'?':  {"01110", "10001", "00001", "00010", "00100", "00000", "00100"},
	'[':  {"01110", "01000", "01000", "01000", "01000", "01000", "01110"},
	'\\': {"10000", "01000", "00100", "00010", "00001", "00000", "00000"},
	']':  {"01110", "00010", "00010", "00010", "00010", "00010", "01110"},
	'_':  {"00000", "00000", "00000", "00000", "00000", "00000", "11111"},
	'|':  {"00100", "00100", "00100", "00100", "00100", "00100", "00100"},
	'A':  {"01110", "10001", "10001", "11111", "10001", "10001", "10001"},
	'B':  {"11110", "10001", "10001", "11110", "10001", "10001", "11110"},
	'C':  {"01110", "10001", "10000", "10000", "10000", "10001", "01110"},
	'D':  {"11110", "10001", "10001", "10001", "10001", "10001", "11110"},
	'E':  {"11111", "10000", "10000", "11110", "10000", "10000", "11111"},
	'F':  {"11111", "10000", "10000", "11110", "10000", "10000", "10000"},
	'G':  {"01110", "10001", "10000", "10111", "10001", "10001", "01110"},
	'H':  {"10001", "10001", "10001", "11111", "10001", "10001", "10001"},
	'I':  {"01110", "00100", "00100", "00100", "00100", "00100", "01110"},
	'J':  {"00111", "00010", "00010", "00010", "10010", "10010", "01100"},
	'K':  {"10001", "10010", "10100", "11000", "10100", "10010", "10001"},
	'L':  {"10000", "10000", "10000", "10000", "10000", "10000", "11111"},
	'M':  {"10001", "11011", "10101", "10101", "10001", "10001", "10001"},
	'N':  {"10001", "11001", "10101", "10011", "10001", "10001", "10001"},
	'O':  {"01110", "10001", "10001", "10001", "10001", "10001", "01110"},
	'P':  {"11110", "10001", "10001", "11110", "10000", "10000", "10000"},
	'Q':  {"01110", "10001", "10001", "10001", "10101", "10010", "01101"},
	'R':  {"11110", "10001", "10001", "11110", "10100", "10010", "10001"},
	'S':  {"01111", "10000", "10000", "01110", "00001", "00001", "11110"},
	'T':  {"11111", "00100", "00100", "00100", "00100", "00100", "00100"},
	'U':  {"10001", "10001", "10001", "10001", "10001", "10001", "01110"},
	'V':  {"10001", "10001", "10001", "10001", "10001", "01010", "00100"},
	'W':  {"10001", "10001", "10001", "10101", "10101", "10101", "01010"},
	'X':  {"10001", "10001", "01010", "00100", "01010", "10001", "10001"},
	'Y':  {"10001", "10001", "01010", "00100", "00100", "00100", "00100"},
	'Z':  {"11111", "00001", "00010", "00100", "01000", "10000", "11111"},
}
