package status

import (
	"context"
	"errors"
	"testing"
	"time"
)

type probePinger struct {
	err        error
	block      bool
	sawDone    bool
	lastErr    error
	calledWith context.Context
}

func (p *probePinger) Ping(ctx context.Context) error {
	p.calledWith = ctx
	if p.block {
		<-ctx.Done()
		p.sawDone = true
		p.lastErr = ctx.Err()
		return ctx.Err()
	}
	return p.err
}

type recordingBackendExistenceChecker struct {
	exists     bool
	err        error
	block      bool
	sawDone    bool
	lastErr    error
	tenantID   string
	bucket     string
	name       string
	calledWith context.Context
}

func (f *recordingBackendExistenceChecker) BackendExists(ctx context.Context, tenantID, bucket, name string) (bool, error) {
	f.calledWith = ctx
	f.tenantID = tenantID
	f.bucket = bucket
	f.name = name
	if f.block {
		<-ctx.Done()
		f.sawDone = true
		f.lastErr = ctx.Err()
		return false, ctx.Err()
	}
	return f.exists, f.err
}

func (f *recordingBackendExistenceChecker) assertSentinelArgs(t *testing.T) {
	t.Helper()
	if f.tenantID != "" || f.bucket != "ayb_status_probe" || f.name != "reachability" {
		t.Fatalf("BackendExists args = tenantID:%q bucket:%q name:%q, want tenantID:%q bucket:%q name:%q",
			f.tenantID, f.bucket, f.name, "", "ayb_status_probe", "reachability")
	}
}

func TestDatabaseProbe_Check(t *testing.T) {
	t.Run("nil pool unhealthy", func(t *testing.T) {
		probe := NewDatabaseProbe(nil)
		res := probe.Check(context.Background())
		if res.Service != Database {
			t.Fatalf("service = %q, want %q", res.Service, Database)
		}
		if res.Healthy {
			t.Fatal("expected unhealthy for nil pool")
		}
		if res.Error == "" {
			t.Fatal("expected error message for nil pool")
		}
	})

	t.Run("nil receiver unhealthy", func(t *testing.T) {
		var probe *DatabaseProbe
		res := probe.Check(context.Background())
		if res.Service != Database || res.Healthy || res.Error == "" {
			t.Fatalf("nil receiver result = {Service:%q Healthy:%v Error:%q}, want unhealthy database error",
				res.Service, res.Healthy, res.Error)
		}
	})

	t.Run("already canceled context unhealthy", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		pinger := &probePinger{}
		probe := &DatabaseProbe{pinger: pinger}

		res := probe.Check(ctx)
		if res.Healthy || res.Error == "" {
			t.Fatalf("canceled context result = {Healthy:%v Error:%q}, want unhealthy error", res.Healthy, res.Error)
		}
		if pinger.calledWith != nil {
			t.Fatalf("pinger was called with canceled context, want fail-closed before dependency call")
		}
	})

	t.Run("healthy pinger", func(t *testing.T) {
		probe := &DatabaseProbe{pinger: &probePinger{}}
		res := probe.Check(context.Background())
		if !res.Healthy {
			t.Fatalf("expected healthy result, got error=%q", res.Error)
		}
		if res.Service != Database {
			t.Fatalf("service = %q, want %q", res.Service, Database)
		}
		if res.CheckedAt.IsZero() {
			t.Fatal("expected checkedAt to be set")
		}
		if res.Latency < 0 {
			t.Fatalf("latency must be non-negative, got %s", res.Latency)
		}
	})

	t.Run("failing pinger unhealthy", func(t *testing.T) {
		probe := &DatabaseProbe{pinger: &probePinger{err: errors.New("boom")}}
		res := probe.Check(context.Background())
		if res.Healthy {
			t.Fatal("expected unhealthy result")
		}
		if res.Error == "" {
			t.Fatal("expected error message")
		}
	})

	t.Run("blocking pinger bounded by probe timeout", func(t *testing.T) {
		pinger := &probePinger{block: true}
		probe := &DatabaseProbe{pinger: pinger, timeout: 10 * time.Millisecond}

		res := probe.Check(context.Background())
		if res.Healthy || res.Error == "" {
			t.Fatalf("blocking pinger result = {Healthy:%v Error:%q}, want unhealthy timeout error", res.Healthy, res.Error)
		}
		if !pinger.sawDone || !errors.Is(pinger.lastErr, context.DeadlineExceeded) {
			t.Fatalf("pinger deadline = sawDone:%v err:%v, want context deadline", pinger.sawDone, pinger.lastErr)
		}
	})
}

