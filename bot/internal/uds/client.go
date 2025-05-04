package uds

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"os"
	"time"

	"quidque.com/discord-musican/internal/audio"
	"quidque.com/discord-musican/internal/logger"
)

type Client struct {
	socketPath string
	timeout    time.Duration
}

type Request struct {
	Command   string      `json:"command"`
	ID        string      `json:"id"`
	Params    interface{} `json:"params,omitempty"`
	Timestamp string      `json:"timestamp,omitempty"`
}

type Response struct {
	Status    string                 `json:"status"`
	ID        string                 `json:"id"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Timestamp string                 `json:"timestamp,omitempty"`
}

func NewClient(socketPath string) *Client {
	return &Client{
		socketPath: socketPath,
		timeout:    300 * time.Second,
	}
}

func (c *Client) SetTimeout(timeout time.Duration) {
	c.timeout = timeout
}

func (c *Client) generateRequestID() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	rand.Seed(time.Now().UnixNano())
	
	id := make([]byte, 12)
	for i := range id {
		id[i] = charset[rand.Intn(len(charset))]
	}
	
	return string(id)
}

func (c *Client) SendRequest(command string, params interface{}) (*Response, error) {
	requestID := c.generateRequestID()
	
	request := Request{
		Command:   command,
		ID:        requestID,
		Params:    params,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	
	logger.InfoLogger.Printf("UDS: Sending request - Command: %s, ID: %s", command, requestID)
	
	jsonData, err := json.Marshal(request)
	if err != nil {
		logger.ErrorLogger.Printf("UDS: Error marshaling request: %v", err)
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}
	
	startTime := time.Now()
	logger.InfoLogger.Printf("UDS: Connecting to socket at %s (timeout: %v)", c.socketPath, c.timeout)
	conn, err := net.DialTimeout("unix", c.socketPath, c.timeout)
	if err != nil {
		logger.ErrorLogger.Printf("UDS: Error connecting to socket: %v", err)
		return nil, fmt.Errorf("error connecting to socket: %w", err)
	}
	logger.InfoLogger.Printf("UDS: Connected to socket in %v", time.Since(startTime))
	defer conn.Close()
	
	conn.SetDeadline(time.Now().Add(c.timeout))
	logger.InfoLogger.Printf("UDS: Set socket deadline to %v from now", c.timeout)
	
	messageLen := uint32(len(jsonData))
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, messageLen)
	
	logger.InfoLogger.Printf("UDS: Sending message header (%d bytes)", len(lenBuf))
	if _, err := conn.Write(lenBuf); err != nil {
		logger.ErrorLogger.Printf("UDS: Error sending message length: %v", err)
		return nil, fmt.Errorf("error sending message length: %w", err)
	}
	
	logger.InfoLogger.Printf("UDS: Sending message body (%d bytes)", len(jsonData))
	if _, err := conn.Write(jsonData); err != nil {
		logger.ErrorLogger.Printf("UDS: Error sending message data: %v", err)
		return nil, fmt.Errorf("error sending message data: %w", err)
	}
	
	logger.InfoLogger.Printf("UDS: Waiting for response header...")
	header := make([]byte, 4)
	if _, err := conn.Read(header); err != nil {
		logger.ErrorLogger.Printf("UDS: Error reading response header: %v", err)
		return nil, fmt.Errorf("error reading response header: %w", err)
	}
	
	responseLen := binary.BigEndian.Uint32(header)
	logger.InfoLogger.Printf("UDS: Response size: %d bytes", responseLen)
	
	responseBuf := make([]byte, responseLen)
	bytesRead := 0
	
	logger.InfoLogger.Printf("UDS: Reading response body...")
	for bytesRead < int(responseLen) {
		n, err := conn.Read(responseBuf[bytesRead:])
		if err != nil {
			logger.ErrorLogger.Printf("UDS: Error reading response data at byte %d: %v", bytesRead, err)
			return nil, fmt.Errorf("error reading response data: %w", err)
		}
		bytesRead += n
		logger.DebugLogger.Printf("UDS: Read %d bytes, total %d of %d", n, bytesRead, responseLen)
	}
	
	var response Response
	if err := json.Unmarshal(responseBuf, &response); err != nil {
		logger.ErrorLogger.Printf("UDS: Error unmarshaling response: %v", err)
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}
	
	elapsedTime := time.Since(startTime)
	logger.InfoLogger.Printf("UDS: Received response - Command: %s, ID: %s, Status: %s, Time: %v", command, requestID, response.Status, elapsedTime)
	
	if response.Status == "error" {
		logger.ErrorLogger.Printf("UDS: Response error: %s", response.Error)
		return &response, errors.New(response.Error)
	}
	
	return &response, nil
}

func (c *Client) DownloadPlaylistItem(url string, index int, maxDuration int, maxSize int, allowLive bool) (*audio.Track, error) {
	logger.InfoLogger.Printf("UDS: Downloading playlist item %d from URL: %s", index, url)
	params := map[string]interface{}{
		"url":                  url,
		"index":                index,
		"max_duration_seconds": maxDuration,
		"max_size_mb":          maxSize,
		"allow_live":           allowLive,
	}
	
	timeout := c.timeout
	newTimeout := 3 * 60 * time.Second
	logger.InfoLogger.Printf("UDS: Setting timeout from %v to %v for playlist item download", timeout, newTimeout)
	c.SetTimeout(newTimeout)
	defer c.SetTimeout(timeout)
	
	startTime := time.Now()
	response, err := c.SendRequest("download_playlist_item", params)
	if err != nil {
		logger.ErrorLogger.Printf("UDS: Download playlist item failed: %v", err)
		return nil, err
	}
	
	logger.InfoLogger.Printf("UDS: Download playlist item completed in %v", time.Since(startTime))
	
	if response.Status != "success" {
		errMsg := "Playlist item download failed"
		if response.Error != "" {
			errMsg = response.Error
		}
		logger.ErrorLogger.Printf("UDS: Playlist item download error: %s", errMsg)
		return nil, errors.New(errMsg)
	}
	
	track := &audio.Track{
		URL:          url,
		Requester:    "",
		RequestedAt:  time.Now().Unix(),
		Position:     index,
	}
	
	if data, ok := response.Data["title"].(string); ok {
		track.Title = data
	} else {
		track.Title = fmt.Sprintf("Unknown Track %d", index+1)
	}
	
	if data, ok := response.Data["filename"].(string); ok {
		track.FilePath = data
		// Validate file path
		if track.FilePath != "" {
			if _, err := os.Stat(track.FilePath); os.IsNotExist(err) {
				logger.WarnLogger.Printf("UDS: File path exists in response but file not found: %s", track.FilePath)
				return nil, fmt.Errorf("file not found: %s", track.FilePath)
			}
		} else {
			logger.WarnLogger.Println("UDS: Empty file path received in response")
			return nil, errors.New("empty file path in response")
		}
	}
	
	if data, ok := response.Data["duration"].(float64); ok {
		track.Duration = int(data)
	}
	
	if data, ok := response.Data["artist"].(string); ok {
		track.ArtistName = data
	}
	
	if data, ok := response.Data["thumbnail_url"].(string); ok {
		track.ThumbnailURL = data
	}
	
	if data, ok := response.Data["is_stream"].(bool); ok {
		track.IsStream = data
	}
	
	track.DownloadStatus = "completed"
	
	// Log important track info
	logger.InfoLogger.Printf("UDS: Playlist item processed - Title: %s, FilePath: %s", track.Title, track.FilePath)
	
	return track, nil
}

func (c *Client) GetPlaylistInfo(url string, maxItems int) (string, int, error) {
	logger.InfoLogger.Printf("UDS: Getting playlist info for URL: %s, max items: %d", url, maxItems)
	params := map[string]interface{}{
		"url":       url,
		"max_items": maxItems,
	}
	
	response, err := c.SendRequest("get_playlist_info", params)
	if err != nil {
		logger.ErrorLogger.Printf("UDS: Get playlist info failed: %v", err)
		return "", 0, err
	}
	
	var playlistTitle string
	var totalTracks int
	
	if data, ok := response.Data["playlist_title"].(string); ok {
		playlistTitle = data
	} else {
		playlistTitle = "Unknown Playlist"
	}
	
	if data, ok := response.Data["total_tracks"].(float64); ok {
		totalTracks = int(data)
		if totalTracks > maxItems {
			totalTracks = maxItems
		}
	} else {
		totalTracks = maxItems
	}
	
	isPlaylist := false
	if data, ok := response.Data["is_playlist"].(bool); ok {
		isPlaylist = data
	}
	
	if !isPlaylist {
		// This is a single track, not a playlist
		return playlistTitle, 1, nil
	}
	
	logger.InfoLogger.Printf("UDS: Playlist info retrieved - Title: %s, Total tracks: %d", 
		playlistTitle, totalTracks)
	
	return playlistTitle, totalTracks, nil
}


func (c *Client) Ping() error {
	logger.InfoLogger.Println("UDS: Sending ping request")
	params := map[string]string{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	
	response, err := c.SendRequest("ping", params)
	if err != nil {
		logger.ErrorLogger.Printf("UDS: Ping failed: %v", err)
		return err
	}
	
	logger.InfoLogger.Printf("UDS: Ping successful: %+v", response.Data)
	return nil
}

func (c *Client) DownloadAudio(url string, maxDuration int, maxSize int, allowLive bool) (*audio.Track, error) {
	logger.InfoLogger.Printf("UDS: Downloading audio from %s", url)
	params := map[string]interface{}{
		"url":                  url,
		"max_duration_seconds": maxDuration,
		"max_size_mb":          maxSize,
		"allow_live":           allowLive,
	}
	
	timeout := c.timeout
	newTimeout := 5 * 60 * time.Second
	logger.InfoLogger.Printf("UDS: Setting timeout from %v to %v for download", timeout, newTimeout)
	c.SetTimeout(newTimeout)
	defer c.SetTimeout(timeout)
	
	startTime := time.Now()
	response, err := c.SendRequest("download_audio", params)
	if err != nil {
		logger.ErrorLogger.Printf("UDS: Download audio failed: %v", err)
		return nil, err
	}
	
	logger.InfoLogger.Printf("UDS: Download audio completed in %v", time.Since(startTime))
	
	if response.Status != "success" {
		errMsg := "Download failed"
		if response.Error != "" {
			errMsg = response.Error
		}
		logger.ErrorLogger.Printf("UDS: Download error: %s", errMsg)
		return nil, errors.New(errMsg)
	}
	
	track := &audio.Track{
		URL:          url,
		Requester:    "",
		RequestedAt:  time.Now().Unix(),
	}
	
	if data, ok := response.Data["title"].(string); ok {
		track.Title = data
	}
	
	if data, ok := response.Data["filename"].(string); ok {
		track.FilePath = data
		// Validate file path
		if track.FilePath != "" {
			if _, err := os.Stat(track.FilePath); os.IsNotExist(err) {
				logger.WarnLogger.Printf("UDS: File path exists in response but file not found: %s", track.FilePath)
			}
		} else {
			logger.WarnLogger.Println("UDS: Empty file path received in response")
		}
	}
	
	if data, ok := response.Data["duration"].(float64); ok {
		track.Duration = int(data)
	}
	
	if data, ok := response.Data["artist"].(string); ok {
		track.ArtistName = data
	}
	
	if data, ok := response.Data["thumbnail_url"].(string); ok {
		track.ThumbnailURL = data
	}
	
	if data, ok := response.Data["is_stream"].(bool); ok {
		track.IsStream = data
	}
	
	track.DownloadStatus = "completed"
	
	// Log important track info
	logger.InfoLogger.Printf("UDS: Track processed - Title: %s, FilePath: %s", track.Title, track.FilePath)
	
	return track, nil
}

func (c *Client) DownloadPlaylist(url string, maxItems int, maxDuration int, maxSize int, allowLive bool) ([]*audio.Track, error) {
	logger.InfoLogger.Printf("UDS: Downloading playlist from %s (max: %d items)", url, maxItems)
	params := map[string]interface{}{
		"url":                  url,
		"max_items":            maxItems,
		"max_duration_seconds": maxDuration,
		"max_size_mb":          maxSize,
		"allow_live":           allowLive,
	}
	
	timeout := c.timeout
	newTimeout := 10 * 60 * time.Second
	logger.InfoLogger.Printf("UDS: Setting timeout from %v to %v for playlist download", timeout, newTimeout)
	c.SetTimeout(newTimeout)
	defer c.SetTimeout(timeout)
	
	startTime := time.Now()
	response, err := c.SendRequest("download_playlist", params) // Make sure to use download_playlist
	if err != nil {
		logger.ErrorLogger.Printf("UDS: Download playlist failed: %v", err)
		return nil, err
	}
	
	logger.InfoLogger.Printf("UDS: Download playlist completed in %v", time.Since(startTime))
	
	if response.Status != "success" {
		errMsg := "Playlist download failed"
		if response.Error != "" {
			errMsg = response.Error
		}
		logger.ErrorLogger.Printf("UDS: Playlist download error: %s", errMsg)
		return nil, errors.New(errMsg)
	}
	
	var tracks []*audio.Track
	
	if countValue, ok := response.Data["count"].(float64); ok {
		count := int(countValue)
		logger.InfoLogger.Printf("UDS: Playlist has %d tracks", count)
		tracks = make([]*audio.Track, 0, count)
	}
	
	if itemsData, ok := response.Data["items"].([]interface{}); ok {
		logger.InfoLogger.Printf("UDS: Processing %d playlist items", len(itemsData))
		for i, itemData := range itemsData {
			if item, ok := itemData.(map[string]interface{}); ok {
				track := &audio.Track{
					URL:          url,
					RequestedAt:  time.Now().Unix(),
					DownloadStatus: "completed",
				}
				
				if data, ok := item["title"].(string); ok {
					track.Title = data
				} else {
					logger.WarnLogger.Printf("UDS: Missing title for playlist item %d", i)
					track.Title = fmt.Sprintf("Unknown Track %d", i+1)
				}
				
				// Important: File path validation
				if data, ok := item["filename"].(string); ok && data != "" {
					track.FilePath = data
					// Check if file exists
					if _, err := os.Stat(track.FilePath); os.IsNotExist(err) {
						logger.WarnLogger.Printf("UDS: File not found for '%s': %s", track.Title, track.FilePath)
						// Skip this track
						continue
					}
				} else {
					logger.WarnLogger.Printf("UDS: Missing file path for '%s'", track.Title)
					// Skip this track without file path
					continue
				}
				
				if data, ok := item["duration"].(float64); ok {
					track.Duration = int(data)
				}
				
				if data, ok := item["artist"].(string); ok {
					track.ArtistName = data
				}
				
				if data, ok := item["thumbnail_url"].(string); ok {
					track.ThumbnailURL = data
				}
				
				logger.InfoLogger.Printf("UDS: Adding playlist item %d - %s (%s)", i, track.Title, track.FilePath)
				tracks = append(tracks, track)
			}
		}
	}
	
	validCount := len(tracks)
	logger.InfoLogger.Printf("UDS: Found %d valid tracks with existing files", validCount)
	
	if validCount == 0 {
		logger.ErrorLogger.Printf("UDS: No valid tracks found in playlist")
		return nil, errors.New("no valid tracks found in playlist")
	}
	
	return tracks, nil
}


func (c *Client) Search(query string, platform string, limit int, includeLive bool) ([]*audio.Track, error) {
	logger.InfoLogger.Printf("UDS: Searching for %s on %s (limit: %d)", query, platform, limit)
	params := map[string]interface{}{
		"query":        query,
		"platform":     platform,
		"limit":        limit,
		"include_live": includeLive,
	}
	
	timeout := c.timeout
	newTimeout := 3 * 60 * time.Second
	logger.InfoLogger.Printf("UDS: Setting timeout from %v to %v for search", timeout, newTimeout)
	c.SetTimeout(newTimeout)
	defer c.SetTimeout(timeout)
	
	startTime := time.Now()
	logger.InfoLogger.Printf("UDS: Starting search request...")
	response, err := c.SendRequest("search", params)
	if err != nil {
		logger.ErrorLogger.Printf("UDS: Search failed: %v", err)
		return nil, err
	}
	
	logger.InfoLogger.Printf("UDS: Search completed in %v", time.Since(startTime))
	
	var results []interface{}
	
	if data, ok := response.Data["results"].([]interface{}); ok {
		results = data
		logger.InfoLogger.Printf("UDS: Search returned %d results", len(results))
	} else {
		logger.WarnLogger.Printf("UDS: Search results not in expected format")
		return []*audio.Track{}, nil
	}
	
	tracks := make([]*audio.Track, 0, len(results))
	
	for i, result := range results {
		if mapResult, ok := result.(map[string]interface{}); ok {
			track := &audio.Track{
				RequestedAt:  time.Now().Unix(),
				DownloadStatus: "pending",
			}
			
			if data, ok := mapResult["title"].(string); ok {
				track.Title = data
			}
			
			if data, ok := mapResult["url"].(string); ok {
				track.URL = data
			}
			
			if data, ok := mapResult["duration"].(float64); ok {
				track.Duration = int(data)
			}
			
			if data, ok := mapResult["thumbnail"].(string); ok {
				track.ThumbnailURL = data
			}
			
			if data, ok := mapResult["uploader"].(string); ok {
				track.ArtistName = data
			}
			
			logger.DebugLogger.Printf("UDS: Search result %d - %s", i, track.Title)
			tracks = append(tracks, track)
		}
	}
	
	logger.InfoLogger.Printf("UDS: Processed %d search tracks", len(tracks))
	return tracks, nil
}