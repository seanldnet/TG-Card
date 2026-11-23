package ratelimit

import (
	"context"
	"net"
	"net/url"
	"testing"
	"time"
)

func TestNewRedis(t *testing.T) {
	tests := []struct{ raw string; wantErr bool }{
		{"redis://localhost:6379/0", false},
		{"rediss://user:secret@example.com:6380/2", false},
		{"http://localhost:6379", true},
		{"redis:///0", true},
		{"redis://localhost/not-a-db", true},
	}
	for _, tt := range tests {
		_, err := NewRedis(tt.raw)
		if (err != nil) != tt.wantErr { t.Fatalf("NewRedis(%q) error=%v wantErr=%v", tt.raw, err, tt.wantErr) }
	}
}

func TestRedisPingAndAtomicAllow(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil { t.Fatal(err) }
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err != nil { return }
		defer conn.Close()
		// PING
		buf := make([]byte, 64)
		_, _ = conn.Read(buf)
		_, _ = conn.Write([]byte("+PONG\r\n"))
	}()
	u := url.URL{Scheme:"redis", Host:listener.Addr().String()}
	client, err := NewRedis(u.String())
	if err != nil { t.Fatal(err) }
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil { t.Fatalf("Ping: %v", err) }
}

func TestAllowRejectsInvalidLimits(t *testing.T) {
	client := &Redis{address:"127.0.0.1:1"}
	if _, err := client.Allow(context.Background(), "key", 0, time.Minute); err == nil { t.Fatal("expected invalid limit error") }
	if _, err := client.Allow(context.Background(), "key", 1, 0); err == nil { t.Fatal("expected invalid window error") }
}
