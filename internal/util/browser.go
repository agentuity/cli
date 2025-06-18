package util

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/tui"
	"github.com/pkg/browser"
)

var ErrTimeout = errors.New("timeout")

type browserCallback func(query url.Values) error

type BrowserFlowOptions struct {
	Logger      logger.Logger
	BaseUrl     string
	StartPath   string
	WaitMessage string
	AuthToken   string
	Query       map[string]string
	Callback    browserCallback
}

// BrowserFlow will open a browser and wait for the user to finish the flow.
// It will return an error if the flow times out with an ErrTimeout error.
// It will return an error if the callback fails or any other error occurs.
func BrowserFlow(opts BrowserFlowOptions) error {

	logger := opts.Logger

	u, err := url.Parse(opts.BaseUrl)
	if err != nil {
		return fmt.Errorf("error parsing url: %s. %w", opts.BaseUrl, err)
	}

	timer := time.NewTimer(time.Minute)
	defer timer.Stop()

	listener, err := net.Listen("tcp", "127.0.0.1:0") // MUST listen only on 127.0.0.1 so we don't open a unintended port to the public
	if err != nil {
		return fmt.Errorf("failed to listen on port: %w", err)
	}

	state := RandStringBytes(16) // generate a random state that will be used to verify the callback

	port := listener.Addr().(*net.TCPAddr).Port
	u.Path = opts.StartPath
	q := u.Query()
	q.Add("callback", fmt.Sprintf("http://127.0.0.1:%d/callback", port))
	q.Add("state", state)
	q.Add("from", "cli")
	if opts.AuthToken != "" {
		q.Add("token", opts.AuthToken)
	}
	for k, v := range opts.Query {
		q.Add(k, v)
	}
	u.RawQuery = q.Encode()

	srv := http.NewServeMux()
	errors := make(chan error, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		defer cancel()
		query := r.URL.Query()
		logger.Trace("callback received with query: %s", query.Encode())
		if query.Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			errors <- fmt.Errorf("state mismatch in response from application")
			return
		}
		if opts.Callback != nil {
			if err := opts.Callback(query); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				errors <- fmt.Errorf("callback failed: %w", err)
				return
			}
			opts.Callback = nil
		}
		callback := query.Get("callback")
		if callback != "" {
			cu, err := url.Parse(callback)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			q := cu.Query()
			q.Set("success", "true")
			q.Del("callback")
			cu.RawQuery = q.Encode()
			logger.Trace("redirecting to %s", cu.String())
			w.Header().Set("Location", cu.String())
			w.WriteHeader(302)
			return
		}
		w.WriteHeader(200)
	})

	server := &http.Server{Handler: srv}

	go func() {
		logger.Trace("listening on port %d", port)
		err = server.Serve(listener)
		if err != nil && err != http.ErrServerClosed {
			errors <- fmt.Errorf("failed to serve: %w", err)
		}
		logger.Trace("server closed")
	}()

	defer func() {
		time.Sleep(time.Millisecond * 250)
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*200)
		server.Shutdown(ctx)
		cancel()
	}()

	var skipOpen bool
	if runtime.GOOS == "linux" {
		// if we don't have a display, we can't open a browser most likely
		if _, ok := os.LookupEnv("DISPLAY"); !ok {
			skipOpen = true
		}
	}

	if !skipOpen {
		fmt.Println(tui.Title("Your browser has been opened to visit the URL:"))
	} else {
		// FIXME: this likely isn't going to work until they are on the same machine
		// we need a non-browser way to handle this via a code that you put in the app or something
		fmt.Println(tui.Title("Please visit the URL in your browser:"))
	}
	fmt.Println()
	url := u.String()
	if !skipOpen {
		fmt.Println(tui.Muted(url))
	} else {
		fmt.Println(tui.Link("%s", url))
	}
	fmt.Println()

	var returnErr error
	var wg sync.WaitGroup

	action := func() {
		defer wg.Done()
		var skip bool
		if runtime.GOOS == "linux" {
			if _, ok := os.LookupEnv("DISPLAY"); !ok {
				skip = true
			}
		}
		if !skip {
			if berr := browser.OpenURL(u.String()); berr != nil {
 returnErr = fmt.Errorf("failed to open browser: %w", berr)
				return
			}
		}
		logger.Trace("waiting for callback to http://127.0.0.1:%d", port)
		select {
		case e := <-errors:
			returnErr = e
			return
		case <-timer.C:
			returnErr = ErrTimeout
			return
		case <-ctx.Done():
			return
		}
	}

	wg.Add(1)

	tui.ShowSpinner("Waiting for response...", action)

	wg.Wait()

	return returnErr
}

// PromptBrowserOpen prompts the user to press Enter to open a browser to the given URL.
// It handles display detection on Linux and provides appropriate user feedback.
func PromptBrowserOpen(logger interface{ Error(string, ...interface{}) }, url string) {
	var skipOpen bool
	if runtime.GOOS == "linux" {
		// if we don't have a display, we can't open a browser most likely
		if _, ok := os.LookupEnv("DISPLAY"); !ok {
			skipOpen = true
		}
	}

	fmt.Println()
	if skipOpen {
		fmt.Print(tui.Secondary("Press Enter to continue, or Ctrl+C to skip: "))
	} else {
		fmt.Print(tui.Secondary("Press Enter to open browser, or Ctrl+C to skip: "))
	}
	
	reader := bufio.NewReader(os.Stdin)
	reader.ReadLine()

	if !skipOpen {
		if err := browser.OpenURL(url); err != nil {
			logger.Error("Failed to open browser: %v", err)
		} else {
			// Clear previous line and move cursor up to remove the "Press Enter..." prompt
			fmt.Print("\r\033[K\033[A\r\033[K")
			tui.ShowSuccess("Browser opened!")
			fmt.Println()
		}
	} else {
		// Clear the prompt and show the URL for manual opening (and the loading spinner)
		fmt.Print("\r\033[K")
		fmt.Println(tui.Muted("Please visit the URL manually:"))
		fmt.Println(tui.Link(url))
		fmt.Println()
	}
}
