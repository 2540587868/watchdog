package prober

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ysqss/watchdog/internal/target"
)

func TestHTTPProber_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	prober := NewHTTPProber()
	tgt := &target.Target{
		ID:           "test-http",
		Type:         target.ProbeHTTP,
		URL:          srv.URL,
		ExpectStatus: 200,
		Timeout:      5000,
	}

	result := prober.Probe(context.Background(), tgt)
	if !result.Success {
		t.Errorf("Success = %v, want true; Error = %q", result.Success, result.Error)
	}
	if result.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", result.StatusCode)
	}
	if result.Latency < 0 {
		t.Error("Latency should be >= 0")
	}
	if result.TargetID != "test-http" {
		t.Errorf("TargetID = %q, want %q", result.TargetID, "test-http")
	}
}

func TestHTTPProber_WrongStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	prober := NewHTTPProber()
	tgt := &target.Target{
		ID:           "test-wrong",
		Type:         target.ProbeHTTP,
		URL:          srv.URL,
		ExpectStatus: 200,
		Timeout:      5000,
	}

	result := prober.Probe(context.Background(), tgt)
	if result.Success {
		t.Error("Success = true, want false for wrong status code")
	}
	if result.StatusCode != 500 {
		t.Errorf("StatusCode = %d, want 500", result.StatusCode)
	}
}

func TestHTTPProber_ExpectBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	prober := NewHTTPProber()
	tgt := &target.Target{
		ID:           "test-body",
		Type:         target.ProbeHTTP,
		URL:          srv.URL,
		ExpectStatus: 200,
		ExpectBody:   "hello",
		Timeout:      5000,
	}

	result := prober.Probe(context.Background(), tgt)
	if !result.Success {
		t.Errorf("Success = %v, want true; Error = %q", result.Success, result.Error)
	}
}

func TestHTTPProber_ExpectBodyFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("goodbye"))
	}))
	defer srv.Close()

	prober := NewHTTPProber()
	tgt := &target.Target{
		ID:           "test-body-fail",
		Type:         target.ProbeHTTP,
		URL:          srv.URL,
		ExpectStatus: 200,
		ExpectBody:   "hello",
		Timeout:      5000,
	}

	result := prober.Probe(context.Background(), tgt)
	if result.Success {
		t.Error("Success = true, want false when body doesn't match")
	}
	if result.Error == "" {
		t.Error("Error should not be empty when body doesn't match")
	}
}

func TestHTTPProber_CustomMethod(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	prober := NewHTTPProber()
	tgt := &target.Target{
		ID:           "test-method",
		Type:         target.ProbeHTTP,
		URL:          srv.URL,
		Method:       "POST",
		ExpectStatus: 200,
		Timeout:      5000,
	}

	result := prober.Probe(context.Background(), tgt)
	if !result.Success {
		t.Errorf("Success = %v, want true; Error = %q", result.Success, result.Error)
	}
}

func TestHTTPProber_Headers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "test-value" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	prober := NewHTTPProber()
	tgt := &target.Target{
		ID:           "test-headers",
		Type:         target.ProbeHTTP,
		URL:          srv.URL,
		ExpectStatus: 200,
		Headers:      map[string]string{"X-Custom": "test-value"},
		Timeout:      5000,
	}

	result := prober.Probe(context.Background(), tgt)
	if !result.Success {
		t.Errorf("Success = %v, want true; Error = %q", result.Success, result.Error)
	}
}

func TestHTTPProber_InvalidURL(t *testing.T) {
	prober := NewHTTPProber()
	tgt := &target.Target{
		ID:       "test-invalid",
		Type:     target.ProbeHTTP,
		URL:      "http://[::1]:namedport",
		Timeout:  5000,
	}

	result := prober.Probe(context.Background(), tgt)
	if result.Success {
		t.Error("Success = true for invalid URL, want false")
	}
	if result.Error == "" {
		t.Error("Error should not be empty for invalid URL")
	}
}

