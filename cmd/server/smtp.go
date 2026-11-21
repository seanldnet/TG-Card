package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

func smtpPortUsesImplicitTLS(port int) bool { return port == 465 }

// sendSMTPMessage supports implicit TLS (normally port 465) and STARTTLS
// (normally port 587). When requireTLS is true, plaintext SMTP is never used.
func sendSMTPMessage(ctx context.Context, host string, port int, username, password, from string, recipients []string, message []byte, requireTLS bool) error {
	host = strings.TrimSpace(host)
	if host == "" || port < 1 || port > 65535 {
		return errors.New("SMTP 主机或端口无效")
	}
	if len(recipients) == 0 {
		return errors.New("SMTP 收件人不能为空")
	}

	address := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := net.Dialer{Timeout: 15 * time.Second}
	var conn net.Conn
	var err error
	implicitTLS := smtpPortUsesImplicitTLS(port)
	if implicitTLS {
		tlsDialer := &tls.Dialer{NetDialer: &dialer, Config: &tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
		}}
		conn, err = tlsDialer.DialContext(ctx, "tcp", address)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return fmt.Errorf("连接 SMTP 服务器失败：%w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("初始化 SMTP 会话失败：%w", err)
	}
	defer client.Close()

	encrypted := implicitTLS
	if !implicitTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
				return fmt.Errorf("SMTP STARTTLS 握手失败：%w", err)
			}
			encrypted = true
		}
	}
	if requireTLS && !encrypted {
		return errors.New("SMTP 服务器未提供 TLS/STARTTLS；已按安全设置拒绝明文发送。465 端口使用 SSL/TLS，587 端口使用 STARTTLS")
	}

	if username != "" {
		if !encrypted {
			return errors.New("SMTP 账号密码认证需要加密连接，请启用 SMTP TLS 或使用支持 STARTTLS 的端口")
		}
		auth := smtp.PlainAuth("", username, password, host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP 账号认证失败：%w", err)
		}
	}

	if err := client.Mail(from); err != nil {
		return fmt.Errorf("SMTP 发件人被拒绝：%w", err)
	}
	for _, recipient := range recipients {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("SMTP 收件人 %s 被拒绝：%w", recipient, err)
		}
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP 无法开始发送邮件内容：%w", err)
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return fmt.Errorf("SMTP 邮件内容发送失败：%w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("SMTP 服务器未接受邮件：%w", err)
	}
	return nil
}
