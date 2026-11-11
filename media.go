package senpai

import (
	"fmt"
	"html"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"git.sr.ht/~delthas/senpai/events"
	"go.rockorager.dev/vaxis"
)

var patternOpenGraphImage = regexp.MustCompile(`<meta property="og:image" content="(.*?)"/?>`)

var patternOpenGraphVideo = regexp.MustCompile(`<meta property="og:video"`)

func (app *App) fetchImage(link string) (image.Image, error) {
	userAgent := "senpai"
	if v, ok := BuildVersion(); ok {
		userAgent = "senpai/" + v
	}

	if u, err := url.Parse(link); err == nil {
		changed := true
		switch u.Host {
		case "twitter.com", "x.com":
			u.Host = "fixupx.com"
		default:
			changed = false
		}
		if changed {
			link = u.String()
		}
	}

	cHead := http.Client{
		Timeout: 1500 * time.Millisecond,
	}
	req, err := http.NewRequest("HEAD", link, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	res, err := cHead.Do(req)
	if err != nil {
		return nil, err
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}
	contentType, _, err := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if err != nil {
		return nil, fmt.Errorf("unexpected content type: %v", res.Header.Get("Content-Type"))
	}
	var isHTML bool
	switch contentType {
	case "image/gif", "image/jpeg", "image/png": // Actual image, fetch
	case "text/html": // Might have an opengraph image, try fetching
		isHTML = true
	default:
		return nil, fmt.Errorf("unexpected content type: %v", contentType)
	}
	if isHTML {
		req, err := http.NewRequest("GET", link, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgent)
		var previewSize int64 = 10 * 1024
		if res.Header.Get("Accept-Ranges") == "bytes" {
			req.Header.Set("Range", fmt.Sprintf("bytes=0-%v", previewSize))
		}
		res, err = cHead.Do(req)
		if err != nil {
			return nil, err
		}
		if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusPartialContent {
			res.Body.Close()
			return nil, fmt.Errorf("unexpected status code: %d", res.StatusCode)
		}
		b, err := io.ReadAll(io.LimitReader(res.Body, previewSize))
		res.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("unexpected read error: %v", err)
		}
		if patternOpenGraphVideo.Match(b) {
			// Do not display image (previews) of video objects
			return nil, fmt.Errorf("video embed found")
		}
		m := patternOpenGraphImage.FindSubmatch(b)
		if len(m) < 2 {
			return nil, fmt.Errorf("image embed not found")
		}
		embed, err := url.Parse(html.UnescapeString(string(m[1])))
		if err != nil {
			return nil, err
		}
		link = res.Request.URL.ResolveReference(embed).String()
	}
	cGet := http.Client{
		Timeout: 5 * time.Second,
	}
	req, err = http.NewRequest("GET", link, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	res, err = cGet.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}
	img, _, err := app.win.DecodeImage(res.Body)
	if err != nil {
		return nil, err
	}
	return img, nil
}

func (app *App) handleLinkEvent(ev *events.EventClickLink) {
	open := func() {
		if strings.HasPrefix(ev.Link, "-") {
			// Avoid injection of parameters.
			// Sadly xdg-open does not support "--"...
			return
		}
		cmd := exec.Command("xdg-open", ev.Link)
		cmd.Run()
	}

	if ev.Event.Modifiers == vaxis.ModCtrl {
		if ev.Mouse {
			// Only react to Ctrl+Click when mouse links are enabled.

			// Explicit external link open requested with Ctrl+Click:
			// just run xdg-open.
			go open()
		}
		return
	}

	// Regular link open requested:
	// Try fetching as an image and displaying a preview;
	// fall back to xdg-open if mouse links are enabled.

	app.imageLoading = true
	go func() {
		img, err := app.fetchImage(ev.Link)
		if err != nil {
			app.postEvent(event{
				src: "*",
				content: &events.EventImageLoaded{
					Image: nil,
				},
			})
			if ev.Mouse {
				open()
			}
		} else {
			app.postEvent(event{
				src: "*",
				content: &events.EventImageLoaded{
					Image: img,
				},
			})
		}
	}()
}