func TestStorageProbe_Check(t *testing.T) {
	t.Run("nil dependency unhealthy", func(t *testing.T) {
		probe := NewStorageProbe(nil)
		res := probe.Check(context.Background())
		if res.Service != Storage || res.Healthy || res.Error == "" {
			t.Fatalf("nil dependency result = {Service:%q Healthy:%v Error:%q}, want unhealthy storage error",
				res.Service, res.Healthy, res.Error)
		}
	})

	t.Run("nil receiver unhealthy", func(t *testing.T) {
		var probe *StorageProbe
		res := probe.Check(context.Background())
		if res.Service != Storage || res.Healthy || res.Error == "" {
			t.Fatalf("nil receiver result = {Service:%q Healthy:%v Error:%q}, want unhealthy storage error",
				res.Service, res.Healthy, res.Error)
		}
	})

	t.Run("already canceled context unhealthy", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		backend := &recordingBackendExistenceChecker{}
		probe := NewStorageProbe(backend)

		res := probe.Check(ctx)
		if res.Healthy || res.Error == "" {
			t.Fatalf("canceled context result = {Healthy:%v Error:%q}, want unhealthy error", res.Healthy, res.Error)
		}
		if backend.calledWith != nil {
			t.Fatalf("backend was called with canceled context, want fail-closed before dependency call")
		}
	})

	t.Run("existing sentinel object means reachable", func(t *testing.T) {
		backend := &recordingBackendExistenceChecker{exists: true}
		probe := NewStorageProbe(backend)

		res := probe.Check(context.Background())
		if res.Service != Storage || !res.Healthy || res.Error != "" {
			t.Fatalf("exists=true result = {Service:%q Healthy:%v Error:%q}, want healthy storage",
				res.Service, res.Healthy, res.Error)
		}
		backend.assertSentinelArgs(t)
	})

	t.Run("missing sentinel object still means reachable", func(t *testing.T) {
		backend := &recordingBackendExistenceChecker{exists: false}
		probe := NewStorageProbe(backend)

		res := probe.Check(context.Background())
		if res.Service != Storage || !res.Healthy || res.Error != "" {
			t.Fatalf("exists=false result = {Service:%q Healthy:%v Error:%q}, want healthy storage",
				res.Service, res.Healthy, res.Error)
		}
		backend.assertSentinelArgs(t)
	})

	t.Run("backend error unhealthy", func(t *testing.T) {
		backend := &recordingBackendExistenceChecker{err: errors.New("backend down: endpoint=https://storage.internal.local bucket=private")}
		probe := NewStorageProbe(backend)

		res := probe.Check(context.Background())
		if res.Service != Storage || res.Healthy || res.Error == "" {
			t.Fatalf("backend error result = {Service:%q Healthy:%v Error:%q}, want unhealthy storage error",
				res.Service, res.Healthy, res.Error)
		}
		if res.Error == backend.err.Error() {
			t.Fatalf("backend error leaked raw dependency detail: %q", res.Error)
		}
		backend.assertSentinelArgs(t)
	})

	t.Run("blocking dependency bounded by probe timeout", func(t *testing.T) {
		backend := &recordingBackendExistenceChecker{block: true}
		probe := NewStorageProbe(backend)
		probe.timeout = 10 * time.Millisecond

		res := probe.Check(context.Background())
		if res.Service != Storage || res.Healthy || res.Error == "" {
			t.Fatalf("blocking dependency result = {Service:%q Healthy:%v Error:%q}, want unhealthy timeout error",
				res.Service, res.Healthy, res.Error)
		}
		backend.assertSentinelArgs(t)
		if !backend.sawDone || !errors.Is(backend.lastErr, context.DeadlineExceeded) {
			t.Fatalf("backend deadline = sawDone:%v err:%v, want context deadline", backend.sawDone, backend.lastErr)
		}
	})
}
