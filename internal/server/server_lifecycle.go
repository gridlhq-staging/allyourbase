package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// StartWithReady begins listening. It closes the ready channel once the
// listener is bound, then blocks serving requests.
func (s *Server) StartWithReady(ready chan<- struct{}) error {
	httpServer := s.newHTTPServer(s.cfg.Address())
	s.http = httpServer

	ln, err := net.Listen("tcp", s.cfg.Address())
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	return s.serveHTTP("server starting", s.cfg.Address(), func() { close(ready) }, func() error {
		return httpServer.Serve(ln)
	})
}

// StartTLSWithReady begins serving TLS using the provided pre-created listener.
// The caller is responsible for creating the listener with the appropriate
// tls.Config (e.g. via certmagic or a self-signed cert for tests).
// It closes the ready channel once serving begins, then blocks until shutdown.
func (s *Server) StartTLSWithReady(ln net.Listener, ready chan<- struct{}) error {
	httpServer := s.newHTTPServer("")
	s.http = httpServer
	return s.serveHTTP("server starting with TLS", ln.Addr(), func() { close(ready) }, func() error {
		return httpServer.Serve(ln)
	})
}

func (s *Server) newHTTPServer(addr string) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           s.router,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

func (s *Server) serveHTTP(logMessage string, address any, signalReady func(), serve func() error) error {
	s.logger.Info(logMessage, "address", address)
	if signalReady != nil {
		signalReady()
	}
	s.startRuntimeServices(context.Background())
	if err := serve(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

func (s *Server) startRuntimeServices(ctx context.Context) {
	s.startHealthChecker()
	s.startStatusChecker(ctx)
	s.startRequestLogger(ctx)
	s.startDrainManager()
	s.startStoragePoller(ctx)
}

func (s *Server) startStoragePoller(ctx context.Context) {
	if s.pool == nil || s.infraMetrics == nil || s.storagePollerCancel != nil {
		return
	}

	pollCtx, cancel := context.WithCancel(ctx)
	s.storagePollerCancel = cancel

	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("storage poller panic recovered", "panic", r)
			}
		}()

		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		poll := func() {
			var total int64
			err := s.pool.QueryRow(pollCtx, "SELECT COALESCE(SUM(bytes_used), 0) FROM _ayb_storage_usage").Scan(&total)
			if err != nil {
				s.logger.Warn("reading storage usage for metrics failed", "error", err)
				return
			}
			s.infraMetrics.SetStorageBytes(total)
		}

		poll()
		for {
			select {
			case <-pollCtx.Done():
				return
			case <-ticker.C:
				poll()
			}
		}
	}()
}

func (s *Server) startRequestLogger(ctx context.Context) {
	if s.requestLogger == nil {
		return
	}
	s.requestLogger.Start(ctx)
}

func (s *Server) startStatusChecker(ctx context.Context) {
	if s.statusChecker == nil {
		return
	}
	s.statusChecker.Start(ctx)
}

func (s *Server) startHealthChecker() {
	if s.healthChecker == nil {
		return
	}
	s.healthChecker.Start()
}

func (s *Server) startDrainManager() {
	manager := s.currentDrainManager()
	if manager == nil {
		return
	}
	manager.Start()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.http == nil && s.requestLogger == nil {
		return nil
	}

	timeout := time.Duration(s.cfg.Server.ShutdownTimeout) * time.Second
	shutdownCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	s.logger.Info("shutting down server", "timeout", timeout)
	s.stopRequestRateLimiters()
	s.stopBackgroundServices()

	shutdownErr := shutdownHTTPThenMetrics(
		shutdownCtx,
		func(ctx context.Context) error {
			if s.http == nil {
				return nil
			}
			err := s.http.Shutdown(ctx)
			s.http = nil
			return err
		},
		nil,
	)
	shutdownErr = errors.Join(shutdownErr, s.shutdownRequestLogger(shutdownCtx))
	s.stopDrainManager()
	shutdownErr = errors.Join(shutdownErr, s.shutdownTelemetry(shutdownCtx))
	s.closeRuntimeResources()
	return shutdownErr
}

