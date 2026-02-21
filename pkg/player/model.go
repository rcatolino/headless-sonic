package player

import "io"

type Downloader interface {
	Download(id string) (io.ReadCloser, error)
}

type Player interface {
	Play() error
	Pause()
	TogglePlayPause()
	Clear()
	Add(id string)
	Insert(id string, index int)
	Done() chan error
}
