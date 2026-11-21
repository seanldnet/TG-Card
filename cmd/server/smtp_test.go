package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
)

func TestSendSMTPMessageRequiresTLSWhenConfigured(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = fmt.Fprint(conn, "220 test SMTP ready\r\n")
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			line := strings.ToUpper(scanner.Text())
			switch {
			case strings.HasPrefix(line, "EHLO "), strings.HasPrefix(line, "HELO "):
				_, _ = fmt.Fprint(conn, "250-test.local\r\n250 AUTH PLAIN\r\n")
			case strings.HasPrefix(line, "QUIT"):
				_, _ = fmt.Fprint(conn, "221 bye\r\n")
				return
			default:
				_, _ = fmt.Fprint(conn, "250 ok\r\n")
			}
		}
	}()

	_, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	var port int
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		t.Fatal(err)
	}

	err = sendSMTPMessage(context.Background(), "127.0.0.1", port, "user", "password", "from@example.com", []string{"to@example.com"}, []byte("Subject: test\r\n\r\nbody"), true)
	if err == nil || !strings.Contains(err.Error(), "未提供 TLS/STARTTLS") {
		t.Fatalf("expected explicit TLS-required error, got %v", err)
	}
	<-serverDone
}

func TestSMTPImplicitTLSIsSelectedForPort465(t *testing.T) {
	if !smtpPortUsesImplicitTLS(465) {
		t.Fatal("port 465 must use implicit TLS")
	}
	if smtpPortUsesImplicitTLS(587) {
		t.Fatal("port 587 must use STARTTLS, not implicit TLS")
	}
}
