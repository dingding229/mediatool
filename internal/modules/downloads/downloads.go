package downloads

import "context"

type Request struct {
	Torrent            []byte
	SavePath, Category string
	Tags               []string
}
type Task struct {
	ID, State, Name            string
	DownloadSpeed, UploadSpeed int64
	Seeds, Peers               int
	Progress                   float64
	SavePath, ContentPath      string
	Size, SeedingTime          int64
	Ratio                      float64
	Complete                   bool
	Seeding                    bool
}
type Downloader interface {
	Add(context.Context, Request) (string, error)
	Inspect(context.Context, string) (Task, error)
}

type File struct {
	Index    int     `json:"index"`
	Name     string  `json:"name"`
	Size     int64   `json:"size"`
	Progress float64 `json:"progress"`
	Priority int     `json:"priority"`
}

type Tracker struct {
	URL     string `json:"url"`
	Status  int    `json:"status"`
	Message string `json:"message"`
	Peers   int    `json:"peers"`
	Seeds   int    `json:"seeds"`
	Leeches int    `json:"leeches"`
}
