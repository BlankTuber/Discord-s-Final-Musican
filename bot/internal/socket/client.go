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
	socketPath      string
	conn            net.Conn
	connected       bool
	downloadHandler func(*state.Song, string)  // Modified to include requestedBy
	playlistHandler func([]state.Song, string) // Modified to include requestedBy
	searchHandler   func([]SearchResult)
	pendingRequests map[string]string // maps request ID to requester ID
	mu              sync.RWMutex
}

func NewClient(socketPath string) *Client {
	return &Client{
		socketPath:      socketPath,
		pendingRequests: make(map[string]string),
	}
}

func (c *Client) SetDownloadHandler(handler func(*state.Song, string)) {
	c.downloadHandler = handler
}

func (c *Client) SetPlaylistHandler(handler func([]state.Song, string)) {
	c.playlistHandler = handler
}

func (c *Client) SetSearchHandler(handler func([]SearchResult)) {
	c.searchHandler = handler
}

func (c *Client) Connect() error {
	logger.Info.Printf("Connecting to socket: %s", c.socketPath)

	conn, err := net.DialTimeout("unix", c.socketPath, 10*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to socket: %w", err)
	}

	c.conn = conn
	c.connected = true

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

	c.mu.Lock()
	c.pendingRequests[requestID] = requestedBy
	c.mu.Unlock()

	request := DownloadRequest{
		Command: "download_audio",
		ID:      requestID,
		Params: map[string]interface{}{
			"url": url,
		},
	}

	data, err := json.Marshal(request)
	if err != nil {
		c.mu.Lock()
		delete(c.pendingRequests, requestID)
		c.mu.Unlock()
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	err = c.sendMessage(data)
	if err != nil {
		c.connected = false
		c.mu.Lock()
		delete(c.pendingRequests, requestID)
		c.mu.Unlock()
		return fmt.Errorf("failed to send request: %w", err)
	}

	return nil
}

func (c *Client) SendPlaylistRequest(url, requestedBy string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}

	requestID := c.generateRequestID()

	c.mu.Lock()
	c.pendingRequests[requestID] = requestedBy
	c.mu.Unlock()

	request := DownloadRequest{
		Command: "download_playlist",
		ID:      requestID,
		Params: map[string]interface{}{
			"url": url,
		},
	}

	data, err := json.Marshal(request)
	if err != nil {
		c.mu.Lock()
		delete(c.pendingRequests, requestID)
		c.mu.Unlock()
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	err = c.sendMessage(data)
	if err != nil {
		c.connected = false
		c.mu.Lock()
		delete(c.pendingRequests, requestID)
		c.mu.Unlock()
		return fmt.Errorf("failed to send request: %w", err)
	}

	return nil
}

func (c *Client) SendSearchRequest(query string, limit int) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}

	request := SearchRequest{
		Command: "search",
		ID:      c.generateRequestID(),
		Params: map[string]interface{}{
			"query": query,
			"limit": limit,
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
	messageLen := uint32(len(data))

	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, messageLen)

	_, err := c.conn.Write(lengthBuf)
	if err != nil {
		return err
	}

	_, err = c.conn.Write(data)
	return err
}

func (c *Client) readMessage() ([]byte, error) {
	lengthBuf := make([]byte, 4)
	_, err := io.ReadFull(c.conn, lengthBuf)
	if err != nil {
		return nil, err
	}

	messageLen := binary.BigEndian.Uint32(lengthBuf)
	if messageLen > 10*1024*1024 { // 10MB limit
		return nil, fmt.Errorf("message too large: %d bytes", messageLen)
	}

	messageBuf := make([]byte, messageLen)
	_, err = io.ReadFull(c.conn, messageBuf)
	if err != nil {
		return nil, err
	}

	return messageBuf, nil
}

func (c *Client) listenForResponses() {
	defer func() {
		if r := recover(); r != nil {
			logger.Error.Printf("Socket listener panic: %v", r)
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

		c.handleResponse(data)
	}
}

type DownloadResponse struct {
	Type      string                 `json:"type"`
	Status    string                 `json:"status"`
	ID        string                 `json:"id"`
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
		logger.Error.Printf("Failed to unmarshal response: %v", err)
		return
	}

	if response.Type == "response" {
		if response.Status == "success" {
			c.handleSuccessResponse(response)
		} else if response.Status == "error" {
			logger.Error.Printf("Download request failed: %s", response.Error)
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

	// Get the requester for this response
	c.mu.Lock()
	requestedBy := c.pendingRequests[response.ID]
	delete(c.pendingRequests, response.ID)
	c.mu.Unlock()

	// Check if this is a download response
	if title, hasTitle := data["title"].(string); hasTitle {
		song := &state.Song{
			ID:          int64(getInt(data, "id")),
			Title:       title,
			FilePath:    getString(data, "filename"),
			Duration:    getInt(data, "duration"),
			Artist:      getString(data, "artist"),
			URL:         getString(data, "url"),
			RequestedBy: requestedBy,
			AddedAt:     time.Now(),
		}

		if c.downloadHandler != nil {
			c.downloadHandler(song, requestedBy)
		}
	}

	// Check if this is a search response
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

	// Check if this is a playlist response
	if items, hasItems := data["items"].([]interface{}); hasItems {
		songs := make([]state.Song, 0)
		for _, item := range items {
			if itemMap, ok := item.(map[string]interface{}); ok {
				song := state.Song{
					ID:          int64(getInt(itemMap, "id")),
					Title:       getString(itemMap, "title"),
					FilePath:    getString(itemMap, "filename"),
					Duration:    getInt(itemMap, "duration"),
					Artist:      getString(itemMap, "artist"),
					URL:         getString(itemMap, "url"),
					RequestedBy: requestedBy,
					AddedAt:     time.Now(),
				}
				songs = append(songs, song)
			}
		}

		if c.playlistHandler != nil {
			c.playlistHandler(songs, requestedBy)
		}
	}
}

func (c *Client) handleEventResponse(response DownloadResponse) {
	// Handle event responses from the downloader
	logger.Info.Printf("Received event: %s", response.ID)
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
