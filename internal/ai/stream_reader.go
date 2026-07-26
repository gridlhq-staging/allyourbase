package ai

import (
	"bufio"
	"io"
	"net/http"
)

const (
	streamReaderInitialLineBuffer = 64 * 1024
	streamReaderMaxLineBytes      = maxResponseSize
)

type streamLineParser func([]byte) (streamLineResult, error)

type streamLineResult struct {
	Delta string
	Emit  bool
	Done  bool
}

type lineStreamReader struct {
	upstream   io.ReadCloser
	scanner    *bufio.Scanner
	parseLine  streamLineParser
	pending    []byte
	pendingErr error
}

func newLineStreamReader(upstream io.ReadCloser, parseLine streamLineParser) io.ReadCloser {
	scanner := bufio.NewScanner(upstream)
	scanner.Buffer(make([]byte, 0, streamReaderInitialLineBuffer), streamReaderMaxLineBytes)
	return &lineStreamReader{
		upstream:  upstream,
		scanner:   scanner,
		parseLine: parseLine,
	}
}

// Read returns decoded text deltas from the upstream line-oriented stream.
func (r *lineStreamReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	copied := 0
	for copied < len(p) {
		if len(r.pending) > 0 {
			n := copy(p[copied:], r.pending)
			copied += n
			r.pending = r.pending[n:]
			if copied == len(p) {
				return copied, nil
			}
			if copied > 0 {
				return copied, nil
			}
			continue
		}

		if r.pendingErr != nil {
			if copied > 0 {
				return copied, nil
			}
			return 0, r.pendingErr
		}

		r.scanNextDelta()
	}
	return copied, nil
}

func (r *lineStreamReader) Close() error {
	return r.upstream.Close()
}

// scanNextDelta advances to the next emitted delta or terminal stream state.
func (r *lineStreamReader) scanNextDelta() {
	for r.scanner.Scan() {
		result, err := r.parseLine(r.scanner.Bytes())
		if err != nil {
			r.pendingErr = err
			return
		}
		if result.Done {
			r.pendingErr = io.EOF
			return
		}
		if result.Emit && result.Delta != "" {
			r.pending = []byte(result.Delta)
			return
		}
	}
	if err := r.scanner.Err(); err != nil {
		r.pendingErr = err
		return
	}
	r.pendingErr = io.EOF
}

func streamHTTPClientWithoutTimeout(client *http.Client) *http.Client {
	if client == nil {
		return http.DefaultClient
	}
	return &http.Client{
		Transport:     client.Transport,
		CheckRedirect: client.CheckRedirect,
		Jar:           client.Jar,
	}
}
