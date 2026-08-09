//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jose/muos-save-importer/internal/syncer"
	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

const width, height = 640, 480

type ui struct {
	win         *sdl.Window
	ren         *sdl.Renderer
	font, small *ttf.Font
}

func main() {
	userdata := envDefault("USERDATA", "/userdata")
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
	if err := ttf.Init(); err != nil {
		fatal(err)
	}
	defer ttf.Quit()
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
	fontPath := findFont()
	font, err := ttf.OpenFont(fontPath, 24)
	if err != nil {
		fatal(err)
	}
	defer font.Close()
	small, err := ttf.OpenFont(fontPath, 17)
	if err != nil {
		fatal(err)
	}
	defer small.Close()
	u := &ui{win: win, ren: ren, font: font, small: small}
	u.menu(engine, idxPath)
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
	if !u.confirm("Atenção aos save states", "Save states podem não funcionar entre núcleos ou versões diferentes. Os saves normais são mais portáveis. Continuar?") {
		return
	}
	u.message("Varredura", "Procurando saves novos ou modificados...")
	items, err := e.Scan(d, full)
	if err != nil {
		u.message("Não foi possível continuar", err.Error())
		return
	}
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
		results = append(results, e.Copy(c, d, action))
	}
	_ = syncer.SaveIndex(idxPath, e.Index)
	report, reportErr := syncer.WriteReport(filepath.Join(e.Config.Userdata, "system", "logs", "muos-save-importer"), d, results)
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
		u.screen(title, append(wrap(message, 67), items...), selected+len(wrap(message, 67)), "A: confirmar   B: cancelar")
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
	u.text(title, 28, 22, u.font, sdl.Color{R: 91, G: 192, B: 235, A: 255})
	y := 75
	for i, item := range items {
		color := sdl.Color{R: 220, G: 225, B: 230, A: 255}
		if i == selected {
			u.ren.SetDrawColor(35, 74, 105, 255)
			u.ren.FillRect(&sdl.Rect{X: 20, Y: int32(y - 5), W: 600, H: 34})
			color = sdl.Color{R: 255, G: 214, B: 102, A: 255}
		}
		for _, line := range wrap(item, 67) {
			u.text(line, 32, y, u.small, color)
			y += 24
		}
		y += 7
		if y > 420 {
			break
		}
	}
	u.text(footer, 20, 449, u.small, sdl.Color{R: 145, G: 155, B: 165, A: 255})
	u.ren.Present()
}
func (u *ui) message(title, msg string) {
	u.screen(title, wrap(msg, 67), -1, "A/B: voltar")
	for {
		key := waitKey()
		if key == "ok" || key == "back" {
			return
		}
	}
}

func (u *ui) text(text string, x, y int, font *ttf.Font, color sdl.Color) {
	surf, err := font.RenderUTF8Blended(text, color)
	if err != nil {
		return
	}
	defer surf.Free()
	tex, err := u.ren.CreateTextureFromSurface(surf)
	if err != nil {
		return
	}
	defer tex.Destroy()
	u.ren.Copy(tex, nil, &sdl.Rect{X: int32(x), Y: int32(y), W: surf.W, H: surf.H})
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
func findFont() string {
	for _, p := range []string{"/usr/share/fonts/dejavu/DejaVuSans.ttf", "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", "/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"
}
func envDefault(k, v string) string {
	if x := os.Getenv(k); x != "" {
		return x
	}
	return v
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); time.Sleep(3 * time.Second); os.Exit(1) }
