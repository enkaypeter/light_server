package pool

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// Manager orchestrates a pool of warmed containers to mitigate cold-starts.
// Acquire blocks until a container is available or a new one is started.
// Release returns containers back into the pool or evicts them if expired.
// Shutdown stops the janitor and tears down all containers.
type Manager interface {
	Acquire(ctx context.Context, image string) (Container, error)
	Release(ctx context.Context, c Container) error
	Shutdown(ctx context.Context) error
}

// Container represents a single Docker container in the pool.
type Container interface {
	ID() string
	Exec(ctx context.Context, args []string) ([]byte, error)
	Close(ctx context.Context) error
}

// pooledContainer holds metadata for a container.
type pooledContainer struct {
	id        string
	createdAt time.Time
	lastUsed  time.Time
}

func (pc *pooledContainer) ID() string { return pc.id }

func (pc *pooledContainer) Exec(ctx context.Context, args []string) ([]byte, error) {
	// Delegate to `docker exec`
	cmd := exec.CommandContext(ctx, "docker", append([]string{"exec", pc.id}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("exec in container %s failed: %w", pc.id, err)
	}
	return output, nil
}

func (pc *pooledContainer) Close(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "rm", "-f", pc.id)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove container %s: %w", pc.id, err)
	}
	return nil
}

// warmPoolManager implements Manager with TTL, max-size, and a janitor.
// pool maps image names to slices of idle containers.
type warmPoolManager struct {
	ttl             time.Duration
	maxSize         int
	cleanupInterval time.Duration
	mu              sync.Mutex
	pool            map[string][]*pooledContainer
	stopJanitor     chan struct{}
}

// NewManager creates a warmPoolManager and starts its janitor.
func NewManager(ttl time.Duration, maxSize int) Manager {
	m := &warmPoolManager{
		ttl:             ttl,
		maxSize:         maxSize,
		cleanupInterval: time.Minute,
		pool:            make(map[string][]*pooledContainer),
		stopJanitor:     make(chan struct{}),
	}
	go m.janitor()
	return m
}

// Acquire returns an idle container or starts a new one if needed.
func (m *warmPoolManager) Acquire(ctx context.Context, image string) (Container, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Reuse an idle container
	if list := m.pool[image]; len(list) > 0 {
		c := list[0]
		m.pool[image] = list[1:]
		c.lastUsed = time.Now()
		return c, nil
	}

	// Evict oldest if at capacity
	if len(m.pool[image]) >= m.maxSize {
		evict := m.pool[image][0]
		m.pool[image] = m.pool[image][1:]
		evict.Close(ctx)
	}

	// Cold start: launch a new container
	pc, err := m.startNew(ctx, image)
	if err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}
	return pc, nil
}

// Release returns a container to the pool or evicts it if expired.
func (m *warmPoolManager) Release(ctx context.Context, c Container) error {
	pc, ok := c.(*pooledContainer)
	if !ok {
		return c.Close(ctx)
	}
	// Evict expired
	if time.Since(pc.createdAt) > m.ttl {
		return pc.Close(ctx)
	}
	// Return to pool
	m.mu.Lock()
	m.pool[pc.id] = append(m.pool[pc.id], pc)
	m.mu.Unlock()
	return nil
}

// Shutdown stops the janitor and closes all containers in the pool.
func (m *warmPoolManager) Shutdown(ctx context.Context) error {
	close(m.stopJanitor)
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, list := range m.pool {
		for _, c := range list {
			c.Close(ctx)
		}
	}
	m.pool = make(map[string][]*pooledContainer)
	return nil
}

// startNew launches a detached container that sleeps indefinitely.
func (m *warmPoolManager) startNew(ctx context.Context, image string) (*pooledContainer, error) {
	// e.g.: docker run -d --rm <image> tail -f /dev/null
	cmd := exec.CommandContext(ctx, "docker", "run", "-d", "--rm", image, "tail", "-f", "/dev/null")
	idBytes, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	id := string(idBytes)
	pc := &pooledContainer{
		id:        id,
		createdAt: time.Now(),
		lastUsed:  time.Now(),
	}
	return pc, nil
}

// janitor periodically evicts expired containers based on TTL.
func (m *warmPoolManager) janitor() {
	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.evictExpired()
		case <-m.stopJanitor:
			return
		}
	}
}

// evictExpired removes containers whose age exceeds TTL.
func (m *warmPoolManager) evictExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for image, list := range m.pool {
		var remaining []*pooledContainer
		for _, c := range list {
			if now.Sub(c.createdAt) > m.ttl {
				c.Close(context.Background())
			} else {
				remaining = append(remaining, c)
			}
		}
		m.pool[image] = remaining
	}
}
