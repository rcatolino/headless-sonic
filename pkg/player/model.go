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
	Add(id string)
	Clear()
	Done() chan error
	Remove(index int)
	SetGain(gain float32)
	Start()
	Skip(int, int)
	Stop()
}
