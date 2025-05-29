package socket

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"musicbot/internal/logger"
	"musicbot/internal/state"
	"net"
	"sync"
	"time"
)

type DownloadRequest struct {
	Command string                 `json:"command"`
	ID      string                 `json:"id"`
	Params  map[string]interface{} `json:"params"`
}

type SearchRequest struct {
	Command string                 `json:"command"`
	ID      string                 `json:"id"`
	Params  map[string]interface{} `json:"params"`
}

type Client struct {
	socketPath           string
	conn                 net.Conn
	connected            bool
	downloadHandler      func(*state.Song)
	playlistHandler      func([]state.Song)
	searchHandler        func([]SearchResult)
	playlistEventHandler func(string, *state.Song)
	playlistStartHandler func(int)
	resetPendingHandler  func()
	mu                   sync.RWMutex
	pendingRequests      map[string]chan interface{}
}

func NewClient(socketPath string) *Client {
	return &Client{
		socketPath:      socketPath,
		pendingRequests: make(map[string]chan interface{}),
	}
}

func (c *Client) SetResetPendingHandler(handler func()) {
	c.resetPendingHandler = handler
}

func (c *Client) SetPlaylistStartHandler(handler func(int)) {
	c.playlistStartHandler = handler
}

func (c *Client) SetDownloadHandler(handler func(*state.Song)) {
	c.downloadHandler = handler
}

func (c *Client) SetPlaylistHandler(handler func([]state.Song)) {
	c.playlistHandler = handler
}

func (c *Client) SetSearchHandler(handler func([]SearchResult)) {
	c.searchHandler = handler
}

func (c *Client) SetPlaylistEventHandler(handler func(string, *state.Song)) {
	c.playlistEventHandler = handler
}

func (c *Client) Connect() error {
	logger.Info.Printf("Connecting to socket: %s", c.socketPath)

	conn, err := net.DialTimeout("unix", c.socketPath, 10*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to socket: %w", err)
	}

	c.conn = conn
	c.connected = true

	if c.resetPendingHandler != nil {
		c.resetPendingHandler()
	}

	go c.listenForResponses()

	logger.Info.Println("Successfully connected to socket")
	return nil
}

func (c *Client) Disconnect() error {
	if !c.connected || c.conn == nil {
		return nil
	}

	logger.Info.Println("Disconnecting from socket...")

	err := c.conn.Close()
	c.conn = nil
	c.connected = false

	if err != nil {
		logger.Error.Printf("Error disconnecting from socket: %v", err)
	} else {
		logger.Info.Println("Successfully disconnected from socket")
	}

	return err
}

func (c *Client) IsConnected() bool {
	return c.connected && c.conn != nil
}

func (c *Client) generateRequestID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func (c *Client) SendDownloadRequest(url, requestedBy string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}

	requestID := c.generateRequestID()

	request := DownloadRequest{
		Command: "download_audio",
		ID:      requestID,
		Params: map[string]interface{}{
			"url": url,
		},
	}

	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	err = c.sendMessage(data)
	if err != nil {
		c.connected = false
		return fmt.Errorf("failed to send request: %w", err)
	}

	return nil
}

func (c *Client) SendPlaylistRequest(url, requestedBy string, limit int) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}

	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	requestID := c.generateRequestID()

	request := DownloadRequest{
		Command: "start_playlist_download",
		ID:      requestID,
		Params: map[string]interface{}{
			"url":       url,
			"requester": requestedBy,
			"max_items": limit,
		},
	}

	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	err = c.sendMessage(data)
	if err != nil {
		c.connected = false
		return fmt.Errorf("failed to send request: %w", err)
	}

	return nil
}

func (c *Client) SendSearchRequest(query string, platform string, limit int) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}

	request := SearchRequest{
		Command: "search",
		ID:      c.generateRequestID(),
		Params: map[string]interface{}{
			"query":    query,
			"platform": platform,
			"limit":    limit,
		},
	}

	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	err = c.sendMessage(data)
	if err != nil {
		c.connected = false
		return fmt.Errorf("failed to send request: %w", err)
	}

	return nil
}

