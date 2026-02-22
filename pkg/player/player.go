package player

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"time"
)

type LocalPlayer struct {
	queue        []string
	index        int
	downloader   Downloader
	updater      StatusUpdater
	playNotifier chan io.ReadCloser
	stopNotifier chan struct{}
	playbackEnd  chan error
	loopEnded    chan error
	gain         float32
	logger       *slog.Logger
}

// Done implements [Player].
func (m *LocalPlayer) Done() chan error {
	return m.loopEnded
}

func NewPlayer(d Downloader, u StatusUpdater) *LocalPlayer {
	p := LocalPlayer{
		downloader:   d,
		updater:      u,
		playNotifier: make(chan io.ReadCloser),
		stopNotifier: make(chan struct{}),
		playbackEnd:  make(chan error),
		logger:       slog.Default().With("component", "LocalPlayer"),
	}

	go p.RunPlayerLoop()
	return &p
}

// Add implements [Player].
func (m *LocalPlayer) Add(id string) {
	m.logger.Debug("Add handler called", "song_id", id)
	m.queue = append(m.queue, id)
}

// Clear implements [Player].
func (m *LocalPlayer) Clear() {
	m.logger.Debug("Clear handler called")
	m.queue = []string{}
	m.index = 0
	m.stopNotifier <- struct{}{}
}

// Insert implements [Player].
func (m *LocalPlayer) Insert(id string, index int) {
	m.logger.Debug("Insert handler called")
	q := append(m.queue[:index], id)
	m.queue = append(q, m.queue[index:]...)
}

// Pause implements [Player].
func (m *LocalPlayer) Pause() {
	panic("unimplemented")
}

func (m *LocalPlayer) Stop() {
	m.stopNotifier <- struct{}{}
}

// Play implements [Player].
func (m *LocalPlayer) Play() error {
	m.logger.Debug("Play handler called")
	if m.index >= len(m.queue) {
		return fmt.Errorf("Play error: No more song in queue")
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

func (m *LocalPlayer) sendStatus(cmd *exec.Cmd, position int, stopPos int) {
	status := DeviceStatus{
		CurrentIndex: m.index,
		Playing:      false,
		Gain:         m.gain,
		Position:     stopPos,
	}

	if cmd != nil && cmd.Process != nil && cmd.ProcessState == nil {
		// Currently playing
		status.Playing = true
		status.Position = position
	}

	m.updater.SendStatus(status)
}

func (m *LocalPlayer) RunPlayerLoop() {
	var cmd *exec.Cmd
	var endError error
	var playbackStartTime time.Time
	playbackStopPos := 0 * time.Second
	statusUpdateTicker := time.NewTicker(5 * time.Second)
	defer statusUpdateTicker.Stop()

	m.logger.Info("Starting playback loop")
exit:
	for {
		select {
		case t := <-statusUpdateTicker.C:
			m.sendStatus(cmd, int(t.Sub(playbackStartTime).Seconds()), int(playbackStopPos.Seconds()))
		case streamReader := <-m.playNotifier:
			if cmd != nil && cmd.Process != nil {
				m.logger.Debug("overriding current playback process")
				err := cmd.Process.Kill()
				if err != nil {
					m.logger.Warn("playback override error killing previous process", "error", err)
				}

				s, err := cmd.Process.Wait()
				m.logger.Debug("playback override, subprocess ended", "state", s)
				if err != nil {
					m.logger.Warn("playback override error while waiting for previous process", "error", err)
				}
			}

			m.logger.Debug("player loop, starting playback")
			cmd = exec.Command("ffplay", "-i", "-", "-vn", "-nodisp", "-hide_banner", "-ss", fmt.Sprintf("%ds", int(playbackStopPos.Seconds())))
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Stdin = streamReader
			err := cmd.Start()
			if err != nil {
				m.logger.Warn("playback start failed with error", "error", err)
				streamReader.Close()
				endError = err
				break exit
			} else {
				playbackStartTime = time.Now().Add(-playbackStopPos)
				go func() {
					err := cmd.Wait()
					streamReader.Close()
					m.playbackEnd <- err
				}()
				m.sendStatus(cmd, 0, 0)
			}
		case <-m.stopNotifier:
			if cmd == nil || cmd.Process == nil {
				m.logger.Debug("stop action called, but no process is running. Ignoring")
			} else {
				err := cmd.Process.Kill()
				if err != nil {
					m.logger.Debug("failed to kill ffmpeg subprocess", "error", err)
				}
			}
		case err := <-m.playbackEnd:
			if cmd == nil || cmd.ProcessState == nil {
				// Spurious event: no command has finished (how ?). Ignore
				m.logger.Warn("playbackEnd notification, but no process has terminated")
				break
			}

			playbackStopPos = time.Since(playbackStartTime)
			if !cmd.ProcessState.Exited() {
				// Killed by signal -> playback interrupted.
				// TODO: deal with signals other than TERM ?
				m.logger.Debug("playback interrupted by signal")
			} else if err != nil && cmd.ProcessState.ExitCode() > 0 {
				m.logger.Warn("playback ended with errors", "error", err)
			} else {
				m.logger.Debug("playback ended successfully ")
				// skip to next song
				playbackStopPos = 0
				m.index += 1
			}

			m.sendStatus(cmd, 0, int(playbackStopPos*time.Second))
		}
	}

	m.loopEnded <- endError
}

var _ Player = &LocalPlayer{}
