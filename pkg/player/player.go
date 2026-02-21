package player

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
)

type LocalPlayer struct {
	queue         []string
	downloader    Downloader
	playNotifier  chan io.ReadCloser
	pauseNotifier chan struct{}
	stopNotifier  chan struct{}
	playbackEnd   chan error
	loopEnded     chan error
}

// Done implements [Player].
func (m *LocalPlayer) Done() chan error {
	return m.loopEnded
}

func NewPlayer(d Downloader) *LocalPlayer {
	p := LocalPlayer{
		downloader:    d,
		playNotifier:  make(chan io.ReadCloser),
		pauseNotifier: make(chan struct{}),
		stopNotifier:  make(chan struct{}),
		playbackEnd:   make(chan error),
	}

	go p.RunPlayerLoop()
	return &p
}

// Add implements [Player].
func (m *LocalPlayer) Add(id string) {
	log.Printf("Player: Add handler called\n")
	m.queue = append(m.queue, id)
}

// Clear implements [Player].
func (m *LocalPlayer) Clear() {
	log.Printf("Player: Clear handler called\n")
	m.queue = []string{}
	m.stopNotifier <- struct{}{}
}

// Insert implements [Player].
func (m *LocalPlayer) Insert(id string, index int) {
	log.Printf("Player: Insert handler called\n")
	q := append(m.queue[:index], id)
	m.queue = append(q, m.queue[index:]...)
}

// Pause implements [Player].
func (m *LocalPlayer) Pause() {
	panic("unimplemented")
}

// Play implements [Player].
func (m *LocalPlayer) Play() error {
	log.Printf("Player: Play handler called\n")
	if len(m.queue) == 0 {
		return fmt.Errorf("Player: Play error: No song in queue")
	}

	reader, err := m.downloader.Download(m.queue[0])
	if err != nil {
		return err
	}

	m.playNotifier <- reader
	return nil
}

// TogglePlayPause implements [Player].
func (m *LocalPlayer) TogglePlayPause() {
	panic("unimplemented")
}

func (m *LocalPlayer) RunPlayerLoop() {
	var cmd *exec.Cmd
	var endError error

	log.Printf("Player: Starting playback loop")
	exit:
	for {
		select {
		case streamReader := <-m.playNotifier:
			if cmd != nil && cmd.Process != nil {
				log.Printf("Player: overriding current playback process")
				err := cmd.Process.Kill()
				if err != nil {
					log.Printf("Player: playback override error killing previous process: %s\n", err)
				}

				s, err := cmd.Process.Wait()
				log.Printf("Process end state: %s", s)
				if err != nil {
					log.Printf("Player: playback override error waiting for previous process: %s\n", err)
				}
			}

			log.Printf("Player: player loop, starting playback\n")
			cmd = exec.Command("ffplay", "-i", "-", "-vn", "-nodisp", "-hide_banner")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Stdin = streamReader
			err := cmd.Start()
			if err != nil {
				log.Printf("Player: playback start failed with error: %s\n", err)
				streamReader.Close()
				endError = err
				break exit
			} else {
				go func() {
					err := cmd.Wait()
					streamReader.Close()
					m.playbackEnd <- err
				}()
			}
		case <-m.pauseNotifier:
			log.Printf("Player: pause action called. Not implemented yet.\n")
		case <-m.stopNotifier:
			if cmd == nil || cmd.Process == nil {
				log.Printf("Player: stop action called, but no process is running. Ignoring\n")
			} else {
				err := cmd.Process.Kill()
				if err != nil {
					log.Printf("Player: ffmpeg kill error: %s\n", err)
				}
			}
		case err := <-m.playbackEnd:
			if err != nil {
				if cmd != nil && cmd.ProcessState != nil && cmd.ProcessState.ExitCode() >= 0 {
					log.Printf("Player: playback ended with error: %s\n", err)
				}
			} else {
				log.Printf("Player: playback ended\n")
			}
			//TODO: Skip to next item in queue
		}
	}

	m.loopEnded <- endError
}

var _ Player = &LocalPlayer{}