func (s *Server) stopRequestRateLimiters() {
	if s.authRL != nil {
		stopServerRateLimiter(s.authRL)
	}
	if s.authSensitiveRL != nil {
		stopServerRateLimiter(s.authSensitiveRL)
	}
	if s.authHandler != nil {
		s.authHandler.StopRateLimiters()
	}
	if s.assistantRL != nil {
		stopServerRateLimiter(s.assistantRL)
	}
	if s.apiRL != nil {
		stopServerRateLimiter(s.apiRL)
	}
	if s.apiAnonRL != nil {
		stopServerRateLimiter(s.apiAnonRL)
	}
	if s.appRL != nil {
		stopServerRateLimiter(s.appRL)
	}
	if s.adminRL != nil {
		stopServerRateLimiter(s.adminRL)
	}
	if s.storageCDNPurgeAllRL != nil {
		stopServerRateLimiter(s.storageCDNPurgeAllRL)
	}
}

func (s *Server) stopBackgroundServices() {
	if s.jobService != nil {
		s.jobService.Stop()
	}
	if s.webhookDispatcher != nil {
		closeServerWebhookDispatcher(s.webhookDispatcher)
	}
	if s.storagePollerCancel != nil {
		s.storagePollerCancel()
		s.storagePollerCancel = nil
	}
	if s.healthChecker != nil {
		s.healthChecker.Stop()
	}
	if s.statusChecker != nil {
		s.statusChecker.Stop()
	}
}

func (s *Server) shutdownRequestLogger(ctx context.Context) error {
	if s.requestLogger != nil {
		if err := s.requestLogger.Shutdown(ctx); err != nil {
			s.logger.Error("request logger shutdown error", "error", err)
			return err
		}
		s.requestLogger = nil
	}
	return nil
}

func (s *Server) stopDrainManager() {
	if manager := s.currentDrainManager(); manager != nil {
		manager.Stop()
	}
}

func (s *Server) shutdownTelemetry(ctx context.Context) error {
	var shutdownErr error
	if s.tracerProvider != nil {
		if err := s.tracerProvider.Shutdown(ctx); err != nil {
			s.logger.Error("tracer provider shutdown error", "error", err)
			shutdownErr = errors.Join(shutdownErr, err)
		}
	}
	if s.httpMetrics != nil {
		if err := s.httpMetrics.Shutdown(ctx); err != nil {
			s.logger.Error("metrics shutdown error", "error", err)
			shutdownErr = errors.Join(shutdownErr, err)
		}
	}
	return shutdownErr
}

func (s *Server) closeRuntimeResources() {
	// Drain the connection manager before hard-closing transports: give existing
	// connections a chance to leave gracefully, then force-close any that remain.
	if s.connManager != nil {
		drainTimeout := time.Duration(s.cfg.Server.ShutdownTimeout) * time.Second
		s.connManager.Drain(drainTimeout)
	}
	if s.poolRouter != nil {
		s.poolRouter.Close()
	}
	if s.wsHandler != nil {
		s.wsHandler.Shutdown()
	}
	s.hub.Close()
}

func shutdownHTTPThenMetrics(ctx context.Context, shutdownHTTP func(context.Context) error, shutdownMetrics func(context.Context) error) error {
	var errs []error
	if shutdownHTTP != nil {
		if err := shutdownHTTP(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if shutdownMetrics != nil {
		if err := shutdownMetrics(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func stopServerRateLimiter(rl interface{ Stop() }) {
	recoverClosedChannelPanic(func() {
		rl.Stop()
	})
}

func closeServerWebhookDispatcher(dispatcher interface{ Close() }) {
	recoverClosedChannelPanic(func() {
		dispatcher.Close()
	})
}

func recoverClosedChannelPanic(fn func()) {
	defer func() {
		if recovered := recover(); recovered != nil && fmt.Sprint(recovered) != "close of closed channel" {
			panic(recovered)
		}
	}()
	fn()
}
