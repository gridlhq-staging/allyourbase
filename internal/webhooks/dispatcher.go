// Package webhooks Stub summary for /Users/stuart/parallel_development/allyourbase_dev/may31_pm_7_webhook_replay_deadletter/allyourbase_dev/internal/webhooks/dispatcher.go.
package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/allyourbase/ayb/internal/realtime"
)

const (
	queueSize           = 1024
	maxRetries          = 3
	maxDeliveryAttempts = maxRetries + 1
)

// defaultBackoff holds the production retry delays.
var defaultBackoff = [maxRetries]time.Duration{
	1 * time.Second,
	5 * time.Second,
	25 * time.Second,
}

var errUnsafeReplayDestination = errors.New("webhook replay destination is unsafe")

var replayBlockedCIDRs = mustParseCIDRs([]string{
	"0.0.0.0/8",          // This network (RFC 1122)
	"10.0.0.0/8",         // Private (RFC 1918)
	"100.64.0.0/10",      // Carrier-grade NAT (RFC 6598)
	"127.0.0.0/8",        // Loopback (RFC 1122)
	"169.254.0.0/16",     // Link-local (RFC 3927)
	"172.16.0.0/12",      // Private (RFC 1918)
	"192.168.0.0/16",     // Private (RFC 1918)
	"224.0.0.0/4",        // Multicast (RFC 5771)
	"240.0.0.0/4",        // Reserved (RFC 1112)
	"255.255.255.255/32", // Broadcast
	"::1/128",            // IPv6 loopback
	"::/128",             // IPv6 unspecified
	"fc00::/7",           // IPv6 unique local (RFC 4193)
	"fe80::/10",          // IPv6 link-local (RFC 4291)
})

func mustParseCIDRs(cidrs []string) []*net.IPNet {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Sprintf("invalid CIDR %q: %v", cidr, err))
		}
		nets = append(nets, network)
	}
	return nets
}

func isPrivateOrReservedReplayIP(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	for _, cidr := range replayBlockedCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

func validateReplayDestination(ctx context.Context, rawURL string) error {
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return fmt.Errorf("%w: invalid webhook URL", errUnsafeReplayDestination)
	}
	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("%w: missing webhook host", errUnsafeReplayDestination)
	}
	if strings.EqualFold(host, "localhost") {
		return fmt.Errorf("%w: localhost destinations are blocked", errUnsafeReplayDestination)
	}
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateOrReservedReplayIP(ip) {
			return fmt.Errorf("%w: private or reserved IP destinations are blocked", errUnsafeReplayDestination)
		}
		return nil
	}

	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIP(lookupCtx, "ip", host)
	if err != nil {
		return nil
	}
	for _, ip := range ips {
		if isPrivateOrReservedReplayIP(ip) {
			return fmt.Errorf("%w: resolved destination is private or reserved", errUnsafeReplayDestination)
		}
	}
	return nil
}

// Dispatcher receives realtime events and delivers them to matching webhooks.
type Dispatcher struct {
	store     replayWebhookStore
	deliveryS DeliveryStore // optional — nil disables delivery logging
	client    *http.Client
	logger    *slog.Logger
	queue     chan *realtime.Event
	done      chan struct{}
	wg        sync.WaitGroup
	backoff   [maxRetries]time.Duration // per-instance; tests override without touching globals
}

type replayWebhookStore interface {
	ListEnabled(ctx context.Context) ([]Webhook, error)
	Get(ctx context.Context, id string) (*Webhook, error)
}

// NewDispatcher creates a Dispatcher and starts its background worker.
func NewDispatcher(store replayWebhookStore, logger *slog.Logger) *Dispatcher {
	d := &Dispatcher{
		store:   store,
		client:  &http.Client{Timeout: 10 * time.Second},
		logger:  logger,
		queue:   make(chan *realtime.Event, queueSize),
		done:    make(chan struct{}),
		backoff: defaultBackoff,
	}
	d.wg.Add(1)
	go d.run()
	return d
}

// SetDeliveryStore enables persistent delivery logging.
func (d *Dispatcher) SetDeliveryStore(ds DeliveryStore) {
	d.deliveryS = ds
}

// SetHTTPTransport overrides the HTTP transport used for outbound webhook requests.
func (d *Dispatcher) SetHTTPTransport(rt http.RoundTripper) {
	if d.client == nil || rt == nil {
		return
	}
	d.client.Transport = rt
}

// Enqueue adds an event to the delivery queue.
// Non-blocking: drops events if the queue is full.
func (d *Dispatcher) Enqueue(event *realtime.Event) {
	select {
	case d.queue <- event:
	default:
		d.logger.Warn("webhook queue full, dropping event",
			"table", event.Table, "action", event.Action)
	}
}

// Close signals the worker to stop and waits for it to finish.
func (d *Dispatcher) Close() {
	close(d.done)
	d.wg.Wait()
}

func (d *Dispatcher) run() {
	defer d.wg.Done()
	for {
		select {
		case <-d.done:
			return
		case event, ok := <-d.queue:
			if !ok {
				return
			}
			d.processEvent(event)
		}
	}
}

// Loads enabled webhooks, filters those matching the event's table and action, marshals the event as JSON, and delivers the payload to each matching webhook. Logs errors at the store or marshaling stages and exits early.
func (d *Dispatcher) processEvent(event *realtime.Event) {
	hooks, err := d.store.ListEnabled(context.Background())
	if err != nil {
		d.logger.Error("failed to load webhooks", "error", err)
		return
	}

	payload, err := json.Marshal(event)
	if err != nil {
		d.logger.Error("failed to marshal webhook payload", "error", err)
		return
	}

	for i := range hooks {
		if !matches(&hooks[i], event) {
			continue
		}
		d.deliver(&hooks[i], event, payload)
	}
}

