package senpai

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"git.sr.ht/~delthas/senpai/irc"
	"git.sr.ht/~delthas/senpai/ui"
	"go.rockorager.dev/vaxis"
)

const eventChanSize = 1024

type App struct {
	win              *ui.UI
	sessions         map[string]*irc.Session // map of network IDs to their current session
	pasting          bool
	pastingInputOnly bool // true is pasting started when the editor input was empty

	// events MUST NOT be posted to directly; instead, use App.postEvent.
	events chan event

	cfg        Config
	highlights []string
	shortcuts  map[keyMatch][]string

	lastQuery     string
	lastQueryNet  string
	messageBounds map[boundKey]bound
	lastNetID     string
	lastBuffer    string
	bufferStore   bufferStore

	monitor map[string]map[string]struct{} // set of targets we want to monitor per netID, best-effort. netID->target->{}

	networkLock sync.RWMutex                 // locks networks
	networks    map[string]map[string]string // set of network IDs to attributes we want to connect to; to be locked with networkLock

	pendingCompletions    map[string][]pendingCompletion
	pendingCompletionsOff int

	lastMessageTime time.Time
	lastCloseTime   time.Time

	lastConfirm string

	imageLoading bool
	imageOverlay bool

	uploadingProgress *float64

	shownBouncerNotice bool
	shownPasteHint     bool

	closing     atomic.Bool
	stop        chan struct{}
	eventMu     sync.RWMutex
	runMu       sync.Mutex
	running     bool
	cleanupOnce sync.Once

	harper *harperState
}

func NewApp(cfg Config) (app *App, err error) {
	if cfg.Addr == "" {
		return nil, errors.New("address is required")
	}
	if cfg.Nick == "" {
		return nil, errors.New("nick is required")
	}
	if cfg.User == "" {
		cfg.User = cfg.Nick
	}
	if cfg.Real == "" {
		cfg.Real = cfg.Nick
	}

	app = &App{
		networks: map[string]map[string]string{
			"": {}, // add the master network by default
		},
		pendingCompletions: make(map[string][]pendingCompletion),
		sessions:           map[string]*irc.Session{},
		events:             make(chan event, eventChanSize),
		stop:               make(chan struct{}),
		cfg:                cfg,
		shortcuts:          make(map[keyMatch][]string),
		messageBounds:      map[boundKey]bound{},
		monitor:            make(map[string]map[string]struct{}),
	}
	for _, m := range []map[string][]string{defaultCommands, app.cfg.Shortcuts} {
		for name, actions := range m {
			k := keyNameMatch(name)
			if k == nil {
				return nil, fmt.Errorf("unknown key name: %v", name)
			}
			app.shortcuts[*k] = actions
		}
	}

	if cfg.Highlights != nil {
		app.highlights = make([]string, len(cfg.Highlights))
		for i := range app.highlights {
			app.highlights[i] = strings.ToLower(cfg.Highlights[i])
		}
	}

	mouse := cfg.Mouse

	app.win, app.cfg.Colors, err = ui.New(ui.Config{
		NickColWidth:     cfg.NickColWidth,
		ChanColWidth:     cfg.ChanColWidth,
		ChanColEnabled:   cfg.ChanColEnabled,
		MemberColWidth:   cfg.MemberColWidth,
		MemberColEnabled: cfg.MemberColEnabled,
		TextMaxWidth:     cfg.TextMaxWidth,
		AutoComplete: func(cursorIdx int, text []rune) []ui.Completion {
			return app.completions(cursorIdx, text)
		},
		Mouse: mouse,
		MergeLine: func(former *ui.Line, addition ui.Line) {
			app.mergeLine(former, addition)
		},
		Colors:            cfg.Colors,
		LocalIntegrations: cfg.LocalIntegrations,
		WithTTY:           cfg.WithTTY,
		WithConsole:       cfg.WithConsole,
	})
	if err != nil {
		return
	}

	ui.DBusStart(func(ev any) {
		app.postEvent(event{
			src:     "*",
			content: ev,
		})
	})
	app.win.SetPrompt(ui.Styled(">", vaxis.Style{
		Foreground: app.cfg.Colors.Prompt,
	}),
	)

	app.initWindow()

	return
}

// Close requests shutdown and is safe to call repeatedly from another goroutine.
// Run owns session and subprocess cleanup until its event loop has stopped.
func (app *App) Close() {
	app.runMu.Lock()
	defer app.runMu.Unlock()
	if app.closing.CompareAndSwap(false, true) {
		app.win.Exit()
		close(app.stop)
	}
	if !app.running {
		app.cleanup()
	}
}

func (app *App) cleanup() {
	app.cleanupOnce.Do(func() {
		// Wait for canceled producers before draining unhandled session arrivals.
		app.eventMu.Lock()
		defer app.eventMu.Unlock()
		for {
			select {
			case ev := <-app.events:
				if session, ok := ev.content.(*irc.Session); ok {
					session.Close()
				}
			default:
				for _, session := range app.sessions {
					session.Close()
				}
				ui.DBusStop()
				app.harperClose()
				app.win.Close()
				return
			}
		}
	})
}

func (app *App) SwitchToBuffer(netID, buffer string) {
	app.lastNetID = netID
	app.lastBuffer = buffer
}

func (app *App) Run() {
	app.runMu.Lock()
	if app.closing.Load() || app.running {
		app.runMu.Unlock()
		return
	}
	app.running = true
	app.runMu.Unlock()
	defer func() {
		app.Close()
		app.cleanup()
		app.runMu.Lock()
		app.running = false
		app.runMu.Unlock()
	}()

	if app.lastCloseTime.IsZero() {
		app.lastCloseTime = time.Now()
	}
	app.harperInit()
	go app.uiLoop()
	go app.ircLoop("")
	app.eventLoop()
}

func (app *App) CurrentSession() *irc.Session {
	netID, _ := app.win.CurrentBuffer()
	return app.sessions[netID]
}

func (app *App) CurrentBuffer() (netID, buffer string) {
	return app.win.CurrentBuffer()
}

func (app *App) LastMessageTime() time.Time {
	return app.lastMessageTime
}

func (app *App) SetLastClose(t time.Time) {
	app.lastCloseTime = t
}
