package player

import "io"

type DeviceStatus struct {
	CurrentIndex int     `json:"currentIndex"`
	Playing      bool    `json:"playing"`
	Gain         float32 `json:"gain"`
	Position     int     `json:"position"`
}

type Downloader interface {
	Download(id string) (io.ReadCloser, error)
}

type StatusUpdater interface {
	SendStatus(DeviceStatus) error
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
