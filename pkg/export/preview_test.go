package export

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewPreviewServer(t *testing.T) {
	server := NewPreviewServer("/tmp/test", 8080)

	if server == nil {
		t.Fatal("NewPreviewServer returned nil")
	}

	if server.bundlePath != "/tmp/test" {
		t.Errorf("Expected bundlePath '/tmp/test', got %s", server.bundlePath)
	}

	if server.port != 8080 {
		t.Errorf("Expected port 8080, got %d", server.port)
	}
}

func TestPreviewServer_Port(t *testing.T) {
	server := NewPreviewServer("/tmp/test", 9001)

	if server.Port() != 9001 {
		t.Errorf("Expected Port() to return 9001, got %d", server.Port())
	}
}

func TestPreviewServer_URL(t *testing.T) {
	port := 9002
	server := NewPreviewServer("/tmp", port)
	expected := fmt.Sprintf("http://127.0.0.1:%d", port)
	if got := server.URL(); got != expected {
		t.Errorf("Expected URL() to return %s, got %s", expected, got)
	}
}

func TestFindAvailablePort(t *testing.T) {
	// This should find an available port in the range
	port, err := FindAvailablePort(19000, 19100)
	if err != nil {
		t.Errorf("FindAvailablePort failed: %v", err)
	}

	if port < 19000 || port > 19100 {
		t.Errorf("Port %d is outside expected range 19000-19100", port)
	}
}

func TestFindAvailablePort_NoAvailable(t *testing.T) {
	// Try to find in a very narrow range that's likely already in use
	// This is a bit tricky to test reliably, so we just verify the function exists
	// and returns the expected type
	port, err := FindAvailablePort(19200, 19200)
	if err == nil {
		// Port was available, which is fine
		if port != 19200 {
			t.Errorf("Expected port 19200, got %d", port)
		}
	}
}

func TestDefaultPreviewConfig(t *testing.T) {
	config := DefaultPreviewConfig()

	if config.Port != 0 {
		t.Errorf("Expected Port 0 (auto-select), got %d", config.Port)
	}

	if !config.OpenBrowser {
		t.Error("Expected OpenBrowser to be true")
	}

	if config.Quiet {
		t.Error("Expected Quiet to be false")
	}
}

func TestPreviewConfig(t *testing.T) {
	config := PreviewConfig{
		BundlePath:  "/tmp/bundle",
		Port:        8888,
		OpenBrowser: false,
		Quiet:       true,
	}

	if config.BundlePath != "/tmp/bundle" {
		t.Errorf("Expected BundlePath '/tmp/bundle', got %s", config.BundlePath)
	}

	if config.Port != 8888 {
		t.Errorf("Expected Port 8888, got %d", config.Port)
	}

	if config.OpenBrowser {
		t.Error("Expected OpenBrowser to be false")
	}

	if !config.Quiet {
		t.Error("Expected Quiet to be true")
	}
}

func TestPreviewServer_Start_MissingBundle(t *testing.T) {
	server := NewPreviewServer("/nonexistent/path/12345", 19050)

	err := server.Start()
	if err == nil {
		t.Error("Expected error for missing bundle path")
	}
}

func TestPreviewServer_Start_MissingIndex(t *testing.T) {
	// Create a temp directory without index.html
	tmpDir := t.TempDir()

	server := NewPreviewServer(tmpDir, 19051)

	err := server.Start()
	if err == nil {
		t.Error("Expected error for missing index.html")
	}
}

func TestPreviewServer_Integration(t *testing.T) {
	// Create a temp bundle directory
	tmpDir := t.TempDir()

	// Create index.html
	indexContent := `<!DOCTYPE html><html><head><title>Test</title></head><body>Hello</body></html>`
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte(indexContent), 0644); err != nil {
		t.Fatalf("Failed to create index.html: %v", err)
	}

	// Create a data file
	dataDir := filepath.Join(tmpDir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("Failed to create data dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "meta.json"), []byte(`{"test": true}`), 0644); err != nil {
		t.Fatalf("Failed to create meta.json: %v", err)
	}

	// Find available port
	port, err := FindAvailablePort(19060, 19080)
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}

	server := NewPreviewServer(tmpDir, port)

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := server.Start(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Test index.html
	resp, err := http.Get(server.URL())
	if err != nil {
		t.Fatalf("Failed to GET index.html: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Check no-cache headers
	cacheControl := resp.Header.Get("Cache-Control")
	if cacheControl == "" {
		t.Error("Expected Cache-Control header")
	}

	pragma := resp.Header.Get("Pragma")
	if pragma != "no-cache" {
		t.Errorf("Expected Pragma: no-cache, got %s", pragma)
	}

	// Check body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read body: %v", err)
	}

	if string(body) != indexContent {
		t.Errorf("Expected body %q, got %q", indexContent, string(body))
	}

	// Test status endpoint
	statusResp, err := http.Get(server.URL() + "/__preview__/status")
	if err != nil {
		t.Fatalf("Failed to GET status: %v", err)
	}
	defer statusResp.Body.Close()

	if statusResp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", statusResp.StatusCode)
	}

	statusBody, _ := io.ReadAll(statusResp.Body)
	if len(statusBody) == 0 {
		t.Error("Expected non-empty status response")
	}

	// Clean shutdown
	if err := server.Stop(); err != nil {
		t.Errorf("Failed to stop server: %v", err)
	}
}

func TestNoCacheMiddleware(t *testing.T) {
	// Create a simple handler
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	})

	// Wrap with no-cache middleware
	handler := noCacheMiddleware(inner)

	// Create a test request
	req, _ := http.NewRequest("GET", "/", nil)
	rec := &testResponseWriter{headers: make(http.Header)}

	handler.ServeHTTP(rec, req)

	// Check headers
	if rec.headers.Get("Cache-Control") == "" {
		t.Error("Expected Cache-Control header")
	}

	if rec.headers.Get("Pragma") != "no-cache" {
		t.Errorf("Expected Pragma: no-cache, got %s", rec.headers.Get("Pragma"))
	}

	if rec.headers.Get("Expires") != "0" {
		t.Errorf("Expected Expires: 0, got %s", rec.headers.Get("Expires"))
	}

	// Verify no CORS headers
	if rec.headers.Get("Access-Control-Allow-Origin") != "" {
		t.Error("Unexpected CORS header Access-Control-Allow-Origin found")
	}
}

// testResponseWriter is a simple ResponseWriter for testing
type testResponseWriter struct {
	headers    http.Header
	body       []byte
	statusCode int
}

func (w *testResponseWriter) Header() http.Header {
	return w.headers
}

func (w *testResponseWriter) Write(data []byte) (int, error) {
	w.body = append(w.body, data...)
	return len(data), nil
}

func (w *testResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}
