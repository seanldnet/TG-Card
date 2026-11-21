package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
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


func TestSendSMTPMessageCompletesSTARTTLSAuthenticationAndDelivery(t *testing.T) {
	certServer := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	certificate := certServer.TLS.Certificates[0]
	certServer.Close()
	parsed, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(parsed)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	type result struct {
		body string
		authenticated bool
	}
	serverDone := make(chan result, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- result{}
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		_, _ = fmt.Fprint(conn, "220 test SMTP ready\r\n")
		tlsActive := false
		authenticated := false
		var body strings.Builder
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil {
				serverDone <- result{body: body.String(), authenticated: authenticated}
				return
			}
			command := strings.TrimSpace(line)
			upper := strings.ToUpper(command)
			switch {
			case strings.HasPrefix(upper, "EHLO "):
				if tlsActive {
					_, _ = fmt.Fprint(conn, "250-test.local\r\n250 AUTH PLAIN\r\n")
				} else {
					_, _ = fmt.Fprint(conn, "250-test.local\r\n250-STARTTLS\r\n250 AUTH PLAIN\r\n")
				}
			case upper == "STARTTLS":
				_, _ = fmt.Fprint(conn, "220 begin TLS\r\n")
				tlsConn := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
				if tlsConn.Handshake() != nil {
					serverDone <- result{body: body.String(), authenticated: authenticated}
					return
				}
				conn = tlsConn
				reader = bufio.NewReader(conn)
				tlsActive = true
			case strings.HasPrefix(upper, "AUTH PLAIN"):
				authenticated = tlsActive
				if authenticated {
					_, _ = fmt.Fprint(conn, "235 authentication successful\r\n")
				} else {
					_, _ = fmt.Fprint(conn, "538 encryption required\r\n")
				}
			case strings.HasPrefix(upper, "MAIL FROM:"):
				_, _ = fmt.Fprint(conn, "250 sender accepted\r\n")
			case strings.HasPrefix(upper, "RCPT TO:"):
				_, _ = fmt.Fprint(conn, "250 recipient accepted\r\n")
			case upper == "DATA":
				_, _ = fmt.Fprint(conn, "354 send message\r\n")
				for {
					dataLine, dataErr := reader.ReadString('\n')
					if dataErr != nil {
						serverDone <- result{body: body.String(), authenticated: authenticated}
						return
					}
					if dataLine == ".\r\n" {
						break
					}
					body.WriteString(dataLine)
				}
				_, _ = fmt.Fprint(conn, "250 message accepted\r\n")
			case upper == "QUIT":
				_, _ = fmt.Fprint(conn, "221 bye\r\n")
				serverDone <- result{body: body.String(), authenticated: authenticated}
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
	message := []byte("Subject: TLS test\r\n\r\nsmtp delivery body\r\n")
	err = sendSMTPMessageWithTLSConfig(context.Background(), "127.0.0.1", port, "test-user", "test-password", "from@example.com", []string{"to@example.com"}, message, true, &tls.Config{RootCAs: roots})
	if err != nil {
		t.Fatalf("expected successful STARTTLS delivery, got %v", err)
	}
	got := <-serverDone
	if !got.authenticated {
		t.Fatal("SMTP AUTH must occur only after successful STARTTLS")
	}
	if !strings.Contains(got.body, "smtp delivery body") {
		t.Fatalf("SMTP server did not receive message body: %q", got.body)
	}
}

func TestSMTPTLSConfigPreservesTrustRootsAndEnforcesTLS12(t *testing.T) {
	roots := x509.NewCertPool()
	got := smtpTLSConfig("smtp.example.test", &tls.Config{RootCAs: roots})
	if got.ServerName != "smtp.example.test" {
		t.Fatalf("unexpected ServerName: %q", got.ServerName)
	}
	if got.MinVersion != tls.VersionTLS12 {
		t.Fatalf("expected TLS 1.2 minimum, got %d", got.MinVersion)
	}
	if got.RootCAs != roots {
		t.Fatal("custom root pool must be preserved")
	}
}
