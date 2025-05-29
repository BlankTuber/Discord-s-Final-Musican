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
	lastDownloaderPing   time.Time
	pingTicker           *time.Ticker
	stopPing             chan struct{}
	reconnectAttempts    int
	maxReconnectAttempts int
}

func NewClient(socketPath string) *Client {
	return &Client{
		socketPath:           socketPath,
		pendingRequests:      make(map[string]chan interface{}),
		stopPing:             make(chan struct{}),
		maxReconnectAttempts: 5,
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

	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.lastDownloaderPing = time.Now()
	c.reconnectAttempts = 0
	c.mu.Unlock()

	if c.resetPendingHandler != nil {
		c.resetPendingHandler()
	}

	go c.listenForResponses()
	c.startKeepaliveRoutine()

	logger.Info.Println("Successfully connected to socket")
	return nil
}

func (c *Client) startKeepaliveRoutine() {
	c.mu.Lock()
	if c.pingTicker != nil {
		c.pingTicker.Stop()
	}
	c.pingTicker = time.NewTicker(90 * time.Second) // Ping every 90 seconds
	c.mu.Unlock()

	go func() {
		defer func() {
			c.mu.Lock()
			if c.pingTicker != nil {
				c.pingTicker.Stop()
				c.pingTicker = nil
			}
			c.mu.Unlock()
		}()

		for {
			select {
			case <-c.pingTicker.C:
				if !c.IsConnected() {
					logger.Info.Println("Keepalive: Not connected to downloader, stopping keepalive")
					return
				}

				err := c.sendKeepalivePing()
				if err != nil {
					logger.Error.Printf("Keepalive ping failed: %v", err)
					c.handleConnectionError(err)
					return
				}

			case <-c.stopPing:
				logger.Debug.Println("Keepalive routine stopped")
				return
			}
		}
	}()
}

func (c *Client) sendKeepalivePing() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	requestID := c.generateRequestID()
	request := map[string]interface{}{
		"command": "ping",
		"id":      requestID,
		"params": map[string]interface{}{
			"timestamp": time.Now().Format(time.RFC3339),
			"keepalive": true,
		},
	}

	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal keepalive ping: %w", err)
	}

	responseChan := make(chan interface{}, 1)
	c.mu.Lock()
	c.pendingRequests[requestID] = responseChan
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pendingRequests, requestID)
		c.mu.Unlock()
	}()

	err = c.sendMessage(data)
	if err != nil {
		return fmt.Errorf("failed to send keepalive ping: %w", err)
	}

	select {
	case response := <-responseChan:
		if responseData, ok := response.(map[string]interface{}); ok {
			if msg, exists := responseData["message"].(string); exists && msg == "pong" {
				c.mu.Lock()
				c.lastDownloaderPing = time.Now()
				c.mu.Unlock()
				logger.Debug.Println("Keepalive pong received")
				return nil
			}
		}
		return fmt.Errorf("unexpected keepalive response format")

	case <-ctx.Done():
		return fmt.Errorf("keepalive ping timeout")
	}
}

func (c *Client) handleConnectionError(err error) {
	c.mu.Lock()
	wasConnected := c.connected
	c.connected = false
	c.mu.Unlock()

	if !wasConnected {
		return // Already handling disconnection
	}

	logger.Error.Printf("Connection error detected: %v", err)

	// Stop keepalive routine
	select {
	case c.stopPing <- struct{}{}:
	default:
	}

	// Close current connection
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}

	// Attempt reconnection
	go c.attemptReconnection()
}

func (c *Client) attemptReconnection() {
	for attempt := 1; attempt <= c.maxReconnectAttempts; attempt++ {
		delay := time.Duration(attempt*attempt) * time.Second // Exponential backoff
		logger.Info.Printf("Attempting reconnection %d/%d in %v...", attempt, c.maxReconnectAttempts, delay)

		time.Sleep(delay)

		err := c.Connect()
		if err == nil {
			logger.Info.Printf("Reconnection successful after %d attempts", attempt)
			return
		}

		logger.Error.Printf("Reconnection attempt %d failed: %v", attempt, err)
	}

	logger.Error.Printf("Failed to reconnect after %d attempts", c.maxReconnectAttempts)
}

