package ai

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestLineStreamReaderParsesPlainTextDeltas(t *testing.T) {
	reader := newLineStreamReader(
		io.NopCloser(strings.NewReader("\n: keepalive\ntoken: hello\ncontrol: ignored\ntoken:  world\n")),
		parseTestStreamLine,
	)

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "hello world" {
		t.Fatalf("stream text = %q; want %q", got, "hello world")
	}
}

func TestLineStreamReaderPreservesPendingBytesWithSmallCallerBuffer(t *testing.T) {
	reader := newLineStreamReader(
		io.NopCloser(strings.NewReader("token: streaming-token\n")),
		parseTestStreamLine,
	)
	buf := make([]byte, 4)
	var chunks []string

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			chunks = append(chunks, string(buf[:n]))
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
	}

	got := strings.Join(chunks, "")
	if got != "streaming-token" {
		t.Fatalf("stream text = %q; want %q", got, "streaming-token")
	}
	if strings.Join(chunks, "|") != "stre|amin|g-to|ken" {
		t.Fatalf("chunks = %q; want exact small-buffer splits", chunks)
	}
}

func TestLineStreamReaderReturnsOneParsedDeltaPerRead(t *testing.T) {
	reader := newLineStreamReader(
		io.NopCloser(strings.NewReader("token: first\ntoken: second\n")),
		parseTestStreamLine,
	)
	buf := make([]byte, 1024)

	n, err := reader.Read(buf)
	if err != nil {
		t.Fatalf("first Read err = %v; want nil", err)
	}
	if string(buf[:n]) != "first" {
		t.Fatalf("first Read = %q; want first", buf[:n])
	}

	n, err = reader.Read(buf)
	if err != nil {
		t.Fatalf("second Read err = %v; want nil", err)
	}
	if string(buf[:n]) != "second" {
		t.Fatalf("second Read = %q; want second", buf[:n])
	}
}

func TestLineStreamReaderDrainsPendingBytesBeforeEOFAndDelegatesClose(t *testing.T) {
	upstream := &closeTrackingReadCloser{Reader: strings.NewReader("token: final\n")}
	reader := newLineStreamReader(upstream, parseTestStreamLine)
	buf := make([]byte, len("final"))

	n, err := reader.Read(buf)
	if err != nil {
		t.Fatalf("first Read err = %v; want nil", err)
	}
	if string(buf[:n]) != "final" {
		t.Fatalf("first Read = %q; want %q", buf[:n], "final")
	}

	n, err = reader.Read(buf)
	if n != 0 || err != io.EOF {
		t.Fatalf("second Read = %d, %v; want 0, EOF", n, err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !upstream.closed {
		t.Fatal("Close did not delegate to upstream")
	}
}

func TestLineStreamReaderStopsWhenParserMarksStreamDone(t *testing.T) {
	reader := newLineStreamReader(
		io.NopCloser(strings.NewReader("token: kept\ndone\n token: leaked\n")),
		parseTestStreamLine,
	)

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "kept" {
		t.Fatalf("stream text = %q; want kept", got)
	}
}

func TestLineStreamReaderDefersReadErrorUntilCopiedBytesAreReturned(t *testing.T) {
	readErr := errors.New("upstream read failed")
	reader := newLineStreamReader(
		&errorAfterBytesReadCloser{payload: []byte("token: preserved\n"), err: readErr},
		parseTestStreamLine,
	)
	buf := make([]byte, len("preserved"))

	n, err := reader.Read(buf)
	if err != nil {
		t.Fatalf("first Read err = %v; want nil", err)
	}
	if string(buf[:n]) != "preserved" {
		t.Fatalf("first Read = %q; want %q", buf[:n], "preserved")
	}

	n, err = reader.Read(buf)
	if n != 0 || !errors.Is(err, readErr) {
		t.Fatalf("second Read = %d, %v; want 0, readErr", n, err)
	}
}

func TestLineStreamReaderAcceptsLinesLargerThanDefaultScannerLimit(t *testing.T) {
	longDelta := strings.Repeat("x", bufio.MaxScanTokenSize+1024)
	reader := newLineStreamReader(
		io.NopCloser(strings.NewReader("token: "+longDelta+"\n")),
		parseTestStreamLine,
	)

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != longDelta {
		t.Fatalf("stream text length = %d; want %d", len(got), len(longDelta))
	}
}

func parseTestStreamLine(line []byte) (streamLineResult, error) {
	text := strings.TrimSpace(string(line))
	if text == "" || strings.HasPrefix(text, ":") {
		return streamLineResult{}, nil
	}
	if text == "done" {
		return streamLineResult{Done: true}, nil
	}
	if token, ok := strings.CutPrefix(text, "token:"); ok {
		return streamLineResult{Delta: strings.TrimPrefix(token, " "), Emit: true}, nil
	}
	return streamLineResult{}, nil
}

type closeTrackingReadCloser struct {
	*strings.Reader
	closed bool
}

func (r *closeTrackingReadCloser) Close() error {
	r.closed = true
	return nil
}

type errorAfterBytesReadCloser struct {
	payload []byte
	err     error
	sent    bool
}

func (r *errorAfterBytesReadCloser) Read(p []byte) (int, error) {
	if r.sent {
		return 0, io.EOF
	}
	r.sent = true
	return copy(p, r.payload), r.err
}

func (r *errorAfterBytesReadCloser) Close() error {
	return nil
}
