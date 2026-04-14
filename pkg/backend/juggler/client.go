package juggler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/VulpineOS/foxbridge/pkg/backend"
)

// Verify Client implements backend.Backend at compile time.
var _ backend.Backend = (*Client)(nil)

// Client is a high-level Juggler protocol client that implements backend.Backend.
type Client struct {
	transport Transport
	nextID    atomic.Int64
	pending   map[int]chan *Message
	pendingMu sync.Mutex
	handlers  map[string][]backend.EventHandler
	handlerMu sync.RWMutex
	done      chan struct{}
	closeOnce sync.Once
}

// NewClient creates a Juggler client using the given transport.
func NewClient(transport Transport) *Client {
	c := &Client{
		transport: transport,
		pending:   make(map[int]chan *Message),
		handlers:  make(map[string][]backend.EventHandler),
		done:      make(chan struct{}),
	}
	go c.readLoop()
	return c
}

// DefaultCallTimeout is the default timeout for Call().
const DefaultCallTimeout = 30 * time.Second

// Call sends a synchronous RPC request and waits for the response with a default 30-second timeout.
func (c *Client) Call(sessionID, method string, params json.RawMessage) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultCallTimeout)
	defer cancel()
	return c.CallWithContext(ctx, sessionID, method, params)
}

// CallWithContext sends a synchronous RPC request and waits for the response, respecting the given context.
func (c *Client) CallWithContext(ctx context.Context, sessionID, method string, params json.RawMessage) (json.RawMessage, error) {
	id := int(c.nextID.Add(1))

	msg := &Message{
		ID:        id,
		Method:    method,
		Params:    params,
		SessionID: sessionID,
	}

	ch := make(chan *Message, 1)
	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	if err := c.transport.Send(msg); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, err
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("call %s: %w", method, ctx.Err())
	case <-c.done:
		return nil, fmt.Errorf("client closed")
	}
}

// Subscribe registers a handler for a Juggler event.
func (c *Client) Subscribe(event string, handler func(sessionID string, params json.RawMessage)) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	c.handlers[event] = append(c.handlers[event], handler)
}

// Close shuts down the client and transport.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		close(c.done)
	})
	return c.transport.Close()
}

func (c *Client) readLoop() {
	for {
		msg, err := c.transport.Receive()
		if err != nil {
			select {
			case <-c.done:
				return
			default:
				c.Close()
				return
			}
		}

		if msg.IsResponse() {
			c.pendingMu.Lock()
			ch, ok := c.pending[msg.ID]
			if ok {
				delete(c.pending, msg.ID)
			}
			c.pendingMu.Unlock()
			if ok {
				ch <- msg
			}
		} else if msg.IsEvent() {
			c.handlerMu.RLock()
			handlers := c.handlers[msg.Method]
			c.handlerMu.RUnlock()
			if len(handlers) == 0 {
				log.Printf("[juggler] unhandled event: %s (session=%s)", msg.Method, msg.SessionID)
			}
			for _, h := range handlers {
				h(msg.SessionID, msg.Params)
			}
		}
	}
}
