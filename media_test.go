package senpai

import (
	"bytes"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sr.ht/~delthas/senpai/ui"
)

type responseBody struct {
	io.Reader
	closed bool
}

func (b *responseBody) Close() error { b.closed = true; return nil }

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestUploadClosesEveryResponse(t *testing.T) {
	for _, status := range []int{http.StatusCreated, http.StatusRequestEntityTooLarge, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			body := &responseBody{Reader: strings.NewReader("reply")}
			old := http.DefaultTransport
			http.DefaultTransport = transportFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Body: body, Header: http.Header{"Location": {"https://files.example/file"}}, Request: req}, nil
			})
			defer func() { http.DefaultTransport = old }()
			app := &App{}
			_, err := app.upload("https://files.example", strings.NewReader("file"), -1, "", "")
			if (err == nil) != (status == http.StatusCreated) {
				t.Fatalf("status %d: %v", status, err)
			}
			if !body.closed {
				t.Fatal("response body was not closed")
			}
		})
	}
}

func TestImagePreviewResolvesRelativeURLs(t *testing.T) {
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/page/index", http.StatusFound)
		case "/page/index":
			w.Header().Set("Content-Type", "text/html")
			io.WriteString(w, `<meta property="og:image" content="../image.png"/>`)
		case "/image.png":
			w.Write(data.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	app := &App{win: &ui.UI{}}
	img, err := app.fetchImage(server.URL + "/start")
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 2 || img.Bounds().Dy() != 3 {
		t.Fatalf("wrong image: %v", img.Bounds())
	}
}

func TestImagePreviewRejectsErrorResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		if r.Method == "GET" {
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer server.Close()
	app := &App{win: &ui.UI{}}
	if _, err := app.fetchImage(server.URL); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("error response accepted: %v", err)
	}
}
