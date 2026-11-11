package senpai

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"git.sr.ht/~delthas/senpai/ui"
	"github.com/delthas/go-libnp"
)

func commandDoNP(app *App, args []string) (err error) {
	song, err := getSong()
	if err != nil {
		return fmt.Errorf("failed detecting the song: %v", err)
	}
	if song == "" {
		return fmt.Errorf("no song was detected")
	}
	return commandDoMe(app, []string{fmt.Sprintf("np: %s", song)})
}

func commandDoUpload(app *App, args []string) (err error) {
	if app.cfg.Transient || !app.cfg.LocalIntegrations {
		return fmt.Errorf("usage of UPLOAD is disabled")
	}
	s := app.CurrentSession()
	if s == nil {
		return errOffline
	}
	upload := s.UploadURL()
	if upload == "" {
		return fmt.Errorf("file upload is not supported on this server; try using soju and enabling file upload")
	}

	if len(args) == 0 {
		rc, mimetype, err := readClipboard()
		if err != nil {
			return err
		}
		app.handleUpload(upload, rc, -1, "", mimetype, rc)
		return nil
	}

	path := args[0]
	if home, err := os.UserHomeDir(); err == nil && !filepath.IsAbs(path) {
		path = filepath.Join(home, path)
	}

	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("opening file: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening file: %v", err)
	}

	app.handleUpload(upload, f, fi.Size(), filepath.Base(path), "", f)
	return nil
}

func commandDoScreenshot(app *App, args []string) (err error) {
	if app.cfg.Transient || !app.cfg.LocalIntegrations {
		return fmt.Errorf("usage of SCREENSHOT is disabled")
	}
	return ui.Screenshot()
}

func getSong() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	info, err := libnp.GetInfo(ctx)
	if err != nil {
		return "", err
	}
	if info == nil {
		return "", nil
	}
	if info.Title == "" {
		return "", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "\x02%s\x02", info.Title)
	if len(info.Artists) > 0 && info.Artists[0] != "" {
		fmt.Fprintf(&sb, " by \x02%s\x02", info.Artists[0])
	}
	if info.Album != "" {
		fmt.Fprintf(&sb, " from \x02%s\x02", info.Album)
	}
	if u, err := url.Parse(info.URL); err == nil {
		switch u.Scheme {
		case "http", "https":
			fmt.Fprintf(&sb, " — %s", info.URL)
		}
	}
	return sb.String(), nil
}
