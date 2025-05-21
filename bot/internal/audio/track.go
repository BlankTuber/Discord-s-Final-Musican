package audio

import (
	"fmt"
	"time"
)

type Track struct {
	Title       string
	URL         string
	Duration    int
	FilePath    string
	Requester   string
	RequestedAt int64

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

func (t *Track) SetMetadata(duration int, filePath, artist, thumbnail string, isStream bool) {
	t.Duration = duration
	t.FilePath = filePath
	t.ArtistName = artist
	t.ThumbnailURL = thumbnail
	t.IsStream = isStream
	t.DownloadStatus = "completed"
}

func (t *Track) GetDurationString() string {
	// Format duration as MM:SS
	minutes := t.Duration / 60
	seconds := t.Duration % 60
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

func (t *Track) IsDownloaded() bool {
	return t.DownloadStatus == "completed" && t.FilePath != ""
}

func (t *Track) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"title":           t.Title,
		"url":             t.URL,
		"duration":        t.Duration,
		"file_path":       t.FilePath,
		"requester":       t.Requester,
		"requested_at":    t.RequestedAt,
		"artist":          t.ArtistName,
		"thumbnail_url":   t.ThumbnailURL,
		"is_stream":       t.IsStream,
		"download_status": t.DownloadStatus,
		"position":        t.Position,
	}
}

func TrackFromMap(data map[string]interface{}) *Track {
	track := &Track{}

	if title, ok := data["title"].(string); ok {
		track.Title = title
	}

	if url, ok := data["url"].(string); ok {
		track.URL = url
	}

	if duration, ok := data["duration"].(float64); ok {
		track.Duration = int(duration)
	}

	if filePath, ok := data["file_path"].(string); ok {
		track.FilePath = filePath
	}

	if requester, ok := data["requester"].(string); ok {
		track.Requester = requester
	}

	if requestedAt, ok := data["requested_at"].(float64); ok {
		track.RequestedAt = int64(requestedAt)
	}

	if artist, ok := data["artist"].(string); ok {
		track.ArtistName = artist
	}

	if thumbnail, ok := data["thumbnail_url"].(string); ok {
		track.ThumbnailURL = thumbnail
	}

	if isStream, ok := data["is_stream"].(bool); ok {
		track.IsStream = isStream
	}

	if status, ok := data["download_status"].(string); ok {
		track.DownloadStatus = status
	}

	if position, ok := data["position"].(float64); ok {
		track.Position = int(position)
	}

	return track
}
