package uds

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"time"

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
		timeout:    30 * time.Second,
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
	
	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}
	
	conn, err := net.DialTimeout("unix", c.socketPath, c.timeout)
	if err != nil {
		return nil, fmt.Errorf("error connecting to socket: %w", err)
	}
	defer conn.Close()
	
	conn.SetDeadline(time.Now().Add(c.timeout))
	
	messageLen := uint32(len(jsonData))
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, messageLen)
	
	if _, err := conn.Write(lenBuf); err != nil {
		return nil, fmt.Errorf("error sending message length: %w", err)
	}
	
	if _, err := conn.Write(jsonData); err != nil {
		return nil, fmt.Errorf("error sending message data: %w", err)
	}
	
	header := make([]byte, 4)
	if _, err := conn.Read(header); err != nil {
		return nil, fmt.Errorf("error reading response header: %w", err)
	}
	
	responseLen := binary.BigEndian.Uint32(header)
	
	responseBuf := make([]byte, responseLen)
	bytesRead := 0
	
	for bytesRead < int(responseLen) {
		n, err := conn.Read(responseBuf[bytesRead:])
		if err != nil {
			return nil, fmt.Errorf("error reading response data: %w", err)
		}
		bytesRead += n
	}
	
	var response Response
	if err := json.Unmarshal(responseBuf, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}
	
	if response.Status == "error" {
		return &response, errors.New(response.Error)
	}
	
	return &response, nil
}

func (c *Client) Ping() error {
	params := map[string]string{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	
	response, err := c.SendRequest("ping", params)
	if err != nil {
		return err
	}
	
	logger.DebugLogger.Printf("Ping response: %+v", response.Data)
	return nil
}

func (c *Client) DownloadAudio(url string, maxDuration int, maxSize int, allowLive bool) (map[string]interface{}, error) {
	params := map[string]interface{}{
		"url":                  url,
		"max_duration_seconds": maxDuration,
		"max_size_mb":          maxSize,
		"allow_live":           allowLive,
	}
	
	response, err := c.SendRequest("download_audio", params)
	if err != nil {
		return nil, err
	}
	
	return response.Data, nil
}

func (c *Client) DownloadPlaylist(url string, maxItems int, maxDuration int, maxSize int, allowLive bool) (map[string]interface{}, error) {
	params := map[string]interface{}{
		"url":                  url,
		"max_items":            maxItems,
		"max_duration_seconds": maxDuration,
		"max_size_mb":          maxSize,
		"allow_live":           allowLive,
	}
	
	response, err := c.SendRequest("download_playlist", params)
	if err != nil {
		return nil, err
	}
	
	return response.Data, nil
}

func (c *Client) Search(query string, platform string, limit int, includeLive bool) ([]map[string]interface{}, error) {
	params := map[string]interface{}{
		"query":        query,
		"platform":     platform,
		"limit":        limit,
		"include_live": includeLive,
	}
	
	response, err := c.SendRequest("search", params)
	if err != nil {
		return nil, err
	}
	
	if results, ok := response.Data["results"].([]interface{}); ok {
		typedResults := make([]map[string]interface{}, 0, len(results))
		
		for _, result := range results {
			if mapResult, ok := result.(map[string]interface{}); ok {
				typedResults = append(typedResults, mapResult)
			}
		}
		
		return typedResults, nil
	}
	
	return []map[string]interface{}{}, nil
}