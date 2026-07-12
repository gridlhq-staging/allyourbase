package allyourbase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	realtimePath              = "/api/realtime/ws"
	realtimeTypeAuth          = "auth"
	realtimeTypeSubscribe     = "subscribe"
	realtimeTypeUnsubscribe   = "unsubscribe"
	realtimeTypeReply         = "reply"
	realtimeTypeConnected     = "connected"
	realtimeTypeEvent         = "event"
	realtimeReplyOK           = "ok"
	realtimeDefaultBufferSize = 16
)

var realtimeRefCounter atomic.Uint64

type Event struct {
	Action    string         `json:"action"`
	Table     string         `json:"table"`
	Record    map[string]any `json:"record"`
	OldRecord map[string]any `json:"oldRecord,omitempty"`
}

// UnmarshalJSON unmarshals row events and normalizes oldRecord/old_record with
// camelCase taking precedence, matching the existing JS and Python SDKs.
func (e *Event) UnmarshalJSON(data []byte) error {
	type eventWire struct {
		Action         string         `json:"action"`
		Table          string         `json:"table"`
		Record         map[string]any `json:"record"`
		OldRecord      map[string]any `json:"oldRecord"`
		OldRecordSnake map[string]any `json:"old_record"`
	}

	var wire eventWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}

	e.Action = wire.Action
	e.Table = wire.Table
	e.Record = wire.Record
	if wire.OldRecord != nil {
		e.OldRecord = wire.OldRecord
	} else {
		e.OldRecord = wire.OldRecordSnake
	}
	return nil
}

type SubscribeOptions struct {
	Filter    string
	Reconnect RealtimeReconnectOptions
}

type RealtimeReconnectOptions struct {
	MaxAttempts int
	Delays      []time.Duration
	Jitter      time.Duration
}

func DefaultRealtimeReconnectOptions() RealtimeReconnectOptions {
	return RealtimeReconnectOptions{
		MaxAttempts: 5,
		Delays: []time.Duration{
			200 * time.Millisecond,
			500 * time.Millisecond,
			time.Second,
			2 * time.Second,
			5 * time.Second,
		},
		Jitter: 100 * time.Millisecond,
	}
}

type RealtimeClient struct {
	client *Client
}

func (r *RealtimeClient) Subscribe(ctx context.Context, table string, opts SubscribeOptions) (<-chan Event, func() error, error) {
	if r == nil || r.client == nil {
		return nil, nil, errors.New("realtime client is not configured")
	}
	if ctx == nil {
		return nil, nil, errors.New("context is required")
	}
	table = strings.TrimSpace(table)
	if table == "" {
		return nil, nil, errors.New("table is required")
	}

	subCtx, stop := context.WithCancel(ctx)
	sub := newRealtimeSubscription(subCtx, r.client, table, opts, stop)
	conn, err := sub.dialAndSubscribe(subCtx)
	if err != nil {
		stop()
		return nil, nil, err
	}
	sub.setConn(conn)
	go sub.run(conn)
	return sub.events, sub.cancel, nil
}

type realtimeSubscription struct {
	client    *Client
	ctx       context.Context
	table     string
	opts      SubscribeOptions
	reconnect RealtimeReconnectOptions
	events    chan Event
	stop      context.CancelFunc
	done      chan struct{}

	mu   sync.Mutex
	conn *websocket.Conn
	once sync.Once
}

type realtimeFrame struct {
	Type           string         `json:"type"`
	Token          string         `json:"token,omitempty"`
	Ref            string         `json:"ref,omitempty"`
	Tables         []string       `json:"tables,omitempty"`
	Filter         string         `json:"filter,omitempty"`
	Status         string         `json:"status,omitempty"`
	Message        string         `json:"message,omitempty"`
	ClientID       string         `json:"client_id,omitempty"`
	Action         string         `json:"action,omitempty"`
	Table          string         `json:"table,omitempty"`
	Record         map[string]any `json:"record,omitempty"`
	OldRecord      map[string]any `json:"oldRecord,omitempty"`
	OldRecordSnake map[string]any `json:"old_record,omitempty"`
}

func newRealtimeSubscription(ctx context.Context, client *Client, table string, opts SubscribeOptions, stop context.CancelFunc) *realtimeSubscription {
	return &realtimeSubscription{
		client:    client,
		ctx:       ctx,
		table:     table,
		opts:      opts,
		reconnect: normalizeRealtimeReconnectOptions(opts.Reconnect),
		events:    make(chan Event, realtimeDefaultBufferSize),
		stop:      stop,
		done:      make(chan struct{}),
	}
}

func (s *realtimeSubscription) cancel() error {
	s.once.Do(func() {
		s.stop()
		s.mu.Lock()
		conn := s.conn
		s.mu.Unlock()
		if conn != nil {
			_ = conn.WriteJSON(realtimeFrame{Type: realtimeTypeUnsubscribe, Tables: []string{s.table}, Ref: nextRealtimeRef()})
			_ = conn.Close()
		}
	})
	<-s.done
	return nil
}

