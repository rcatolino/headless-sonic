package main

import (
	"bytes"
	"encoding/json"
	"headless-sonic/pkg/config"
	"headless-sonic/pkg/events"
	"headless-sonic/pkg/player"
	"headless-sonic/pkg/subsonic"
	"io"
	"log"
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
	log.Printf("SendStatus response: %d (%s)", resp.StatusCode, string(respContent))
	return nil
}

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("Usage: %s config.yaml", os.Args[0])
	}

	cfg, err := config.Load(os.Args[1])
	if err != nil {
		log.Fatalf("error loading config: %s", err)
	}

	client, err := subsonic.NewClient(cfg)
	if err != nil {
		log.Fatalf("error creating subsonic client: %s", err)
	}

	statusUpdater := feedbackUpdater{
		client:  client.Client,
		baseUrl: client.BaseUrl,
		username: cfg.Username,
		password: cfg.Password,
	}

	p := player.NewPlayer(client, &statusUpdater)
	c, err := events.StartEventHandler(client.Client, client.BaseUrl, p)
	if err != nil {
		log.Fatalf("error waiting for events: %s\n", err)
	}

	for {
		select {
		case err := <-c:
			log.Printf("Event handler terminated: %s\n", err)
		case err := <-p.Done():
			log.Printf("Player loop terminated: %s\n", err)
		}
	}
}
