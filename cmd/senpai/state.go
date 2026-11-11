package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"git.sr.ht/~delthas/senpai"
)

func cachePath() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		panic(err)
	}
	cache := path.Join(cacheDir, "senpai")
	err = os.MkdirAll(cache, 0755)
	if err != nil {
		panic(err)
	}
	return cache
}

func configPathHash(configPath string) string {
	abs, err := filepath.Abs(configPath)
	if err != nil {
		abs = configPath
	}
	h := fnv.New32a()
	h.Write([]byte(abs))
	return hex.EncodeToString(h.Sum(nil))
}

func lastBufferPath(hash string) string {
	name := "lastbuffer.txt"
	if hash != "" {
		name = "lastbuffer-" + hash + ".txt"
	}
	return path.Join(cachePath(), name)
}

func buffersPath(hash string) string {
	return path.Join(cachePath(), "buffers-"+hash+".json")
}

func getBuffers(hash string) []senpai.BufferState {
	p := buffersPath(hash)
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	var states []senpai.BufferState
	if err == nil {
		err = json.Unmarshal(data, &states)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read conversation list at %q: %s\n", p, err)
		return nil
	}
	return states
}

func writeBuffers(states []senpai.BufferState, hash string) {
	p := buffersPath(hash)
	if err := saveBuffers(p, states); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write conversation list at %q: %s\n", p, err)
	}
}

func saveBuffers(p string, states []senpai.BufferState) error {
	data, err := json.Marshal(states)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".buffers-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), p)
}

func getLastBuffer(hash string) (netID, buffer string) {
	p := lastBufferPath(hash)
	buf, err := os.ReadFile(p)
	if err != nil && hash != "" {
		buf, err = os.ReadFile(lastBufferPath(""))
		if err != nil {
			return "", ""
		}
	} else if err != nil {
		return "", ""
	}

	// The empty network ID denotes the primary connection, so its leading
	// separator is meaningful. A network's home buffer has an empty target.
	fields := strings.SplitN(strings.TrimRight(string(buf), "\r\n"), " ", 2)
	if len(fields) < 2 {
		return "", ""
	}

	return fields[0], fields[1]
}

func writeLastBuffer(app *senpai.App, hash string) {
	p := lastBufferPath(hash)
	lastNetID, lastBuffer := app.CurrentBuffer()
	err := os.WriteFile(p, []byte(fmt.Sprintf("%s %s", lastNetID, lastBuffer)), 0666)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to write last buffer at %q: %s\n", p, err)
	}
}

func lastStampPath(hash string) string {
	name := "laststamp.txt"
	if hash != "" {
		name = "laststamp-" + hash + ".txt"
	}
	return path.Join(cachePath(), name)
}

func getLastStamp(hash string) time.Time {
	p := lastStampPath(hash)
	buf, err := os.ReadFile(p)
	if err != nil && hash != "" {
		buf, err = os.ReadFile(lastStampPath(""))
		if err != nil {
			return time.Time{}
		}
	} else if err != nil {
		return time.Time{}
	}

	stamp := strings.TrimSpace(string(buf))
	t, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return time.Time{}
	}
	return t
}

func writeLastStamp(app *senpai.App, hash string) {
	p := lastStampPath(hash)
	last := app.LastMessageTime()
	if last.IsZero() {
		return
	}
	err := os.WriteFile(p, []byte(last.UTC().Format(time.RFC3339Nano)), 0666)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to write last stamp at %q: %s\n", p, err)
	}
}
