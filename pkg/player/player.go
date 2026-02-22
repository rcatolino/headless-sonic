package player

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"time"
)

type LocalPlayer struct {
	queue         []string
	queueMutex    sync.Mutex
	downloader    Downloader
	updater       StatusUpdater
	startNotifier chan struct{}
	stopNotifier  chan struct{}
	clearNotifier chan struct{}
	skipNotifier  chan skipPayload
	playbackEnd   chan error
	loopEnded     chan error
	logger        *slog.Logger
}

var _ Player = &LocalPlayer{}

type skipPayload struct {
	index  int
	offset int
}

// Done implements [Player].
func (m *LocalPlayer) Done() chan error {
	return m.loopEnded
}

func NewPlayer(d Downloader, u StatusUpdater) *LocalPlayer {
	p := LocalPlayer{
		downloader:    d,
		updater:       u,
		startNotifier: make(chan struct{}),
		stopNotifier:  make(chan struct{}),
		clearNotifier: make(chan struct{}),
		skipNotifier:  make(chan skipPayload),
		playbackEnd:   make(chan error),
		loopEnded:     make(chan error),
		logger:        slog.Default().With("component", "LocalPlayer"),
	}

	go p.RunPlayerLoop()
	return &p
}

// Add implements [Player].
func (m *LocalPlayer) Add(id string) {
	m.logger.Debug("Add handler called", "song_id", id)
	m.queueMutex.Lock()
	defer m.queueMutex.Unlock()
	m.queue = append(m.queue, id)
}

// Clear implements [Player].
func (m *LocalPlayer) Clear() {
	m.logger.Debug("Clear handler called")
	m.queueMutex.Lock()
	m.queue = []string{}
	m.queueMutex.Unlock()

	m.clearNotifier <- struct{}{}
}

// Insert implements [Player].
func (m *LocalPlayer) Insert(id string, index int) {
	m.logger.Debug("Insert handler called")
	m.queueMutex.Lock()
	defer m.queueMutex.Unlock()

	q := append(m.queue[:index], id)
	m.queue = append(q, m.queue[index:]...)
}

func (m *LocalPlayer) Skip(index int, offset int) {
	m.skipNotifier <- skipPayload{
		index:  index,
		offset: offset,
	}
}

func (m *LocalPlayer) Stop() {
	m.stopNotifier <- struct{}{}
}

// Start implements [Player].
func (m *LocalPlayer) Start() {
	m.logger.Debug("Play handler called")
	m.startNotifier <- struct{}{}
}

// TogglePlayPause implements [Player].
func (m *LocalPlayer) TogglePlayPause() {
	panic("unimplemented")
}

func (m *LocalPlayer) RunPlayerLoop() {
	state := playbackState{
		gain: 1,
	}
	var endError error
	statusUpdateTicker := time.NewTicker(5 * time.Second)
	defer statusUpdateTicker.Stop()

	m.logger.Info("Starting playback loop")

exit:
	for {
		select {
		case t := <-statusUpdateTicker.C:
			m.sendStatus(t, &state)
		case <-m.startNotifier:
			endError = m.startPlayback(&state)
			if endError != nil {
				break exit
			}
		case <-m.clearNotifier:
			state.index = 0
			state.offset = 0
			m.stopCurrentPlayback(state.cmd)
		case <-m.stopNotifier:
			m.stopCurrentPlayback(state.cmd)
		case skipSettings := <-m.skipNotifier:
			m.stopCurrentPlayback(state.cmd)
			state.offset = time.Duration(skipSettings.offset) * time.Second
			state.index = skipSettings.index
			endError = m.startPlayback(&state)
			if endError != nil {
				break exit
			}
		case err := <-m.playbackEnd:
			if state.cmd == nil {
				// Spurious event: no command has finished (how ?). Ignore
				m.logger.Warn("playbackEnd notification, but no process was started")
				break
			}

			state.offset = time.Since(state.startTime)
			sid, _ := m.getQueueItem(&state)
			if state.cmd.ProcessState == nil || !state.cmd.ProcessState.Exited() {
				// I don't understand how the ProcessState can be nil, but sometimes it is...
				// Handle this case as an 'interrupted' playback.
				// Otherwise, killed by signal -> playback interrupted.
				// we conserve the current offset in this case, to be able to resume later
				// TODO: deal with signals other than TERM ?
				m.logger.Info("playback interrupted", "index", state.index, "song_id", sid, "offset", state.offset)
			} else if err != nil && state.cmd.ProcessState.ExitCode() > 0 {
				// This can happend if the audio format is invalid/unsupported. Maybe skip to next song in this case ?
				m.logger.Warn("playback ended with errors", "error", err)
			} else {
				m.logger.Info("playback end", "index", state.index, "song_id", sid)
				// move to next song
				state.offset = 0
				state.index += 1
				endError = m.startPlayback(&state)
				if endError != nil {
					break exit
				}
			}

			m.sendStatus(time.Now(), &state)
		}
	}

	m.loopEnded <- endError
}