func (c *Client) sendMessage(data []byte) error {
	if len(data) > 50*1024*1024 {
		return fmt.Errorf("message too large: %d bytes", len(data))
	}

	messageLen := uint32(len(data))
	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, messageLen)

	c.conn.SetWriteDeadline(time.Now().Add(30 * time.Second))

	_, err := c.conn.Write(lengthBuf)
	if err != nil {
		return fmt.Errorf("failed to write length: %w", err)
	}

	_, err = c.conn.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	return nil
}

func (c *Client) readMessage() ([]byte, error) {
	c.conn.SetReadDeadline(time.Now().Add(5 * time.Minute))

	lengthBuf := make([]byte, 4)
	_, err := io.ReadFull(c.conn, lengthBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to read length: %w", err)
	}

	messageLen := binary.BigEndian.Uint32(lengthBuf)

	if messageLen == 0 {
		return nil, fmt.Errorf("received zero-length message")
	}

	if messageLen > 100*1024*1024 {
		return nil, fmt.Errorf("message too large: %d bytes (likely protocol error)", messageLen)
	}

	messageBuf := make([]byte, messageLen)

	totalRead := 0
	for totalRead < int(messageLen) {
		n, err := c.conn.Read(messageBuf[totalRead:])
		if err != nil {
			return nil, fmt.Errorf("failed to read message data at offset %d: %w", totalRead, err)
		}
		totalRead += n
	}

	return messageBuf, nil
}

func (c *Client) listenForResponses() {
	defer func() {
		if r := recover(); r != nil {
			logger.Error.Printf("Socket listener panic: %v", r)
		}
		if c.connected {
			c.connected = false
			logger.Info.Println("Socket listener stopped")
		}
	}()

	for c.connected {
		data, err := c.readMessage()
		if err != nil {
			if c.connected {
				logger.Error.Printf("Socket read error: %v", err)
				c.connected = false
			}
			return
		}

		if len(data) == 0 {
			logger.Error.Println("Received empty message")
			continue
		}

		go c.handleResponse(data)
	}
}