func (c *Client) Disconnect() error {
	c.mu.Lock()
	if !c.connected || c.conn == nil {
		c.mu.Unlock()
		return nil
	}
	c.connected = false
	c.mu.Unlock()

	logger.Info.Println("Disconnecting from socket...")

	// Stop keepalive routine
	select {
	case c.stopPing <- struct{}{}:
	default:
	}

	c.mu.Lock()
	if c.pingTicker != nil {
		c.pingTicker.Stop()
		c.pingTicker = nil
	}
	c.mu.Unlock()

	err := c.conn.Close()
	c.conn = nil

	if err != nil {
		logger.Error.Printf("Error disconnecting from socket: %v", err)
	} else {
		logger.Info.Println("Successfully disconnected from socket")
	}

	return err
}

func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
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
		c.handleConnectionError(err)
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
		c.handleConnectionError(err)
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
		c.handleConnectionError(err)
		return fmt.Errorf("failed to send request: %w", err)
	}

	return nil
}

func (c *Client) sendMessage(data []byte) error {
	if len(data) > 50*1024*1024 {
		return fmt.Errorf("message too large: %d bytes", len(data))
	}

	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("no connection available")
	}

	messageLen := uint32(len(data))
	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, messageLen)

	conn.SetWriteDeadline(time.Now().Add(30 * time.Second))

	_, err := conn.Write(lengthBuf)
	if err != nil {
		return fmt.Errorf("failed to write length: %w", err)
	}

	_, err = conn.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	return nil
}

func (c *Client) readMessage() ([]byte, error) {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("no connection available")
	}

	conn.SetReadDeadline(time.Now().Add(10 * time.Minute)) // Longer read timeout

	lengthBuf := make([]byte, 4)
	_, err := io.ReadFull(conn, lengthBuf)
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
		conn.SetReadDeadline(time.Now().Add(2 * time.Minute)) // Reset deadline for each chunk
		n, err := conn.Read(messageBuf[totalRead:])
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
		c.handleConnectionError(fmt.Errorf("listener stopped"))
	}()

	for c.IsConnected() {
		data, err := c.readMessage()
		if err != nil {
			if c.IsConnected() {
				logger.Error.Printf("Socket read error: %v", err)
				c.handleConnectionError(err)
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

	// Check if this is a response to a pending request
	if response.ID != "" {
		c.mu.Lock()
		if ch, ok := c.pendingRequests[response.ID]; ok {
			ch <- response.Data
			delete(c.pendingRequests, response.ID)
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()
	}

	if response.Status == "success" && response.ID != "" && response.Data != nil {
		if msg, ok := response.Data["message"].(string); ok && msg == "pong" {
			c.mu.Lock()
			c.lastDownloaderPing = time.Now()
			c.mu.Unlock()
			logger.Debug.Println("Received pong from downloader, updated lastDownloaderPing.")
		}
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
		c.handleConnectionError(err)
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

func (c *Client) SendPingWithResponse() (map[string]interface{}, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected to downloader")
	}

	requestID := c.generateRequestID()
	request := map[string]interface{}{
		"command": "ping",
		"id":      requestID,
		"params": map[string]interface{}{
			"timestamp": time.Now().Format(time.RFC3339),
		},
	}

	data, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ping request: %w", err)
	}

	responseChan := make(chan interface{})
	c.mu.Lock()
	c.pendingRequests[requestID] = responseChan
	c.mu.Unlock()

	err = c.sendMessage(data)
	if err != nil {
		c.mu.Lock()
		delete(c.pendingRequests, requestID)
		c.mu.Unlock()
		c.handleConnectionError(err)
		return nil, fmt.Errorf("failed to send ping request: %w", err)
	}

	select {
	case responseData := <-responseChan:
		if result, ok := responseData.(map[string]interface{}); ok {
			return result, nil
		}
		return nil, fmt.Errorf("unexpected response format for ping")
	case <-time.After(5 * time.Second):
		c.mu.Lock()
		delete(c.pendingRequests, requestID)
		c.mu.Unlock()
		return nil, fmt.Errorf("ping response timed out")
	}
}

func (c *Client) GetDownloaderStatus() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return "🔴 Disconnected"
	}

	if time.Since(c.lastDownloaderPing) > 3*time.Minute {
		return "🟠 Unresponsive (No pong in 3 minutes)"
	}

	return "🟢 Connected and Responsive"
}
