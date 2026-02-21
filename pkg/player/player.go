package player

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"time"
)

type LocalPlayer struct {
	queue         []string
	index         int
	downloader    Downloader
	updater       StatusUpdater
	playNotifier  chan io.ReadCloser
	pauseNotifier chan struct{}
	stopNotifier  chan struct{}
	playbackEnd   chan error
	loopEnded     chan error
	gain          float32
}

// Done implements [Player].
func (m *LocalPlayer) Done() chan error {
	return m.loopEnded
}

func NewPlayer(d Downloader, u StatusUpdater) *LocalPlayer {
	p := LocalPlayer{
		downloader:    d,
		updater:       u,
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
	if m.index >= len(m.queue) {
		return fmt.Errorf("Player: Play error: No more song in queue")
	}

	reader, err := m.downloader.Download(m.queue[m.index])
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
	var playbackStartTime time.Time
	statusUpdateTicker := time.NewTicker(5 * time.Second)
	defer statusUpdateTicker.Stop()

	log.Printf("Player: Starting playback loop")
exit:
	for {
		select {
		case t := <-statusUpdateTicker.C:
			status := DeviceStatus{
				CurrentIndex: m.index,
				Playing:      false,
				Gain:         m.gain,
				Position:     0,
			}

			if cmd != nil && cmd.Process != nil && cmd.ProcessState == nil {
				status.Playing = true
				status.Position = int(t.Sub(playbackStartTime).Seconds())
			}

			m.updater.SendStatus(status)
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
				playbackStartTime = time.Now()
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
			if cmd == nil || cmd.ProcessState == nil {
				// Spurious event: no command has finished (how ?). Ignore
				log.Printf("Warning: Player: playbackEnd notification, but no process has terminated")
				break
			}

			if !cmd.ProcessState.Exited() {
				// Killed by signal -> playback interrupted.
				// TODO: deal with signals other than TERM ?
				log.Printf("Player: playback interrupted by signal\n")
			} else if err != nil && cmd.ProcessState.ExitCode() > 0 {
				log.Printf("Player: playback ended with error: %s\n", err)
			} else {
				log.Printf("Player: playback ended successfully \n")
				// skip to next song
				m.index += 1
			}
		}
	}

	m.loopEnded <- endError
}

var _ Player = &LocalPlayer{}
