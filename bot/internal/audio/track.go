package audio

import "time"

type Track struct {
    Title          string
    URL            string
    Duration       int
    FilePath       string
    Requester      string
    RequestedAt    int64
    
    ArtistName     string
    ThumbnailURL   string
    IsStream       bool
    DownloadStatus string
    Position       int
}

func NewTrack(title, url, requester string) *Track {
    return &Track{
        Title:          title,
        URL:            url,
        Requester:      requester,
        RequestedAt:    time.Now().Unix(),
        DownloadStatus: "pending",
    }
}