func TestHTTPProber_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	prober := NewHTTPProber()
	tgt := &target.Target{
		ID:      "test-cancel",
		Type:    target.ProbeHTTP,
		URL:     srv.URL,
		Timeout: 5000,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result := prober.Probe(ctx, tgt)
	if result.Success {
		t.Error("Success = true for cancelled context, want false")
	}
}

func TestHTTPProber_TLSSkipVerify(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	prober := NewHTTPProber()
	tgt := &target.Target{
		ID:             "test-tls-skip",
		Type:           target.ProbeHTTP,
		URL:            srv.URL,
		ExpectStatus:   200,
		TLSSkipVerify:  true,
		Timeout:        5000,
	}

	result := prober.Probe(context.Background(), tgt)
	if !result.Success {
		t.Errorf("Success = %v, want true with TLSSkipVerify; Error = %q", result.Success, result.Error)
	}
}

func TestHTTPProber_TLSExpiry(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	prober := NewHTTPProber()
	tgt := &target.Target{
		ID:             "test-tls-expiry",
		Type:           target.ProbeHTTP,
		URL:            srv.URL,
		ExpectStatus:   200,
		TLSSkipVerify:  true,
		Timeout:        5000,
	}

	result := prober.Probe(context.Background(), tgt)
	if result.TLSExpiry == nil {
		t.Error("TLSExpiry should not be nil for TLS connection")
	}
}

func TestTCPProber_Success(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	prober := NewTCPProber()
	tgt := &target.Target{
		ID:      "test-tcp",
		Type:    target.ProbeTCP,
		URL:     ln.Addr().String(),
		Timeout: 5000,
	}

	result := prober.Probe(context.Background(), tgt)
	if !result.Success {
		t.Errorf("Success = %v, want true; Error = %q", result.Success, result.Error)
	}
	if result.Latency < 0 {
		t.Error("Latency should be >= 0")
	}
}

func TestTCPProber_ConnectionRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	prober := NewTCPProber()
	tgt := &target.Target{
		ID:      "test-tcp-fail",
		Type:    target.ProbeTCP,
		URL:     addr,
		Timeout: 1000,
	}

	result := prober.Probe(context.Background(), tgt)
	if result.Success {
		t.Error("Success = true for refused connection, want false")
	}
	if result.Error == "" {
		t.Error("Error should not be empty for refused connection")
	}
}

func TestTCPProber_ContextCancelled(t *testing.T) {
	prober := NewTCPProber()
	tgt := &target.Target{
		ID:      "test-tcp-cancel",
		Type:    target.ProbeTCP,
		URL:     "192.0.2.1:80",
		Timeout: 5000,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result := prober.Probe(ctx, tgt)
	if result.Success {
		t.Error("Success = true for cancelled context, want false")
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		s, substr string
		want      bool
	}{
		{"hello world", "world", true},
		{"hello world", "hello", true},
		{"hello world", "xyz", false},
		{"hello", "hello", true},
		{"hello", "", true},
		{"", "", true},
		{"short", "longer substring", false},
	}
	for _, tt := range tests {
		got := contains(tt.s, tt.substr)
		if got != tt.want {
			t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
		}
	}
}

func TestHTTPProber_DefaultExpectStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	prober := NewHTTPProber()
	tgt := &target.Target{
		ID:      "test-default-status",
		Type:    target.ProbeHTTP,
		URL:     srv.URL,
		Timeout: 5000,
	}

	result := prober.Probe(context.Background(), tgt)
	if !result.Success {
		t.Errorf("Success = %v, want true with default expect status; Error = %q", result.Success, result.Error)
	}
}

func TestHTTPProber_SecureClientFailsSelfSigned(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	prober := NewHTTPProber()
	tgt := &target.Target{
		ID:           "test-secure-fail",
		Type:         target.ProbeHTTP,
		URL:          srv.URL,
		ExpectStatus: 200,
		TLSSkipVerify: false,
		Timeout:       5000,
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
	}
	_ = transport

	result := prober.Probe(context.Background(), tgt)
	if result.Success {
		t.Error("Success = true for self-signed cert without skip verify, want false")
	}
}
