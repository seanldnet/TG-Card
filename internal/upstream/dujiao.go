package upstream

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const dujiaoAPIPrefix = "/api/v1/upstream"

type DujiaoPingResult struct {
	SiteName string `json:"site_name"`
	ProtocolVersion string `json:"protocol_version"`
	UserID int64 `json:"user_id"`
	Balance string `json:"balance"`
	Currency string `json:"currency"`
}

type dujiaoEnvelope struct {
	OK bool `json:"ok"`
	ErrorCode string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
	DujiaoPingResult
}

// ValidateDujiaoBaseURL accepts only an HTTPS origin. API paths are appended by
// the adapter, preventing user-controlled paths, query strings, or credentials.
func ValidateDujiaoBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil { return "", fmt.Errorf("站点地址格式无效") }
	if parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("站点地址必须是 HTTPS 根地址，不得包含账号、路径、查询参数或片段")
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return "", fmt.Errorf("站点地址主机名无效")
	}
	if ip := net.ParseIP(host); ip != nil && (!isPublicIP(ip)) {
		return "", fmt.Errorf("站点地址不能指向本地或私有网络 IP")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func isPublicIP(ip net.IP) bool {
	return ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsUnspecified() && !ip.IsMulticast()
}

func DujiaoSignature(method, path, timestamp string, body []byte, apiSecret string) string {
	bodyHash := md5.Sum(body)
	signingText := strings.ToUpper(method) + "\n" + path + "\n" + timestamp + "\n" + hex.EncodeToString(bodyHash[:])
	mac := hmac.New(sha256.New, []byte(apiSecret))
	_, _ = mac.Write([]byte(signingText))
	return hex.EncodeToString(mac.Sum(nil))
}

func DujiaoPing(ctx context.Context, baseURL, apiKey, apiSecret string) (DujiaoPingResult, error) {
	var result DujiaoPingResult
	origin, err := ValidateDujiaoBaseURL(baseURL)
	if err != nil { return result, err }
	if strings.TrimSpace(apiKey) == "" || strings.TrimSpace(apiSecret) == "" { return result, fmt.Errorf("Dujiao API Key 和 API Secret 均不能为空") }

	const path = dujiaoAPIPrefix + "/ping"
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	body := []byte{}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, origin+path, strings.NewReader(string(body)))
	if err != nil { return result, fmt.Errorf("create Dujiao ping request: %w", err) }
	req.Header.Set("Dujiao-Next-Api-Key", apiKey)
	req.Header.Set("Dujiao-Next-Timestamp", timestamp)
	req.Header.Set("Dujiao-Next-Signature", DujiaoSignature(http.MethodPost, path, timestamp, body, apiSecret))
	req.Header.Set("Content-Type", "application/json")

	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(dialCtx context.Context, network, address string) (net.Conn, error) {
			host, port, splitErr := net.SplitHostPort(address)
			if splitErr != nil { return nil, splitErr }
			ips, lookupErr := net.DefaultResolver.LookupIPAddr(dialCtx, host)
			if lookupErr != nil { return nil, fmt.Errorf("resolve upstream host: %w", lookupErr) }
			for _, candidate := range ips {
				if !isPublicIP(candidate.IP) { return nil, fmt.Errorf("upstream host resolves to a non-public IP address") }
			}
			if len(ips) == 0 { return nil, fmt.Errorf("upstream host has no IP addresses") }
			return dialer.DialContext(dialCtx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
	}
	client := &http.Client{
		Timeout: 12 * time.Second,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Do(req)
	if err != nil { return result, fmt.Errorf("连接 Dujiao-Next 失败：%w", err) }
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil { return result, fmt.Errorf("读取 Dujiao-Next 响应失败") }
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return result, fmt.Errorf("Dujiao-Next 返回 HTTP %d", response.StatusCode)
	}
	var envelope dujiaoEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil { return result, fmt.Errorf("Dujiao-Next 返回了无法识别的响应") }
	if !envelope.OK {
		message := strings.TrimSpace(envelope.ErrorMessage)
		if message == "" { message = "上游拒绝连接，请检查 API Key、API Secret 和凭证审批状态" }
		if len(message) > 300 { message = message[:300] }
		return result, fmt.Errorf("Dujiao-Next: %s", message)
	}
	return envelope.DujiaoPingResult, nil
}
