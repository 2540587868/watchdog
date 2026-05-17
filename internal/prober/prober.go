package prober

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/ysqss/watchdog/internal/target"
)

type ProbeResult struct {
	TargetID   string
	Timestamp  time.Time
	Success    bool
	StatusCode int
	Latency    time.Duration
	Error      string
	BodySize   int64
	TLSExpiry  *time.Time
}

type Prober interface {
	Probe(ctx context.Context, t *target.Target) *ProbeResult
}

type HTTPProber struct {
	secureClient   *http.Client
	insecureClient *http.Client
}

func NewHTTPProber() *HTTPProber {
	secureTransport := &http.Transport{
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     30 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
	}
	insecureTransport := &http.Transport{
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     30 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	return &HTTPProber{
		secureClient: &http.Client{
			Transport: secureTransport,
		},
		insecureClient: &http.Client{
			Transport: insecureTransport,
		},
	}
}

func (p *HTTPProber) Probe(ctx context.Context, t *target.Target) *ProbeResult {
	result := &ProbeResult{
		TargetID:  t.ID,
		Timestamp: time.Now(),
	}

	client := p.secureClient
	if t.TLSSkipVerify {
		client = p.insecureClient
	}

	req, err := http.NewRequestWithContext(ctx, t.GetMethod(), t.URL, nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	for k, v := range t.Headers {
		req.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := client.Do(req)
	result.Latency = time.Since(start)

	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.Success = resp.StatusCode == t.GetExpectStatus()

	if t.ExpectBody != "" {
		buf := make([]byte, 64*1024)
		n, _ := resp.Body.Read(buf)
		body := string(buf[:n])
		if !contains(body, t.ExpectBody) {
			result.Success = false
			result.Error = "response body does not contain expected string"
		}
	}

	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		cert := resp.TLS.PeerCertificates[0]
		result.TLSExpiry = &cert.NotAfter
	}

	return result
}

type TCPProber struct{}

func NewTCPProber() *TCPProber {
	return &TCPProber{}
}

func (p *TCPProber) Probe(ctx context.Context, t *target.Target) *ProbeResult {
	result := &ProbeResult{
		TargetID:  t.ID,
		Timestamp: time.Now(),
	}

	start := time.Now()
	dialer := net.Dialer{Timeout: t.TimeoutDuration()}
	conn, err := dialer.DialContext(ctx, "tcp", t.URL)
	result.Latency = time.Since(start)

	if err != nil {
		result.Error = err.Error()
		return result
	}
	conn.Close()
	result.Success = true
	return result
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
