package notifier

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHttpNotifier_Notify_GET_JSONEncoding(t *testing.T) {
	var gotMethod, gotPath string
	var gotContentType string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	cfg := Config{
		Type: "http",
		Params: map[string]string{
			"Target":       srv.URL + "/notify/{{.Amount}}/{{.Message}}",
			"Method":       http.MethodGet,
			"Encoding":     string(EncodingJson),
			"BodyTemplate": "", // ignored for GET
		},
	}

	n := NewHttpNotifier(cfg)

	comment := `quote: "hello world"`
	if err := n.Notify(12345, comment); err != nil {
		t.Fatalf("Notify returned error: %v", err)
	}

	if gotMethod != http.MethodGet {
		t.Errorf("expected method GET, got %s", gotMethod)
	}
	// For JSON encoding, Message is escaped using template.JSEscapeString; in URL it will include backslashes.
	// Validate structure and escaping without relying on exact backslash counts in %q formatting.
	if !strings.HasPrefix(gotPath, "/notify/12345/") {
		t.Errorf("unexpected path prefix, got %q", gotPath)
	}
	if !strings.Contains(gotPath, `\"hello world\"`) {
		t.Errorf("expected escaped quotes in path, got %q", gotPath)
	}
	if gotContentType != string(EncodingJson) {
		t.Errorf("unexpected Content-Type. want %q got %q", string(EncodingJson), gotContentType)
	}
}

func TestHttpNotifier_Notify_POST_FormEncoding(t *testing.T) {
	var gotMethod, gotPath string
	var gotContentType string
	var gotBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	cfg := Config{
		Type: "http",
		Params: map[string]string{
			"Target":       srv.URL + "/api",
			"Method":       http.MethodPost,
			"Encoding":     string(EncodingForm),
			"BodyTemplate": "amount={{.Amount}}&message={{.Message}}",
		},
	}

	n := NewHttpNotifier(cfg)

	comment := "a b&c"
	if err := n.Notify(42, comment); err != nil {
		t.Fatalf("Notify returned error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("expected method POST, got %s", gotMethod)
	}
	if gotPath != "/api" {
		t.Errorf("unexpected path. want %q got %q", "/api", gotPath)
	}
	if gotContentType != string(EncodingForm) {
		t.Errorf("unexpected Content-Type. want %q got %q", string(EncodingForm), gotContentType)
	}
	expectedBody := "amount=42&message=a+b%26c"
	if gotBody != expectedBody {
		t.Errorf("unexpected body. want %q got %q", expectedBody, gotBody)
	}
}

func TestHttpNotifier_Notify_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "oops", http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfg := Config{
		Type: "http",
		Params: map[string]string{
			"Target":       srv.URL,
			"Method":       http.MethodGet,
			"Encoding":     string(EncodingJson),
			"BodyTemplate": "",
		},
	}

	n := NewHttpNotifier(cfg)

	err := n.Notify(1, "")
	if err == nil {
		t.Fatalf("expected error on non-200 response, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected status code: 500") {
		t.Errorf("unexpected error: %v", err)
	}
}
