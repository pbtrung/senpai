package ui

import (
	"context"
	"os"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"go.rockorager.dev/vaxis"
)

type Config struct {
	NickColWidth      int
	ChanColWidth      int
	ChanColEnabled    bool
	MemberColWidth    int
	MemberColEnabled  bool
	TextMaxWidth      int
	AutoComplete      func(cursorIdx int, text []rune) []Completion
	Mouse             bool
	MergeLine         func(former *Line, addition Line)
	Colors            ConfigColors
	LocalIntegrations bool
	WithConsole       vaxis.Console
	WithTTY           string
}

type ConfigColors struct {
	Gray   vaxis.Color
	Status vaxis.Color
	Prompt vaxis.Color
	Unread vaxis.Color
	Nicks  ColorScheme
}

type Vaxis struct {
	*vaxis.Vaxis
	window vaxis.Window
	xPixel int
	yPixel int
}

type NotifyEvent struct {
	NetID  string
	Buffer string
}

type ScreenshotEvent struct {
	Path string
}

type clickEvent struct {
	xb    int
	xe    int
	y     int
	event interface{}
}

type UI struct {
	vx     *Vaxis
	Events chan any
	exit   atomic.Value // bool
	config Config

	bs          BufferList
	e           Editor
	prompt      StyledString
	status      string
	title       string
	overlayHint string

	channelOffset int
	memberClicked int
	memberOffset  int

	channelWidth int
	memberWidth  int

	channelColClicked bool
	memberColClicked  bool

	clickEvents []clickEvent

	image vaxis.Image

	mouseLinks bool

	colorThemeMode vaxis.ColorThemeMode
}

