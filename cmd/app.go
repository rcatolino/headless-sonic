package main

import (
	"headless-sonic/pkg/config"
	"headless-sonic/pkg/events"
	"headless-sonic/pkg/player"
	"headless-sonic/pkg/subsonic"
	"log"
	"os"
)

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

	p := player.NewPlayer(client)
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