func matches(hook *Webhook, event *realtime.Event) bool {
	if len(hook.Tables) > 0 && !contains(hook.Tables, event.Table) {
		return false
	}
	if len(hook.Events) > 0 && !contains(hook.Events, event.Action) {
		return false
	}
	return true
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// deliver attempts to POST the webhook payload with exponential backoff retries,
// signing the request with HMAC if the webhook has a secret configured.
func (d *Dispatcher) deliver(hook *Webhook, event *realtime.Event, payload []byte) {
	for attempt := 0; attempt < maxDeliveryAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(d.backoff[attempt-1])
		}

		recorded, err := d.sendAndRecordSingleAttempt(context.Background(), hook, event.Action, event.Table, payload, attempt+1)
		if err != nil {
			d.logger.Error("failed to create webhook request", "error", err, "url", hook.URL)
			return
		}
		if recorded.Success {
			return
		}
		if recorded.Error != "" {
			d.logger.Warn("webhook delivery failed",
				"url", hook.URL, "attempt", attempt+1, "error", recorded.Error)
			continue
		}
		d.logger.Warn("webhook returned non-2xx",
			"url", hook.URL, "status", recorded.StatusCode, "attempt", attempt+1)
	}
	d.logger.Error("webhook delivery exhausted retries", "url", hook.URL, "webhookID", hook.ID)
}

// Replay performs a synchronous, single-attempt resend of a previously recorded
// webhook delivery. It intentionally reuses the stored request body verbatim,
// which may already be truncated by the delivery recorder contract.
func (d *Dispatcher) Replay(ctx context.Context, webhookID, deliveryID string) (*Delivery, error) {
	if d.deliveryS == nil {
		return nil, fmt.Errorf("delivery history is unavailable")
	}
	recorded, err := d.deliveryS.GetDelivery(ctx, webhookID, deliveryID)
	if err != nil {
		return nil, err
	}
	hook, err := d.store.Get(ctx, webhookID)
	if err != nil {
		return nil, err
	}
	// Replays are admin-triggered and synchronous, so fail closed before the
	// outbound request if the stored destination points at localhost or a
	// private/reserved network target.
	if err := validateReplayDestination(ctx, hook.URL); err != nil {
		return nil, err
	}
	return d.sendAndRecordSingleAttempt(ctx, hook, recorded.EventAction, recorded.EventTable, []byte(recorded.RequestBody), 1)
}

// sendAndRecordSingleAttempt is the single source of truth for request creation,
// signing, HTTP send, response capture, and delivery recording.
func (d *Dispatcher) sendAndRecordSingleAttempt(ctx context.Context, hook *Webhook, eventAction, eventTable string, payload []byte, attempt int) (*Delivery, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hook.URL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if hook.Secret != "" {
		req.Header.Set("X-AYB-Signature", Sign(hook.Secret, payload))
	}

	start := time.Now()
	resp, err := d.client.Do(req)
	durationMs := int(time.Since(start).Milliseconds())
	if err != nil {
		return d.recordDelivery(hook, eventAction, eventTable, payload, 0, false, attempt, durationMs, err.Error(), ""), nil
	}
	respBytes, err := readLimitedResponseBody(resp.Body)
	if err != nil {
		readErr := fmt.Errorf("failed to read webhook response body: %w", err)
		return d.recordDelivery(hook, eventAction, eventTable, payload, resp.StatusCode, false, attempt, durationMs, readErr.Error(), ""), nil
	}

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	return d.recordDelivery(hook, eventAction, eventTable, payload, resp.StatusCode, success, attempt, durationMs, "", string(respBytes)), nil
}

func readLimitedResponseBody(body io.ReadCloser) ([]byte, error) {
	defer body.Close()
	return io.ReadAll(io.LimitReader(body, 1024))
}

// Records a webhook delivery attempt to the delivery store if one is configured, truncating the request body to 4096 bytes if needed. Logs any recording errors without failing the delivery.
func (d *Dispatcher) recordDelivery(hook *Webhook, eventAction, eventTable string, payload []byte, statusCode int, success bool, attempt, durationMs int, errMsg, respBody string) *Delivery {
	reqBody := string(payload)
	if len(reqBody) > 4096 {
		reqBody = reqBody[:4096]
	}
	del := &Delivery{
		WebhookID:    hook.ID,
		EventAction:  eventAction,
		EventTable:   eventTable,
		Success:      success,
		StatusCode:   statusCode,
		Attempt:      attempt,
		DurationMs:   durationMs,
		Error:        errMsg,
		RequestBody:  reqBody,
		ResponseBody: respBody,
	}
	if d.deliveryS == nil {
		return del
	}
	if err := d.deliveryS.RecordDelivery(context.Background(), del); err != nil {
		d.logger.Error("failed to record delivery", "error", err)
	}
	return del
}

// StartPruner begins periodic cleanup of old delivery logs.
// Does nothing if deliveryS is nil.
func (d *Dispatcher) StartPruner(interval, retention time.Duration) {
	if d.deliveryS == nil {
		return
	}
	d.wg.Add(1)
	go d.runPruner(interval, retention)
}

// Periodically deletes delivery logs older than the retention duration until the dispatcher is closed. Logs the count of pruned records and any errors encountered.
func (d *Dispatcher) runPruner(interval, retention time.Duration) {
	defer d.wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-d.done:
			return
		case <-ticker.C:
			pruned, err := d.deliveryS.PruneDeliveries(context.Background(), retention)
			if err != nil {
				d.logger.Error("failed to prune webhook deliveries", "error", err)
			} else if pruned > 0 {
				d.logger.Info("pruned old webhook deliveries", "count", pruned)
			}
		}
	}
}

// Sign computes the HMAC-SHA256 signature of body using the given secret.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
