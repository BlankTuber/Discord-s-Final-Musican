package downloader

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"sync"
	"time"

	"quidque.com/discord-musican/internal/audio"
	"quidque.com/discord-musican/internal/logger"
)

type MessageType string

const (
	MessageTypeRequest  MessageType = "request"
	MessageTypeResponse MessageType = "response"
	MessageTypeEvent    MessageType = "event"
)

type Message struct {
	Type      MessageType    `json:"type"`
	Command   string         `json:"command"`
	Event     string         `json:"event,omitempty"` // Add this field for events
	ID        string         `json:"id"`
	Params    map[string]any `json:"params,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	Error     string         `json:"error,omitempty"`
	Timestamp string         `json:"timestamp,omitempty"`
}

type EventCallback func(eventType string, data map[string]any)

type Client struct {
	socketPath    string
	conn          net.Conn
	connected     bool
	reconnecting  bool
	mu            sync.Mutex
	reqMu         sync.Mutex
	connMu        sync.RWMutex
	pendingReqs   map[string]chan *Message
	eventCallback EventCallback
	stopChan      chan struct{}
	timeout       time.Duration
}

func NewClient(socketPath string) *Client {
	return &Client{
		socketPath:  socketPath,
		pendingReqs: make(map[string]chan *Message),
		stopChan:    make(chan struct{}),
		timeout:     300 * time.Second,
	}
}

func (c *Client) SetTimeout(timeout time.Duration) {
	c.timeout = timeout
}

func (c *Client) SetEventCallback(callback EventCallback) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.eventCallback = callback
}

func (c *Client) Connect() error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.connected {
		return nil
	}

	logger.InfoLogger.Printf("Connecting to downloader service at %s", c.socketPath)
	conn, err := net.DialTimeout("unix", c.socketPath, 10*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to downloader service: %w", err)
	}

	c.conn = conn
	c.connected = true

	go c.readLoop()

	logger.InfoLogger.Println("Successfully connected to downloader service")
	return nil
}

func (c *Client) IsConnected() bool {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.connected
}

func (c *Client) reconnect() {
	c.connMu.Lock()

	if c.reconnecting {
		c.connMu.Unlock()
		return
	}

	c.reconnecting = true
	c.connMu.Unlock()

	logger.InfoLogger.Println("Attempting to reconnect to downloader service...")

	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		c.connMu.Lock()
		if c.conn != nil {
			c.conn.Close()
			c.conn = nil
		}
		c.connected = false
		c.connMu.Unlock()

		time.Sleep(time.Duration(i+1) * time.Second)

		err := c.Connect()
		if err == nil {
			c.connMu.Lock()
			c.reconnecting = false
			c.connMu.Unlock()
			logger.InfoLogger.Println("Reconnected to downloader service")
			return
		}

		logger.ErrorLogger.Printf("Reconnection attempt %d/%d failed: %v", i+1, maxRetries, err)
	}

	c.connMu.Lock()
	c.reconnecting = false
	c.connMu.Unlock()
	logger.ErrorLogger.Printf("Failed to reconnect to downloader service after %d attempts", maxRetries)
}

func (c *Client) Disconnect() error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if !c.connected {
		return nil
	}

	close(c.stopChan)

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		c.connected = false
		return err
	}

	return nil
}

func (c *Client) readLoop() {
	defer func() {
		c.connMu.Lock()
		c.connected = false
		if c.conn != nil {
			c.conn.Close()
			c.conn = nil
		}
		c.connMu.Unlock()
	}()

	for {
		select {
		case <-c.stopChan:
			return
		default:
			c.connMu.RLock()
			conn := c.conn
			c.connMu.RUnlock()

			if conn == nil {
				logger.ErrorLogger.Println("Connection is nil in read loop, triggering reconnect")
				go c.reconnect()
				return
			}

			err := conn.SetReadDeadline(time.Now().Add(c.timeout))
			if err != nil {
				logger.ErrorLogger.Printf("Failed to set read deadline: %v", err)
				go c.reconnect()
				return
			}

			headerBuf := make([]byte, 4)
			_, err = io.ReadFull(conn, headerBuf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				logger.ErrorLogger.Printf("Failed to read message header: %v", err)
				go c.reconnect()
				return
			}

			messageLen := binary.BigEndian.Uint32(headerBuf)
			if messageLen > 100*1024*1024 {
				logger.ErrorLogger.Printf("Message size too large: %d bytes", messageLen)
				go c.reconnect()
				return
			}

			messageBuf := make([]byte, messageLen)
			_, err = io.ReadFull(conn, messageBuf)
			if err != nil {
				logger.ErrorLogger.Printf("Failed to read message body: %v", err)
				go c.reconnect()
				return
			}

			var message Message
			err = json.Unmarshal(messageBuf, &message)
			if err != nil {
				logger.ErrorLogger.Printf("Failed to unmarshal message: %v", err)
				continue
			}

			c.handleMessage(&message)
		}
	}
}

func (c *Client) handleMessage(message *Message) {
	switch message.Type {
	case MessageTypeResponse:
		c.mu.Lock()
		resChan, exists := c.pendingReqs[message.ID]
		if exists {
			delete(c.pendingReqs, message.ID)
		}
		c.mu.Unlock()

		if exists {
			resChan <- message
		} else {
			logger.WarnLogger.Printf("Received response for unknown request ID: %s", message.ID)
		}

	case MessageTypeEvent:
		c.mu.Lock()
		callback := c.eventCallback
		c.mu.Unlock()

		if callback != nil {
			go callback(message.Event, message.Data)
		} else {
			logger.InfoLogger.Printf("Received event %s but no callback registered", message.Event)
		}

	default:
		logger.WarnLogger.Printf("Received unknown message type: %s", message.Type)
	}
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

func (c *Client) SendRequest(command string, params map[string]any) (*Message, error) {
	c.reqMu.Lock()
	defer c.reqMu.Unlock()

	if !c.IsConnected() {
		err := c.Connect()
		if err != nil {
			return nil, fmt.Errorf("not connected to downloader service: %w", err)
		}
	}

	requestID := c.generateRequestID()

	request := Message{
		Type:      MessageTypeRequest,
		Command:   command,
		ID:        requestID,
		Params:    params,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	logger.InfoLogger.Printf("Sending request - Command: %s, ID: %s", command, requestID)

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	resChan := make(chan *Message, 1)
	c.mu.Lock()
	c.pendingReqs[requestID] = resChan
	c.mu.Unlock()

	c.connMu.RLock()
	conn := c.conn
	c.connMu.RUnlock()

	if conn == nil {
		c.mu.Lock()
		delete(c.pendingReqs, requestID)
		c.mu.Unlock()
		return nil, errors.New("connection is nil")
	}

	err = conn.SetWriteDeadline(time.Now().Add(c.timeout))
	if err != nil {
		c.mu.Lock()
		delete(c.pendingReqs, requestID)
		c.mu.Unlock()
		return nil, fmt.Errorf("failed to set write deadline: %w", err)
	}

	messageLen := uint32(len(jsonData))
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, messageLen)

	logger.InfoLogger.Printf("Sending message header (%d bytes)", len(lenBuf))
	_, err = conn.Write(lenBuf)
	if err != nil {
		c.mu.Lock()
		delete(c.pendingReqs, requestID)
		c.mu.Unlock()
		go c.reconnect()
		return nil, fmt.Errorf("error sending message length: %w", err)
	}

	logger.InfoLogger.Printf("Sending message body (%d bytes)", len(jsonData))
	_, err = conn.Write(jsonData)
	if err != nil {
		c.mu.Lock()
		delete(c.pendingReqs, requestID)
		c.mu.Unlock()
		go c.reconnect()
		return nil, fmt.Errorf("error sending message data: %w", err)
	}

	select {
	case response := <-resChan:
		return response, nil
	case <-time.After(c.timeout):
		c.mu.Lock()
		delete(c.pendingReqs, requestID)
		c.mu.Unlock()
		return nil, fmt.Errorf("request timed out after %v", c.timeout)
	}
}

func (c *Client) Ping() error {
	params := map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	response, err := c.SendRequest("ping", params)
	if err != nil {
		return err
	}

	if response.Error != "" {
		return errors.New(response.Error)
	}

	return nil
}

func (c *Client) DownloadAudio(url string, maxDuration int, maxSize int, allowLive bool) (*audio.Track, error) {
	params := map[string]any{
		"url":                  url,
		"max_duration_seconds": maxDuration,
		"max_size_mb":          maxSize,
		"allow_live":           allowLive,
	}

	response, err := c.SendRequest("download_audio", params)
	if err != nil {
		return nil, err
	}

	if response.Error != "" {
		return nil, errors.New(response.Error)
	}

	// Check if response contains an error or status message
	if status, ok := response.Data["status"].(string); ok && status == "error" {
		errorMsg := "Download failed"
		if msg, ok := response.Data["message"].(string); ok && msg != "" {
			errorMsg = msg
		}
		return nil, errors.New(errorMsg)
	}

	// Check if we got a valid title and filename
	title, titleOk := response.Data["title"].(string)
	filename, filenameOk := response.Data["filename"].(string)

	if !titleOk || title == "" || !filenameOk || filename == "" {
		// Check if we can get a more specific error message
		if skipped, ok := response.Data["skipped"].(bool); ok && skipped {
			return nil, errors.New("download was skipped, possibly due to size or duration limits")
		}
		return nil, errors.New("download failed - received invalid track data")
	}

	// File existence check
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return nil, errors.New("download completed but file is missing")
	}

	track := &audio.Track{
		URL:            url,
		Title:          title,
		FilePath:       filename,
		RequestedAt:    time.Now().Unix(),
		DownloadStatus: "completed",
	}

	if duration, ok := response.Data["duration"].(float64); ok {
		track.Duration = int(duration)
	}

	if artist, ok := response.Data["artist"].(string); ok {
		track.ArtistName = artist
	}

	if thumbnail, ok := response.Data["thumbnail_url"].(string); ok {
		track.ThumbnailURL = thumbnail
	}

	if isStream, ok := response.Data["is_stream"].(bool); ok {
		track.IsStream = isStream
	}

	return track, nil
}

func (c *Client) StartPlaylistDownload(url string, maxItems int, maxDuration int, maxSize int, allowLive bool) (string, int, error) {
	params := map[string]any{
		"url":                  url,
		"max_items":            maxItems,
		"max_duration_seconds": maxDuration,
		"max_size_mb":          maxSize,
		"allow_live":           allowLive,
	}

	response, err := c.SendRequest("start_playlist_download", params)
	if err != nil {
		return "", 0, err
	}

	if response.Error != "" {
		return "", 0, errors.New(response.Error)
	}

	var playlistID string
	var totalTracks int

	if id, ok := response.Data["playlist_id"].(string); ok {
		playlistID = id
	} else {
		return "", 0, errors.New("playlist ID not returned")
	}

	if tracks, ok := response.Data["total_tracks"].(float64); ok {
		totalTracks = int(tracks)
	}

	return playlistID, totalTracks, nil
}

func (c *Client) GetPlaylistDownloadStatus(playlistID string) (map[string]any, error) {
	params := map[string]any{
		"playlist_id": playlistID,
	}

	response, err := c.SendRequest("get_playlist_download_status", params)
	if err != nil {
		return nil, err
	}

	if response.Error != "" {
		return nil, errors.New(response.Error)
	}

	return response.Data, nil
}

func (c *Client) GetPlaylistInfo(url string, maxItems int) (string, int, error) {
	params := map[string]any{
		"url":       url,
		"max_items": maxItems,
	}

	response, err := c.SendRequest("get_playlist_info", params)
	if err != nil {
		return "", 0, err
	}

	if response.Error != "" {
		return "", 0, errors.New(response.Error)
	}

	var playlistTitle string
	var totalTracks int

	if title, ok := response.Data["playlist_title"].(string); ok {
		playlistTitle = title
	} else {
		playlistTitle = "Unknown Playlist"
	}

	if tracks, ok := response.Data["total_tracks"].(float64); ok {
		totalTracks = int(tracks)
		if totalTracks > maxItems {
			totalTracks = maxItems
		}
	} else {
		totalTracks = maxItems
	}

	return playlistTitle, totalTracks, nil
}

func (c *Client) DownloadPlaylist(url string, maxItems int, maxDuration int, maxSize int, allowLive bool, requester string, guildID string) error {
	params := map[string]any{
		"url":                  url,
		"max_items":            maxItems,
		"max_duration_seconds": maxDuration,
		"max_size_mb":          maxSize,
		"allow_live":           allowLive,
		"requester":            requester,
		"guild_id":             guildID,
	}

	response, err := c.SendRequest("download_playlist", params)
	if err != nil {
		return err
	}

	if response.Error != "" {
		return errors.New(response.Error)
	}

	return nil
}

func (c *Client) DownloadPlaylistItem(url string, index int, maxDuration int, maxSize int, allowLive bool) (*audio.Track, error) {
	params := map[string]any{
		"url":                  url,
		"index":                index,
		"max_duration_seconds": maxDuration,
		"max_size_mb":          maxSize,
		"allow_live":           allowLive,
	}

	response, err := c.SendRequest("download_playlist_item", params)
	if err != nil {
		return nil, err
	}

	if response.Error != "" {
		return nil, errors.New(response.Error)
	}

	track := &audio.Track{
		URL:            url,
		RequestedAt:    time.Now().Unix(),
		Position:       index,
		DownloadStatus: "completed",
	}

	if title, ok := response.Data["title"].(string); ok {
		track.Title = title
	} else {
		track.Title = fmt.Sprintf("Unknown Track %d", index+1)
	}

	if filePath, ok := response.Data["filename"].(string); ok {
		track.FilePath = filePath
	}

	if duration, ok := response.Data["duration"].(float64); ok {
		track.Duration = int(duration)
	}

	if artist, ok := response.Data["artist"].(string); ok {
		track.ArtistName = artist
	}

	if thumbnail, ok := response.Data["thumbnail_url"].(string); ok {
		track.ThumbnailURL = thumbnail
	}

	if isStream, ok := response.Data["is_stream"].(bool); ok {
		track.IsStream = isStream
	}

	return track, nil
}

func (c *Client) Search(query string, platform string, limit int, includeLive bool) ([]*audio.Track, error) {
	params := map[string]any{
		"query":        query,
		"platform":     platform,
		"limit":        limit,
		"include_live": includeLive,
	}

	response, err := c.SendRequest("search", params)
	if err != nil {
		return nil, err
	}

	if response.Error != "" {
		return nil, errors.New(response.Error)
	}

	var results []any
	var tracks []*audio.Track

	if data, ok := response.Data["results"].([]any); ok {
		results = data
	} else {
		return []*audio.Track{}, nil
	}

	tracks = make([]*audio.Track, 0, len(results))

	for _, result := range results {
		if mapResult, ok := result.(map[string]any); ok {
			track := &audio.Track{
				RequestedAt:    time.Now().Unix(),
				DownloadStatus: "pending",
			}

			if title, ok := mapResult["title"].(string); ok {
				track.Title = title
			}

			if url, ok := mapResult["url"].(string); ok {
				track.URL = url
			}

			if duration, ok := mapResult["duration"].(float64); ok {
				track.Duration = int(duration)
			}

			if thumbnail, ok := mapResult["thumbnail"].(string); ok {
				track.ThumbnailURL = thumbnail
			}

			if uploader, ok := mapResult["uploader"].(string); ok {
				track.ArtistName = uploader
			}

			tracks = append(tracks, track)
		}
	}

	return tracks, nil
}
