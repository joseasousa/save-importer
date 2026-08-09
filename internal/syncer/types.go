package syncer

import "time"

type Direction string

const (
	MuOSToKnulli Direction = "muos-to-knulli"
	KnulliToMuOS Direction = "knulli-to-muos"
)

type Kind string

const (
	Save  Kind = "save"
	State Kind = "state"
)

type ConflictAction string

const (
	KeepDestination ConflictAction = "keep"
	Replace         ConflictAction = "replace"
	PreserveBoth    ConflictAction = "preserve-both"
)

type Config struct {
	Version         int                 `json:"version"`
	Userdata        string              `json:"userdata"`
	SystemAliases   map[string][]string `json:"system_aliases"`
	SaveExtensions  []string            `json:"save_extensions"`
	StateExtensions []string            `json:"state_extensions"`
}

type Candidate struct {
	Source      string    `json:"source"`
	Destination string    `json:"destination"`
	Relative    string    `json:"relative"`
	System      string    `json:"system"`
	Kind        Kind      `json:"kind"`
	Size        int64     `json:"size"`
	Modified    time.Time `json:"modified"`
	Hash        string    `json:"hash"`
	Conflict    bool      `json:"conflict"`
	Ambiguous   bool      `json:"ambiguous"`
	Options     []string  `json:"options,omitempty"`
}

type Index struct {
	Version int                    `json:"version"`
	Files   map[string]IndexRecord `json:"files"`
}

type IndexRecord struct {
	Size       int64     `json:"size"`
	ModifiedNS int64     `json:"modified_ns"`
	Hash       string    `json:"sha256"`
	CopiedAt   time.Time `json:"copied_at"`
}

type Result struct {
	Source      string
	Destination string
	Action      ConflictAction
	Bytes       int64
	Hash        string
	Err         error
}

func DefaultConfig(userdata string) Config {
	return Config{Version: 1, Userdata: userdata,
		SystemAliases: map[string][]string{
			"gb": {"gambatte", "game boy", "gb"}, "gbc": {"game boy color", "gbc"},
			"gba": {"mgba", "gpsp", "game boy advance", "gba"}, "nes": {"fceumm", "nestopia", "nes"},
			"snes":      {"snes9x", "supafaust", "super nintendo", "sfc", "snes"},
			"megadrive": {"genesis plus gx", "picodrive", "genesis", "megadrive", "md"},
			"psx":       {"pcsx", "playstation", "ps1", "psx"}, "n64": {"mupen64", "parallel", "n64"},
			"nds": {"drastic", "melonds", "nintendo ds", "nds"}, "psp": {"ppsspp", "psp"},
			"dreamcast": {"flycast", "dreamcast", "dc"}, "arcade": {"fbneo", "mame", "arcade"},
		},
		SaveExtensions:  []string{".srm", ".sav", ".eep", ".fla", ".rtc", ".nv", ".mcr", ".mpk"},
		StateExtensions: []string{".state", ".state1", ".state2", ".state3", ".state4", ".state5", ".state6", ".state7", ".state8", ".state9", ".st0", ".st1", ".st2"},
	}
}
