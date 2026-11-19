package main

import (
 "net"
 "net/http"
 "strings"
 "sync"
 "time"
)

func parseTrustedProxyCIDRs(raw string) ([]net.IPNet,error) {
 var out []net.IPNet
 for _,part:=range strings.Split(raw,",") {
  value:=strings.TrimSpace(part);if value==""{continue}
  _,network,err:=net.ParseCIDR(value);if err!=nil{return nil,err}
  out=append(out,*network)
 }
 return out,nil
}

func ipInNetworks(ip net.IP,networks []net.IPNet) bool {
 for _,network:=range networks {if network.Contains(ip){return true}}
 return false
}

func requestPeerIP(r *http.Request) net.IP {
 host,_,err:=net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
 if err!=nil {host=strings.TrimSpace(r.RemoteAddr)}
 return net.ParseIP(strings.Trim(host,"[]"))
}

// clientIPFromRequest trusts forwarding headers only when the immediate peer is
// explicitly allow-listed. X-Forwarded-For is walked right-to-left so an
// untrusted client cannot prepend a spoofed address.
func clientIPFromRequest(r *http.Request, trusted []net.IPNet) string {
 peer:=requestPeerIP(r)
 if peer==nil {return "unknown"}
 if !ipInNetworks(peer,trusted) {return peer.String()}
 forwarded:=strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
 if forwarded!="" {
  chain:=strings.Split(forwarded,",")
  current:=peer
  for i:=len(chain)-1;i>=0;i-- {
   if !ipInNetworks(current,trusted) {break}
   candidate:=net.ParseIP(strings.TrimSpace(chain[i]))
   if candidate==nil {break}
   current=candidate
  }
  return current.String()
 }
 if value:=net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP")));value!=nil{return value.String()}
 return peer.String()
}

func (a *application) clientIP(r *http.Request) string {
 return clientIPFromRequest(r,a.trustedProxyCIDRs)
}

func (a *application) allowSensitiveRequest(key string,limit int,window time.Duration) bool {
 now:=time.Now()
 a.rateLimitMu.Lock()
 defer a.rateLimitMu.Unlock()
 if a.rateLimitHits==nil {a.rateLimitHits=make(map[string][]time.Time)}
 cutoff:=now.Add(-window)
 hits:=a.rateLimitHits[key]
 kept:=hits[:0]
 for _,stamp:=range hits {if stamp.After(cutoff){kept=append(kept,stamp)}}
 if len(kept)>=limit {a.rateLimitHits[key]=kept;return false}
 a.rateLimitHits[key]=append(kept,now)
 // Opportunistically bound memory from clients that no longer send requests.
 if len(a.rateLimitHits)>4096 {
  for k,stamps:=range a.rateLimitHits {
   if len(stamps)==0||stamps[len(stamps)-1].Before(cutoff){delete(a.rateLimitHits,k)}
  }
 }
 return true
}

func (a *application) sensitiveRateLimit(name string,limit int,window time.Duration,next http.HandlerFunc) http.HandlerFunc {
 return func(w http.ResponseWriter,r *http.Request) {
  key:=a.clientIP(r)+"|"+name
  if !a.allowSensitiveRequest(key,limit,window) {
   w.Header().Set("Retry-After",strings.TrimSpace((time.Until(time.Now().Add(window))).Round(time.Second).String()))
   writeError(w,http.StatusTooManyRequests,"操作过于频繁，请稍后重试")
   return
  }
  next(w,r)
 }
}

var _ sync.Locker