func (s *realtimeSubscription) run(conn *websocket.Conn) {
	defer close(s.done)
	defer close(s.events)

	attempts := 0
	for {
		err := s.readEvents(conn)
		if err == nil || !s.shouldReconnect(attempts) {
			s.closeConn(conn)
			return
		}
		if !s.waitBeforeReconnect(attempts) {
			s.closeConn(conn)
			return
		}
		attempts++

		nextConn, dialErr := s.dialAndSubscribe(s.ctx)
		s.closeConn(conn)
		if dialErr != nil {
			if !s.shouldReconnect(attempts) {
				return
			}
			continue
		}
		conn = nextConn
		s.setConn(conn)
	}
}

func (s *realtimeSubscription) readEvents(conn *websocket.Conn) error {
	for {
		var frame realtimeFrame
		if err := conn.ReadJSON(&frame); err != nil {
			return err
		}
		if frame.Type != realtimeTypeEvent && !frame.isRowEvent() {
			continue
		}
		event := Event{
			Action:    frame.Action,
			Table:     frame.Table,
			Record:    frame.Record,
			OldRecord: frame.oldRecord(),
		}
		select {
		case s.events <- event:
		case <-s.ctx.Done():
			return nil
		}
	}
}

func (s *realtimeSubscription) dialAndSubscribe(ctx context.Context) (*websocket.Conn, error) {
	wsURL, err := realtimeWebsocketURL(s.client.baseURL)
	if err != nil {
		return nil, err
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, http.Header{})
	if err != nil {
		return nil, err
	}
	if err := s.handshake(conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func (s *realtimeSubscription) handshake(conn *websocket.Conn) error {
	if token := s.client.Token(); token != "" {
		ref := nextRealtimeRef()
		if err := conn.WriteJSON(realtimeFrame{Type: realtimeTypeAuth, Token: token, Ref: ref}); err != nil {
			return err
		}
		if err := waitRealtimeReply(conn, ref, "auth failed"); err != nil {
			return err
		}
	}

	ref := nextRealtimeRef()
	frame := realtimeFrame{Type: realtimeTypeSubscribe, Tables: []string{s.table}, Ref: ref}
	if s.opts.Filter != "" {
		frame.Filter = s.opts.Filter
	}
	if err := conn.WriteJSON(frame); err != nil {
		return err
	}
	return waitRealtimeReply(conn, ref, "subscribe failed")
}

func waitRealtimeReply(conn *websocket.Conn, ref, fallback string) error {
	for {
		var frame realtimeFrame
		if err := conn.ReadJSON(&frame); err != nil {
			return err
		}
		if frame.Type == realtimeTypeConnected {
			continue
		}
		if frame.Type != realtimeTypeReply || frame.Ref != ref {
			continue
		}
		if frame.Status == realtimeReplyOK {
			return nil
		}
		if frame.Message != "" {
			return errors.New(frame.Message)
		}
		return errors.New(fallback)
	}
}

func (s *realtimeSubscription) shouldReconnect(attempts int) bool {
	select {
	case <-s.ctx.Done():
		return false
	default:
	}
	return attempts < s.reconnect.MaxAttempts
}

func (s *realtimeSubscription) waitBeforeReconnect(attempt int) bool {
	delay := realtimeReconnectDelay(s.reconnect, attempt)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-s.ctx.Done():
		return false
	}
}

func (s *realtimeSubscription) setConn(conn *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conn = conn
}

func (s *realtimeSubscription) closeConn(conn *websocket.Conn) {
	if conn == nil {
		return
	}
	s.mu.Lock()
	if s.conn == conn {
		s.conn = nil
	}
	s.mu.Unlock()
	_ = conn.Close()
}

func (f realtimeFrame) isRowEvent() bool {
	return f.Action != "" && f.Table != "" && f.Record != nil
}

func (f realtimeFrame) oldRecord() map[string]any {
	if f.OldRecord != nil {
		return f.OldRecord
	}
	return f.OldRecordSnake
}

func realtimeWebsocketURL(baseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/") + realtimePath)
	if err != nil {
		return "", err
	}
	switch parsed.Scheme {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	default:
		return "", fmt.Errorf("unsupported realtime URL scheme %q", parsed.Scheme)
	}
	return parsed.String(), nil
}

func nextRealtimeRef() string {
	return fmt.Sprint(realtimeRefCounter.Add(1))
}

func normalizeRealtimeReconnectOptions(opts RealtimeReconnectOptions) RealtimeReconnectOptions {
	defaults := DefaultRealtimeReconnectOptions()
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = defaults.MaxAttempts
	}
	if len(opts.Delays) == 0 {
		opts.Delays = defaults.Delays
	}
	if opts.Jitter < 0 {
		opts.Jitter = defaults.Jitter
	}
	return opts
}

func realtimeReconnectDelay(opts RealtimeReconnectOptions, attempt int) time.Duration {
	delays := opts.Delays
	delay := delays[len(delays)-1]
	if attempt < len(delays) {
		delay = delays[attempt]
	}
	if opts.Jitter <= 0 {
		return delay
	}
	// sdk_go is a nested module and cannot import root internal/backoff, so the
	// Python-parity reconnect delay lives beside the realtime transport owner.
	return delay + time.Duration(rand.Int63n(int64(opts.Jitter)+1))
}