func (app *App) upload(url string, r io.Reader, size int64, filename, mimetype string) (string, error) {
	c := http.Client{
		Timeout: 30 * time.Second,
	}
	rp := ReadProgress{
		Reader: r,
		period: 250 * time.Millisecond,
		f: func(n int64) {
			if size <= 0 {
				return
			}
			app.postEvent(event{
				src: "*",
				content: &events.EventFileUpload{
					Progress: float64(n) / float64(size),
				},
			})
		},
	}
	req, err := http.NewRequest("POST", url, &rp)
	if err != nil {
		return "", fmt.Errorf("creating upload request: %v", err)
	}
	if app.cfg.Password != nil {
		req.SetBasicAuth(app.cfg.User, *app.cfg.Password)
	}
	if size >= 0 {
		req.ContentLength = size
	}
	if filename != "" {
		req.Header.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
			"filename": filename,
		}))
	}
	if mimetype != "" {
		req.Header.Set("Content-Type", mimetype)
	}
	res, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("uploading: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusRequestEntityTooLarge {
		var maxSize int64
		for _, entry := range strings.Split(res.Header.Get("Upload-Limit"), ",") {
			entry = strings.TrimSpace(entry)
			key, value, ok := strings.Cut(entry, "=")
			if !ok || key != "maxsize" {
				continue
			}
			if v, err := strconv.ParseInt(value, 10, 64); err == nil && v > 0 {
				maxSize = v
			}
		}
		if maxSize > 0 && size >= 0 {
			return "", fmt.Errorf("uploading: file too large: maximum %v per file (file was %v)", formatSize(maxSize), formatSize(size))
		} else if maxSize > 0 {
			return "", fmt.Errorf("uploading: file too large: maximum %v per file", formatSize(maxSize))
		} else {
			return "", fmt.Errorf("uploading: file too large")
		}
	}
	if res.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("uploading: unexpected status code: %d", res.StatusCode)
	}
	location, err := res.Location()
	if err != nil {
		return "", fmt.Errorf("uploading: reading file URL: %v", err)
	}
	return location.String(), nil
}

func (app *App) handleUpload(url string, r io.Reader, size int64, filename, mimetype string, closer io.Closer) {
	var progress float64 = 0
	app.uploadingProgress = &progress
	go func() {
		if closer != nil {
			defer closer.Close()
		}
		location, err := app.upload(url, r, size, filename, mimetype)
		if err != nil {
			app.postEvent(event{
				src: "*",
				content: &events.EventFileUpload{
					Error: err.Error(),
				},
			})
		} else {
			app.postEvent(event{
				src: "*",
				content: &events.EventFileUpload{
					Location: location,
				},
			})
		}
	}()
}

func formatSize(v int64) string {
	suffixes := []string{"B", "kB", "MB", "GB"}
	for i, suffix := range suffixes {
		if v < 1024 || i == len(suffixes)-1 {
			return fmt.Sprintf("%v%v", v, suffix)
		}
		v /= 1024
	}
	panic("unreachable")
}

func dropBackslash(s string) string {
	// Naive implementation, just enough to unescape file paths escaped by some terminals.
	var sb strings.Builder
	esc := false
	for _, r := range s {
		if !esc && r == '\\' {
			esc = true
			continue
		}
		sb.WriteRune(r)
		esc = false
	}
	return sb.String()
}

type ReadProgress struct {
	io.Reader
	period time.Duration
	f      func(int64)

	n    int64
	last time.Time
}

func (r *ReadProgress) Read(buf []byte) (int, error) {
	n, err := r.Reader.Read(buf)
	r.n += int64(n)
	now := time.Now()
	if now.Sub(r.last) > r.period {
		r.last = now
		r.f(r.n)
	}
	return n, err
}