type DownloadResponse struct {
	Type      string                 `json:"type"`
	Status    string                 `json:"status"`
	ID        string                 `json:"id"`
	Event     string                 `json:"event,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Timestamp string                 `json:"timestamp,omitempty"`
}

type SearchResult struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	Duration  int    `json:"duration"`
	Uploader  string `json:"uploader"`
	Thumbnail string `json:"thumbnail"`
	Platform  string `json:"platform"`
}

func (c *Client) handleResponse(data []byte) {
	var response DownloadResponse
	err := json.Unmarshal(data, &response)
	if err != nil {
		logger.Error.Printf("Failed to unmarshal response (length: %d): %v", len(data), err)
		if len(data) > 0 && len(data) < 200 {
			logger.Error.Printf("Response data preview: %q", string(data))
		}
		return
	}

	if response.Type == "response" {
		if response.Status == "success" {
			c.handleSuccessResponse(response)
		} else if response.Status == "error" {
			logger.Error.Printf("Download request failed: %s", response.Error)
			if c.downloadHandler != nil {
				c.downloadHandler(nil)
			}
		}
	} else if response.Type == "event" {
		c.handleEventResponse(response)
	} else {
		logger.Error.Printf("Unknown response type: %s", response.Type)
	}
}

func (c *Client) handleSuccessResponse(response DownloadResponse) {
	data := response.Data
	if data == nil {
		return
	}

	if title, hasTitle := data["title"].(string); hasTitle {
		song := &state.Song{
			ID:           int64(getInt(data, "id")),
			Title:        title,
			URL:          getString(data, "url"),
			Platform:     getString(data, "platform"),
			FilePath:     getString(data, "filename"),
			Duration:     getInt(data, "duration"),
			FileSize:     int64(getInt(data, "file_size")),
			ThumbnailURL: getString(data, "thumbnail_url"),
			Artist:       getString(data, "artist"),
			IsStream:     getBool(data, "is_stream"),
		}

		if c.downloadHandler != nil {
			c.downloadHandler(song)
		}
	}

	if results, hasResults := data["results"].([]interface{}); hasResults {
		searchResults := make([]SearchResult, 0)
		for _, result := range results {
			if resultMap, ok := result.(map[string]interface{}); ok {
				searchResult := SearchResult{
					Title:     getString(resultMap, "title"),
					URL:       getString(resultMap, "url"),
					Duration:  getInt(resultMap, "duration"),
					Uploader:  getString(resultMap, "uploader"),
					Thumbnail: getString(resultMap, "thumbnail"),
					Platform:  getString(resultMap, "platform"),
				}
				searchResults = append(searchResults, searchResult)
			}
		}

		if c.searchHandler != nil {
			c.searchHandler(searchResults)
		}
	}

	if items, hasItems := data["items"].([]interface{}); hasItems {
		songs := make([]state.Song, 0)
		for _, item := range items {
			if itemMap, ok := item.(map[string]interface{}); ok {
				song := state.Song{
					ID:           int64(getInt(itemMap, "id")),
					Title:        getString(itemMap, "title"),
					URL:          getString(itemMap, "url"),
					Platform:     getString(itemMap, "platform"),
					FilePath:     getString(itemMap, "filename"),
					Duration:     getInt(itemMap, "duration"),
					FileSize:     int64(getInt(itemMap, "file_size")),
					ThumbnailURL: getString(itemMap, "thumbnail_url"),
					Artist:       getString(itemMap, "artist"),
					IsStream:     getBool(itemMap, "is_stream"),
				}
				songs = append(songs, song)
			}
		}

		if c.playlistHandler != nil {
			c.playlistHandler(songs)
		}
	}

	if playlistID, hasPlaylistID := data["playlist_id"].(string); hasPlaylistID {
		totalTracks := getInt(data, "total_tracks")
		logger.Info.Printf("Started async playlist download: %s with %d tracks", playlistID, totalTracks)

		if c.playlistStartHandler != nil && totalTracks > 0 {
			c.playlistStartHandler(totalTracks)
		}
	}
}

func (c *Client) handleEventResponse(response DownloadResponse) {
	if response.Event == "playlist_item_downloaded" && response.Data != nil {
		data := response.Data

		if trackData, hasTrack := data["track"].(map[string]interface{}); hasTrack {
			song := &state.Song{
				ID:           int64(getInt(trackData, "id")),
				Title:        getString(trackData, "title"),
				URL:          getString(trackData, "url"),
				Platform:     getString(trackData, "platform"),
				FilePath:     getString(trackData, "filename"),
				Duration:     getInt(trackData, "duration"),
				FileSize:     int64(getInt(trackData, "file_size")),
				ThumbnailURL: getString(trackData, "thumbnail_url"),
				Artist:       getString(trackData, "artist"),
				IsStream:     getBool(trackData, "is_stream"),
			}

			var playlistID string
			if playlistData, hasPlaylist := data["playlist"].(map[string]interface{}); hasPlaylist {
				playlistID = getString(playlistData, "url")
			}

			if c.playlistEventHandler != nil {
				c.playlistEventHandler(playlistID, song)
			} else if c.downloadHandler != nil {
				c.downloadHandler(song)
			}
		}
	} else {
		logger.Info.Printf("Received event: %s", response.Event)
	}
}

func getString(data map[string]interface{}, key string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	return ""
}

func getInt(data map[string]interface{}, key string) int {
	if val, ok := data[key].(float64); ok {
		return int(val)
	}
	return 0
}

func getBool(data map[string]interface{}, key string) bool {
	if val, ok := data[key].(bool); ok {
		return val
	}
	return false
}

func (c *Client) Ping() error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}

	request := map[string]interface{}{
		"command": "ping",
		"id":      c.generateRequestID(),
		"params":  map[string]interface{}{},
	}

	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal ping request: %w", err)
	}

	err = c.sendMessage(data)
	if err != nil {
		c.connected = false
		return err
	}

	return nil
}

func (c *Client) Shutdown(ctx context.Context) error {
	logger.Info.Println("Shutting down socket client...")
	return c.Disconnect()
}

func (c *Client) Name() string {
	return "SocketClient"
}