type playbackState struct {
	cmd       *exec.Cmd
	startTime time.Time
	offset    time.Duration
	index     int
	gain      float32
}

// The following methods should only be called from the player loop routine, to prevent concurrent state modification
func (m *LocalPlayer) sendStatus(tickerTime time.Time, state *playbackState) {
	status := DeviceStatus{
		CurrentIndex: state.index,
		Playing:      false,
		Gain:         state.gain,
		Position:     0,
	}

	if state.cmd != nil && state.cmd.Process != nil && state.cmd.ProcessState == nil {
		// Currently playing
		state.offset = tickerTime.Sub(state.startTime) // would probably be better handled by an offset() getter on state
		status.Playing = true
	}

	status.Position = int(state.offset.Seconds())

	m.updater.SendStatus(status)
}

func (m *LocalPlayer) stopCurrentPlayback(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		m.logger.Debug("stop playback called, but no process is running. Ignoring")
	} else {
		err := cmd.Process.Kill()
		if err != nil {
			m.logger.Debug("failed to kill ffmpeg subprocess", "error", err)
			return
		}
		s, err := cmd.Process.Wait()
		m.logger.Debug("subprocess has ended", "state", s)
		if err != nil {
			m.logger.Warn("error while waiting for subprocess end", "error", err)
		}
	}
}

func (m *LocalPlayer) getQueueItem(state *playbackState) (string, error) {
	m.queueMutex.Lock()
	defer m.queueMutex.Unlock()
	if state.index >= len(m.queue) {
		return "", fmt.Errorf("State index %d is past the end of the playback queue", state.index)
	}

	return m.queue[state.index], nil
}

func (m *LocalPlayer) startPlayback(state *playbackState) error {
	sid, err := m.getQueueItem(state)
	if err != nil {
		slog.Info("Playlist end: no more song in queue")
		return nil
	}

	reader, err := m.downloader.Download(sid)
	if err != nil {
		slog.Warn("Failed to download song", "song_id", sid, "error", err)
		return nil
	}

	m.stopCurrentPlayback(state.cmd)
	seekpos := fmt.Sprintf("%d", int(state.offset.Seconds()))
	m.logger.Debug("player loop, starting playback", "ss", seekpos)
	// state.cmd = exec.Command("ffplay", "-i", "-", "-codec:audio", "pcm_s32le", "-ss", seekpos, "-autoexit", "-vn", "-nodisp", "-hide_banner")
	state.cmd = exec.Command("ffmpeg", "-nostats", "-hide_banner", "-ss", seekpos, "-i", "-", "-vn", "-f", "pulse", "headless-sonic")
	state.cmd.Stdout = os.Stdout
	state.cmd.Stderr = os.Stderr
	state.cmd.Stdin = reader
	err = state.cmd.Start()
	if err != nil {
		m.logger.Warn("playback start failed with error", "error", err)
		reader.Close()
		return err // If we can't actuall play the songs then we might as well give up
	} else {
		state.startTime = time.Now().Add(-state.offset)
		m.logger.Info("playback start", "song_id", sid, "offset", state.offset)
		go func() {
			err := state.cmd.Wait()
			reader.Close()
			m.playbackEnd <- err
		}()
		m.sendStatus(time.Now(), state)
	}

	return nil
}
