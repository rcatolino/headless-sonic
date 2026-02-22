package events

import (
	"bufio"
	"encoding/json"
	"fmt"
	"headless-sonic/pkg/player"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/navidrome/navidrome/server/events"
)

func readEvent(rd *bufio.Reader) (*events.JukeboxCommand, int64, error) {
	var parseError error
	cmd := events.JukeboxCommand{}
	counter := int64(0)
	eventType := ""
	data := ""
	for {
		event, err := rd.ReadString([]byte("\n")[0])
		if err != nil {
			return nil, 0, err
		}

		if event == "\n" {
			break
		}

		fields := strings.SplitN(event, ":", 2)
		if len(fields) != 2 {
			parseError = fmt.Errorf("parsing event: line '%s' doesn't match the 'tag: data' format", event)
			continue
		}

		value := strings.Trim(fields[1], " \n")
		if fields[0] == "id" {
			counter, err = strconv.ParseInt(value, 10, 64)
			parseError = err
			continue
		} else if fields[0] == "event" {
			eventType = value
		} else if fields[0] == "data" {
			data = value
		} else {
			parseError = fmt.Errorf("unknown event tag '%s' in event line '%s'", fields[0], event)
			continue
		}
	}

	if parseError != nil {
		return nil, 0, parseError
	}

	if eventType != "jukeboxCommand" {
		return nil, counter, nil
	}

	err := json.Unmarshal([]byte(data), &cmd)
	if err != nil {
		return nil, 0, err
	}

	return &cmd, counter, nil
}

func StartEventHandler(client *http.Client, baseUrl string, musicPlayer player.Player) (chan error, error) {
	reqUrl, err := url.Parse(baseUrl)
	slog.Info("Starting Event Handler", "base_url", reqUrl.RequestURI())
	if err != nil {
		return nil, err
	}

	reqUrl.Path = "/api/events"
	req, err := http.NewRequest("GET", reqUrl.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	c := make(chan error)
	go processEvents(resp, c, musicPlayer)
	return c, nil
}

func processEvents(resp *http.Response, notificationChan chan error, musicPlayer player.Player) {
	logger := slog.Default().With("component", "processEvents")
	defer resp.Body.Close()
	defer close(notificationChan)
	rd := bufio.NewReader(resp.Body)
	lastEv := int64(0)
	for {
		event, counter, err := readEvent(rd)
		if err != nil {
			notificationChan <- err
			break
		}

		if counter <= lastEv || event == nil {
			continue
		}

		toprint, err := json.Marshal(event)
		if err != nil {
			logger.Warn("json marshalling error", "error", err)
			continue
		}

		logger.Debug("new event received", "counter", counter, "event", string(toprint))
		if event.Action == "set" && len(event.Ids) == 0 {
			musicPlayer.Clear()
		} else if event.Action == "set" {
			musicPlayer.Clear()
			for _, id := range event.Ids {
				musicPlayer.Add(id)
			}

			musicPlayer.Start()
		} else if event.Action == "start" {
			musicPlayer.Start()
		} else if event.Action == "stop" {
			musicPlayer.Stop()
		} else if event.Action == "skip" {
			musicPlayer.Skip(event.Index, event.Offset)
		} else if event.Action == "remove" {
			musicPlayer.Remove(event.Index)
		} else if event.Action == "setGain" {
			musicPlayer.SetGain(event.Gain)
		} else {
			logger.Warn("received unimplemented event action", "action", event.Action)
		}
	}
}
