package util

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/leaanthony/spinner"
	"github.com/pkg/browser"
	"github.com/shopmonkeyus/go-common/logger"
)

var ErrTimeout = errors.New("timeout")

type browserCallback func(query url.Values) error

type BrowserFlowOptions struct {
	Logger      logger.Logger
	BaseUrl     string
	StartPath   string
	SuccessPath string
	WaitMessage string
	AuthToken   string
	Query       map[string]string
	Callback    browserCallback
}

func BrowserFlow(opts BrowserFlowOptions) error {

	logger := opts.Logger

	u, err := url.Parse(opts.BaseUrl)
	if err != nil {
		return fmt.Errorf("error parsing url: %s. %w", opts.BaseUrl, err)
	}

	timer := time.NewTimer(time.Minute)
	defer timer.Stop()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
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
		}
		u.Path = opts.SuccessPath
		u.RawQuery = ""
		w.Header().Set("Location", u.String())
		w.WriteHeader(302)
	})

	server := &http.Server{Handler: srv}

	go func() {
		logger.Trace("listening on port %d", port)
		err = server.Serve(listener)
		if err != nil && err != http.ErrServerClosed {
			errors <- fmt.Errorf("failed to serve: %w", err)
		}
	}()

	logger.Trace("opening browser to %s", u.String())
	if err := browser.OpenURL(u.String()); err != nil {
		return fmt.Errorf("failed to open browser: %w", err)
	}
	logger.Trace("waiting for callback to http://127.0.0.1:%d/callback?state=%s&token=", port, state)

	defer server.Shutdown(context.Background())

	var s *spinner.Spinner
	if opts.WaitMessage != "" {
		s = spinner.New(opts.WaitMessage)
		s.Start()
	}

	select {
	case err := <-errors:
		if s != nil {
			s.Error(err.Error())
		}
		return err
	case <-timer.C:
		if s != nil {
			s.Error(ErrTimeout.Error())
		}
		return ErrTimeout
	case <-ctx.Done():
	}

	if s != nil {
		s.Success()
	}

	return nil
}
