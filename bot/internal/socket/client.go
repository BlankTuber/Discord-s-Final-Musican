package socket

import (
	"context"
	"fmt"
	"musicbot/internal/logger"
	"net"
	"time"
)

type Client struct {
	socketPath string
	conn       net.Conn
	connected  bool
}

func NewClient(socketPath string) *Client {
	return &Client{
		socketPath: socketPath,
	}
}

func (c *Client) Connect() error {
	logger.Info.Printf("Connecting to socket: %s", c.socketPath)

	conn, err := net.DialTimeout("unix", c.socketPath, 10*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to socket: %w", err)
	}

	c.conn = conn
	c.connected = true

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

func (c *Client) Ping() error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected")
	}

	_, err := c.conn.Write([]byte("ping"))
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