func New(config Config) (ui *UI, colors ConfigColors, err error) {
	ui = &UI{
		config:        config,
		clickEvents:   make([]clickEvent, 0, 128),
		memberClicked: -1,
	}
	if config.ChanColEnabled {
		ui.channelWidth = config.ChanColWidth
	}
	if config.MemberColEnabled {
		ui.memberWidth = config.MemberColWidth
	}

	if os.Getenv("TERM_PROGRAM") == "Apple_Terminal" {
		if os.Getenv("VAXIS_FORCE_LEGACY_SGR") == "" {
			os.Setenv("VAXIS_FORCE_LEGACY_SGR", "true")
		}
	}
	if runtime.GOOS == "windows" {
		// Work around broken RGB colors on Windows Terminal.
		// Sadly the Windows Terminal does not support TerminalID, so we rely on GOOS here.
		if os.Getenv("COLORTERM") == "" {
			os.Setenv("COLORTERM", "truecolor")
		}
		if os.Getenv("VAXIS_FORCE_LEGACY_SGR") == "" {
			os.Setenv("VAXIS_FORCE_LEGACY_SGR", "true")
		}
	}
	var vx *vaxis.Vaxis
	opts := vaxis.Options{
		DisableMouse: !config.Mouse,
		CSIuBitMask:  vaxis.CSIuDisambiguate | vaxis.CSIuReportEvents | vaxis.CSIuAlternateKeys | vaxis.CSIuAssociatedText,
		WithTTY:      config.WithTTY,
		WithConsole:  config.WithConsole,
	}
	vx, err = vaxis.New(opts)
	if err != nil {
		return
	}
	if strings.HasPrefix(vx.TerminalID(), "iTerm2") {
		// see: https://gitlab.com/gnachman/iterm2/-/issues/12177
		opts.CSIuBitMask = vaxis.CSIuDisambiguate | vaxis.CSIuReportEvents | vaxis.CSIuAlternateKeys
		vx.Close()
		vx, err = vaxis.New(opts)
		if err != nil {
			return
		}
	}
	ui.vx = &Vaxis{
		Vaxis:  vx,
		window: vx.Window(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	bg := ui.vx.QueryBackgroundContext(ctx).Params()
	if len(bg) == 3 {
		if (int(bg[0])+int(bg[1])+int(bg[2]))/3 > 127 {
			ui.colorThemeMode = vaxis.LightMode
		} else {
			ui.colorThemeMode = vaxis.DarkMode
		}
	} else {
		ui.colorThemeMode = vaxis.DarkMode
	}

	ui.config.Colors.Gray = vaxis.IndexColor(8)
	black := ui.vx.QueryColorContext(ctx, vaxis.IndexColor(uint8(0))).Params()
	gray := ui.vx.QueryColorContext(ctx, vaxis.IndexColor(uint8(8))).Params()
	white := ui.vx.QueryColorContext(ctx, vaxis.IndexColor(uint8(15))).Params()
	fg := ui.vx.QueryForegroundContext(ctx).Params()
	if len(bg) == 3 && len(fg) == 3 && ui.vx.CanRGB() {
		// Interpolate gray from fg and bg to make it slightly more readable against the background than default gray.
		p := make([]uint8, 3)
		for i := range p {
			p[i] = uint8((int(bg[i])*3 + int(fg[i])*2) / 5)
		}
		ui.config.Colors.Gray = vaxis.RGBColor(p[0], p[1], p[2])
	} else if len(bg) == 3 && len(gray) == 3 && reflect.DeepEqual(bg, gray) {
		// RGB is not supported.
		// Color theme with background set to gray: gray would be invisible.
		if len(black) == 3 && !reflect.DeepEqual(bg, black) {
			// Black is distinct from background: use that as gray.
			ui.config.Colors.Gray = vaxis.IndexColor(0)
		} else if len(white) == 3 && !reflect.DeepEqual(bg, white) {
			// White is distinct from background: use that as white.
			ui.config.Colors.Gray = vaxis.IndexColor(15)
		} else {
			// Black == gray == background == white. Give up.
			ui.config.Colors.Gray = ColorDefault
		}
	}
	if ui.config.Colors.Status == ColorDefault {
		ui.config.Colors.Status = ui.config.Colors.Gray
	}

	ui.vx.SetTitle("senpai")
	ui.vx.SetAppID("senpai")

	_, h := ui.vx.window.Size()
	ui.vx.window.Clear()
	ui.vx.ShowCursor(0, h-2, vaxis.CursorBeam)

	ui.mouseLinks = ui.config.LocalIntegrations && strings.HasPrefix(ui.vx.TerminalID(), "foot")

	ui.exit.Store(false)

	ui.Events = make(chan any, 128)
	go func() {
		for !ui.ShouldExit() {
			ev := ui.vx.PollEvent()
			if _, ok := ev.(vaxis.QuitEvent); ok {
				ui.Exit()
				break
			}
			ui.Events <- ev
		}
		close(ui.Events)
	}()

	ui.bs = NewBufferList(ui)
	ui.e = NewEditor(ui)
	ui.Relayout()

	return ui, ui.config.Colors, nil
}

func (ui *UI) ShouldExit() bool {
	return ui.exit.Load().(bool)
}

func (ui *UI) Exit() {
	ui.exit.Store(true)
}

func (ui *UI) Close() {
	ui.vx.Refresh() // TODO is this needed?
	ui.vx.Close()
}

func (ui *UI) SetStatus(status string) {
	ui.status = status
}

func (ui *UI) SetPrompt(prompt StyledString) {
	ui.prompt = prompt
}

func (ui *UI) SetTitle(title string) {
	if ui.title == title {
		return
	}
	ui.title = title
	ui.vx.SetTitle(title)
}

func (ui *UI) SetMouseShape(shape vaxis.MouseShape) {
	ui.vx.SetMouseShape(shape)
}

func (ui *UI) SetColorTheme(mode vaxis.ColorThemeMode) {
	ui.colorThemeMode = mode
}

func (ui *UI) Resize(size vaxis.Resize) {
	ui.vx.Resize(size)
	ui.vx.xPixel = size.XPixel
	ui.vx.yPixel = size.YPixel
	ui.Relayout()
}

func (ui *UI) Relayout() {
	ui.vx.window = ui.vx.Window() // Refresh window size
	w, h := ui.vx.window.Size()
	innerWidth := w - 9 - ui.channelWidth - ui.config.NickColWidth - ui.memberWidth
	if innerWidth <= 0 {
		innerWidth = 1 // will break display somewhat, but this is an edge case
	}
	ui.e.Resize(innerWidth)
	textWidth := innerWidth
	if ui.config.TextMaxWidth > 0 && ui.config.TextMaxWidth < textWidth {
		textWidth = ui.config.TextMaxWidth
	}
	if ui.channelWidth == 0 {
		ui.bs.ResizeTimeline(innerWidth, h-3, textWidth)
	} else {
		ui.bs.ResizeTimeline(innerWidth, h-2, textWidth)
	}
	ui.ScrollToBuffer()
	if ui.image != nil {
		ui.image.Resize(w, h)
	}
	ui.vx.Refresh()
}

func (ui *UI) Size() (int, int) {
	return ui.vx.window.Size()
}

func (ui *UI) Beep() {
	ui.vx.Bell()
}

func (ui *UI) Notify(title string, body string) {
	ui.vx.Notify(title, body)
}

func (ui *UI) Highlights() int {
	return ui.bs.Highlights()
}
