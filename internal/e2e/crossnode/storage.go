package crossnode

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"testing"
	"time"
)

// Upload posts a single in-memory file to the storage bucket at baseURL and
// returns the full response (including headers) plus the decoded JSON body.
func Upload(t *testing.T, baseURL, bucket, filename, fileBody, token string) (Response, JSONBody) {
	t.Helper()

	requestBody := &bytes.Buffer{}
	writer := multipart.NewWriter(requestBody)
	fileWriter, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create multipart file field: %v", err)
	}
	if _, err := fileWriter.Write([]byte(fileBody)); err != nil {
		t.Fatalf("write multipart file field: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/storage/"+bucket, requestBody)
	if err != nil {
		t.Fatalf("build upload request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upload %s/%s failed: %v", bucket, filename, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read upload response: %v", err)
	}
	response := Response{Status: resp.StatusCode, Body: string(raw), Header: resp.Header}
	return response, decodeJSONBody(t, http.MethodPost, req.URL.String(), string(raw))
}

// StorageObjectURL builds the GET URL for a stored object.
func StorageObjectURL(baseURL, bucket, filename string) string {
	return baseURL + "/api/storage/" + bucket + "/" + url.PathEscape(filename)
}

// WaitForStorageBody polls a stored object until it returns wantStatus with
// wantBody or the timeout elapses, returning the final response.
func WaitForStorageBody(
	t *testing.T,
	baseURL string,
	bucket string,
	filename string,
	token string,
	wantStatus int,
	wantBody string,
	timeout time.Duration,
) Response {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var resp Response
	objectURL := StorageObjectURL(baseURL, bucket, filename)
	for time.Now().Before(deadline) {
		resp = Do(t, RawRequest{Method: http.MethodGet, URL: objectURL, Token: token})
		if resp.Status == wantStatus && resp.Body == wantBody {
			return resp
		}
		time.Sleep(100 * time.Millisecond)
	}
	return resp
}
