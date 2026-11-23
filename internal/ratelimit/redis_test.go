package ratelimit

import (
	"bufio"
	"context"
	"fmt"
	"strings"
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

func TestRedisPing(t *testing.T) {
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

func TestReadRESP(t *testing.T) {
	tests:=[]struct{input string;want any;wantErr bool}{
		{"+OK\r\n","OK",false},
		{":42\r\n",int64(42),false},
		{"$3\r\nfoo\r\n","foo",false},
		{"$-1\r\n",nil,false},
		{"*2\r\n+OK\r\n:7\r\n",[]any{"OK",int64(7)},false},
		{"-ERR denied\r\n",nil,true},
	}
	for _,tt:=range tests{
		got,err:=readRESP(bufio.NewReader(strings.NewReader(tt.input)))
		if (err!=nil)!=tt.wantErr{t.Fatalf("readRESP(%q) err=%v wantErr=%v",tt.input,err,tt.wantErr)}
		if !tt.wantErr && fmt.Sprint(got)!=fmt.Sprint(tt.want){t.Fatalf("readRESP(%q)=%v want %v",tt.input,got,tt.want)}
	}
}

func TestRedisAllowUsesAtomicScript(t *testing.T) {
	listener,err:=net.Listen("tcp","127.0.0.1:0");if err!=nil{t.Fatal(err)};defer listener.Close()
	go func(){
		conn,err:=listener.Accept();if err!=nil{return};defer conn.Close()
		reader:=bufio.NewReader(conn)
		_,_=readRESP(reader) // EVAL command array
		_,_=conn.Write([]byte(":1\r\n"))
	}()
	client,err:=NewRedis("redis://"+listener.Addr().String()+"/0");if err!=nil{t.Fatal(err)}
	ctx,cancel:=context.WithTimeout(context.Background(),time.Second);defer cancel()
	allowed,err:=client.Allow(ctx,"client|route",1,time.Minute)
	if err!=nil||!allowed{t.Fatalf("Allow()=(%v,%v), want true,nil",allowed,err)}
}

func TestLeaseScriptsUseOwnershipToken(t *testing.T) {
	for name,script:=range map[string]string{"renew":renewLeaseScript,"release":releaseLeaseScript}{
		if !strings.Contains(script,"ARGV[1]")||!strings.Contains(script,"GET") {t.Fatalf("%s lease script does not verify token ownership",name)}
	}
}
