package webclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestIsPublic(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{"93.184.215.14", true},
		{"2606:2800:21f:cb07:6820:80da:af6b:8b2c", true},
		{"127.0.0.1", false},
		{"::1", false},
		{"0.0.0.0", false},
		{"::", false},
		{"10.1.2.3", false},
		{"172.16.0.1", false},
		{"192.168.1.1", false},
		{"169.254.169.254", false},
		{"100.64.0.1", false},
		{"198.18.0.1", false},
		{"224.0.0.1", false},
		{"255.255.255.255", false},
		{"fd00::1", false},
		{"fe80::1", false},
		{"ff02::1", false},
		{"::ffff:127.0.0.1", false},
		{"::ffff:192.168.1.1", false},
		{"64:ff9b::a00:1", false},
		{"2002:c0a8:101::1", false},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			if got := isPublic(netip.MustParseAddr(tt.addr)); got != tt.want {
				t.Errorf("isPublic(%s) = %v, want %v", tt.addr, got, tt.want)
			}
		})
	}
}

func TestCheckConnectionAllowsOnlyWebPorts(t *testing.T) {
	tests := []struct {
		address string
		allowed bool
	}{
		{"93.184.215.14:443", true},
		{"93.184.215.14:80", true},
		{"[2606:2800:21f:cb07:6820:80da:af6b:8b2c]:443", true},
		{"93.184.215.14:22", false},
		{"93.184.215.14:8080", false},
		{"192.168.1.1:80", false},
	}
	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			err := checkConnection("tcp", tt.address, nil)
			if tt.allowed && err != nil {
				t.Errorf("refused %s: %v", tt.address, err)
			}
			if !tt.allowed && !errors.Is(err, ErrNonPublicAddress) {
				t.Errorf("err = %v, want ErrNonPublicAddress", err)
			}
		})
	}
}

func TestClientRefusesLocalTargets(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits++ }))
	t.Cleanup(server.Close)

	client := New(5 * time.Second)
	for _, target := range []string{server.URL, "http://localhost/", "http://169.254.169.254/computeMetadata/v1/"} {
		t.Run(target, func(t *testing.T) {
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := client.Do(req)
			if err == nil {
				resp.Body.Close()
			}
			if !errors.Is(err, ErrNonPublicAddress) {
				t.Fatalf("err = %v, want ErrNonPublicAddress", err)
			}
		})
	}
	if hits != 0 {
		t.Errorf("local server received %d requests", hits)
	}
}

func TestClientIgnoresEnvironmentProxy(t *testing.T) {
	transport := New(time.Second).Transport.(*http.Transport)
	if transport.Proxy != nil {
		t.Error("transport uses a proxy")
	}
}
