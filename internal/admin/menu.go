// Package admin is the hidden maintenance panel drawn on the TV.
//
// It opens with Ctrl+Alt+A on a keyboard or a key sequence on the remote
// (Heart, Up, Heart, Down) and is driven with the arrows, OK and Back. It
// gathers what used to be loose shortcuts and SSH commands: status, QR,
// pause, network mode, logs, photos, phone code, remote pairing, language
// and restart/reboot/power off/console. Dangerous items ask for confirmation.
//
// menu.go is the pure state machine (tested), render.go draws it, admin.go
// wires it to the player, the system and the screen.
package admin

// Key is a key name as sent to /api/tv/key.
type Key string

const (
	KUp        Key = "up"
	KDown      Key = "down"
	KLeft      Key = "left"
	KRight     Key = "right"
	KOK        Key = "ok"
	KBack      Key = "back"
	KHome      Key = "home"
	KPlayPause Key = "playpause"
	KHeart     Key = "heart"
	KPower     Key = "power"
	KVolUp     Key = "volup"
	KVolDown   Key = "voldown"
	KAdmin     Key = "admin" // keyboard shortcut: open the panel directly
)

// ParseKey accepts the names above plus keyboard aliases.
func ParseKey(s string) (Key, bool) {
	switch s {
	case "enter":
		return KOK, true
	case "esc", "escape":
		return KBack, true
	}
	switch k := Key(s); k {
	case KUp, KDown, KLeft, KRight, KOK, KBack, KHome, KPlayPause, KHeart, KPower, KVolUp, KVolDown, KAdmin:
		return k, true
	}
	return "", false
}

// Item is one menu line. Exactly one of Action or Sub is set.
type Item struct {
	Label   string // i18n key
	Action  string // what to run (see admin.go)
	Confirm bool   // ask "are you sure?" first
	Sub     []Item
}

// Menu is the whole tree. Actions are named; admin.go runs them.
func Menu() []Item {
	return []Item{
		{Label: "adm.qr", Action: "qr"},
		{Label: "adm.pause", Action: "pause"},
		{Label: "adm.network", Sub: []Item{
			{Label: "adm.net_home", Action: "sys:wifi-home", Confirm: true},
			{Label: "adm.net_tele", Action: "sys:wifi-tele", Confirm: true},
			{Label: "adm.log_wifi", Action: "log:wifi"},
		}},
		{Label: "adm.logs", Sub: []Item{
			{Label: "adm.log_tvcast", Action: "log:tvcast"},
			{Label: "adm.log_wifi", Action: "log:wifi"},
			{Label: "adm.log_remote", Action: "log:remote"},
			{Label: "adm.log_admin", Action: "log:admin"},
			{Label: "adm.log_kernel", Action: "log:kernel"},
		}},
		{Label: "adm.photos", Sub: []Item{
			{Label: "adm.photos_delete", Action: "photos-delete", Confirm: true},
		}},
		{Label: "adm.new_code", Action: "sys:new-code", Confirm: true},
		{Label: "adm.remote_pair", Action: "sys:remote-pair", Confirm: true},
		{Label: "adm.language", Sub: []Item{
			{Label: "adm.lang_es", Action: "sys:lang-es", Confirm: true},
			{Label: "adm.lang_en", Action: "sys:lang-en", Confirm: true},
		}},
		{Label: "adm.system", Sub: []Item{
			{Label: "adm.restart", Action: "sys:restart-tvcast", Confirm: true},
			{Label: "adm.reboot", Action: "sys:reboot", Confirm: true},
			{Label: "adm.poweroff", Action: "sys:poweroff", Confirm: true},
			{Label: "adm.console", Action: "sys:console", Confirm: true},
		}},
		{Label: "adm.close", Action: "close"},
	}
}

type frame struct {
	title  string // i18n key of the submenu ("" = main menu)
	items  []Item
	cursor int
}

// State is the panel's navigation state. It is not safe for concurrent use:
// the Controller serialises access.
type State struct {
	stack   []frame
	confirm *Item // pending confirmation
	yes     bool  // cursor on "Yes" in the confirmation
	log     *logView
	Message string // last result, shown at the bottom (already translated)
}

type logView struct {
	title string
	lines []string
}

func newState() *State {
	return &State{stack: []frame{{items: Menu()}}}
}

// Key handles one key. It returns the action to run, if any. close=true
// means the panel must close.
func (s *State) Key(k Key) (action string, close bool) {
	if s.log != nil { // log screen: any of Back/OK/Home returns to the menu
		if k == KBack || k == KOK || k == KHome {
			s.log = nil
		}
		return "", false
	}
	if s.confirm != nil {
		switch k {
		case KLeft, KRight, KUp, KDown:
			s.yes = !s.yes
		case KOK:
			it := s.confirm
			s.confirm = nil
			if s.yes {
				return it.Action, false
			}
		case KBack, KHome:
			s.confirm = nil
		}
		return "", false
	}
	f := &s.stack[len(s.stack)-1]
	switch k {
	case KUp:
		f.cursor = (f.cursor - 1 + len(f.items)) % len(f.items)
	case KDown:
		f.cursor = (f.cursor + 1) % len(f.items)
	case KHome:
		return "", true
	case KBack, KLeft:
		if len(s.stack) == 1 {
			return "", k == KBack
		}
		s.stack = s.stack[:len(s.stack)-1]
	case KOK, KRight:
		it := f.items[f.cursor]
		switch {
		case it.Sub != nil:
			s.stack = append(s.stack, frame{title: it.Label, items: it.Sub})
		case k == KRight:
			// Right only enters submenus; it never runs an action.
		case it.Action == "close":
			return "", true
		case it.Confirm:
			s.confirm, s.yes = &f.items[f.cursor], false // default: No
		default:
			return it.Action, false
		}
	}
	return "", false
}

// ShowLog switches to a log screen.
func (s *State) ShowLog(title string, lines []string) { s.log = &logView{title, lines} }

// View is what render.go draws.
type View struct {
	Title    string   // i18n key ("" = main)
	Items    []string // i18n keys
	Cursor   int
	Confirm  string // i18n key of the item being confirmed ("" = none)
	Yes      bool
	LogTitle string
	Log      []string // log lines (nil = not on a log screen)
	Message  string
}

func (s *State) View() View {
	f := s.stack[len(s.stack)-1]
	v := View{Title: f.title, Cursor: f.cursor, Message: s.Message, Yes: s.yes}
	for _, it := range f.items {
		v.Items = append(v.Items, it.Label)
	}
	if s.confirm != nil {
		v.Confirm = s.confirm.Label
	}
	if s.log != nil {
		v.LogTitle, v.Log = s.log.title, s.log.lines
		if v.Log == nil {
			v.Log = []string{}
		}
	}
	return v
}
