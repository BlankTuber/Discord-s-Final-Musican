package shutdown

import (
	"context"
	"musicbot/internal/logger"
	"sync"
	"time"
)

type Component interface {
	Shutdown(ctx context.Context) error
	Name() string
}

type StateManager interface {
	SetShuttingDown(bool)
}

type Manager struct {
	components   []Component
	stateManager StateManager
	mu           sync.RWMutex
	shutdown     chan struct{}
	done         chan struct{}
}

func NewManager() *Manager {
	return &Manager{
		components: make([]Component, 0),
		shutdown:   make(chan struct{}),
		done:       make(chan struct{}),
	}
}

func (m *Manager) SetStateManager(stateManager StateManager) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stateManager = stateManager
}

func (m *Manager) Register(component Component) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.components = append(m.components, component)
	logger.Info.Printf("Registered shutdown component: %s", component.Name())
}

func (m *Manager) Shutdown(timeout time.Duration) error {
	logger.Info.Println("Initiating graceful shutdown...")

	// Signal shutdown state immediately
	if m.stateManager != nil {
		m.stateManager.SetShuttingDown(true)
		logger.Debug.Println("Set shutdown state to prevent reconnections")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	close(m.shutdown)

	m.mu.RLock()
	components := make([]Component, len(m.components))
	copy(components, m.components)
	m.mu.RUnlock()

	var wg sync.WaitGroup
	errors := make(chan error, len(components))

	// Shutdown in reverse order (LIFO)
	for i := len(components) - 1; i >= 0; i-- {
		component := components[i]
		wg.Add(1)

		go func(comp Component) {
			defer wg.Done()
			logger.Info.Printf("Shutting down component: %s", comp.Name())

			if err := comp.Shutdown(ctx); err != nil {
				logger.Error.Printf("Error shutting down %s: %v", comp.Name(), err)
				errors <- err
			} else {
				logger.Info.Printf("Successfully shut down: %s", comp.Name())
			}
		}(component)

		// Small delay between component shutdowns
		time.Sleep(100 * time.Millisecond)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info.Println("All components shut down successfully")
		close(m.done)
		return nil
	case <-ctx.Done():
		logger.Error.Println("Shutdown timed out")
		close(m.done)
		return ctx.Err()
	}
}

func (m *Manager) IsShuttingDown() bool {
	select {
	case <-m.shutdown:
		return true
	default:
		return false
	}
}

func (m *Manager) Wait() {
	<-m.done
}
