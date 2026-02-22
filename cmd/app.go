package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"headless-sonic/pkg/config"
	"headless-sonic/pkg/events"
	"headless-sonic/pkg/player"
	"headless-sonic/pkg/subsonic"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
)

type feedbackUpdater struct {
	client   *http.Client
	baseUrl  string
	username string
	password string
}

var _ player.StatusUpdater = &feedbackUpdater{}

// SendStatus implements [player.StatusUpdater].
func (s *feedbackUpdater) SendStatus(ds player.DeviceStatus) error {
	reqUrl, err := url.Parse(s.baseUrl)
	if err != nil {
		return err
	}

	reqUrl.Path = "/rest/jukeboxRemoteFeedback"
	q := reqUrl.Query()
	q.Add("u", s.username)
	q.Add("p", s.password)
	q.Add("v", "1.15.0")
	q.Add("c", "headless-sonic")
	reqUrl.RawQuery = q.Encode()
	body, err := json.Marshal(ds)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", reqUrl.String(), bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	respContent, err := io.ReadAll(resp.Body)
	if err != nil {
		respContent = []byte{}
	}
	slog.Debug("send status response", "component", "SendStatus", "code", resp.StatusCode, "resp", string(respContent))
	return nil
}

func main() {
	os.Exit(run())
}

func run() int {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s config.yaml\n", os.Args[0])
		return 1
	}

	cfg, err := config.Load(os.Args[1])
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		return 1
	}

	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})
	slog.SetDefault(slog.New(h))

	client, err := subsonic.NewClient(cfg)
	if err != nil {
		slog.Error("Failed to create subsonic client", "error", err)
		return 1
	}

	statusUpdater := feedbackUpdater{
		client:   client.Client,
		baseUrl:  client.BaseUrl,
		username: cfg.Username,
		password: cfg.Password,
	}

	p := player.NewPlayer(client, &statusUpdater)
	c, err := events.StartEventHandler(client.Client, client.BaseUrl, p)
	if err != nil {
		slog.Error("Failed to start event handler", "error", err)
		return 1
	}

out:
	for {
		select {
		case err := <-c:
			slog.Warn("Event handler terminated", "error", err)
			break out
		case err := <-p.Done():
			slog.Warn("Player loop terminated", "error", err)
			break out
		}
	}

	return 2
}
