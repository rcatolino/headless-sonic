package events

import (
	"bufio"
	"encoding/json"
	"fmt"
	"headless-sonic/pkg/player"
	"log"
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
	log.Printf("req: %s\n", reqUrl.RequestURI())
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
	defer resp.Body.Close()
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
			log.Printf("json marshalling error %s\n", err)
			continue
		}

		log.Printf("%d: %s\n", counter, string(toprint))
		if event.Action == "set" && len(event.Ids) == 0 {
			musicPlayer.Clear()
		} else if event.Action == "set" {
			musicPlayer.Clear()
			for _, id := range event.Ids {
				musicPlayer.Add(id)
			}
		} else if event.Action == "start" {
			err := musicPlayer.Play()
			if err != nil {
				log.Printf("Play error: %s\n", err)
			}
		}
	}
}
