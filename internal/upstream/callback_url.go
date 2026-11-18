package upstream

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ValidateDujiaoCallbackURL requires the documented public callback endpoint.
func ValidateDujiaoCallbackURL(raw string) (string,error) {
	raw = strings.TrimSpace(raw)
	if raw == "" { return "",nil }
	u,err:=url.Parse(raw)
	if err!=nil||u==nil||u.Scheme!="https"||u.Host==""||u.User!=nil||u.RawQuery!=""||u.Fragment!=""||u.Path!="/api/v1/upstream/callback" {
		return "",fmt.Errorf("回调地址必须是 HTTPS 公网地址，路径固定为 /api/v1/upstream/callback")
	}
	host:=strings.ToLower(u.Hostname())
	if host==""||host=="localhost"||strings.HasSuffix(host,".localhost")||strings.HasSuffix(host,".local")||strings.HasSuffix(host,".internal") {
		return "",fmt.Errorf("回调地址不能使用本地主机名")
	}
	if ip:=net.ParseIP(host);ip!=nil&&!isPublicIP(ip){return "",fmt.Errorf("回调地址不能指向本地或私有网络 IP")}
	return u.String(),nil
}
