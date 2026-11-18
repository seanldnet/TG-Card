package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response,error)
func (f roundTripFunc) RoundTrip(r *http.Request)(*http.Response,error){return f(r)}

func TestDujiaoCreateOrderReusesDownstreamIdempotencyKey(t *testing.T) {
	created:=map[string]int64{}
	debits:=0
	client:=&http.Client{Transport:roundTripFunc(func(r *http.Request)(*http.Response,error){
		body,err:=io.ReadAll(r.Body);if err!=nil{return nil,err}
		var input struct{SKU int64 `json:"sku_id"`;Quantity int `json:"quantity"`;Downstream string `json:"downstream_order_no"`}
		if err:=json.Unmarshal(body,&input);err!=nil{return nil,err}
		if input.Downstream==""||input.SKU!=202||input.Quantity!=1{return nil,fmt.Errorf("unexpected supplier request: %+v",input)}
		id,ok:=created[input.Downstream]
		if !ok{debits++;id=7001;created[input.Downstream]=id}
		payload,_:=json.Marshal(map[string]any{"ok":true,"order_id":id,"order_no":"DJ-7001","status":"paid"})
		return &http.Response{StatusCode:200,Body:io.NopCloser(strings.NewReader(string(payload))),Header:make(http.Header)},nil
	})}
	ctx:=context.Background()
	first,err:=dujiaoCreateOrderWithClient(ctx,client,"https://supplier.example","api-key","api-secret","202",1,"TC-ORDER-1","")
	if err!=nil{t.Fatal(err)}
	second,err:=dujiaoCreateOrderWithClient(ctx,client,"https://supplier.example","api-key","api-secret","202",1,"TC-ORDER-1","")
	if err!=nil{t.Fatal(err)}
	if first.OrderID!=second.OrderID||first.OrderNo!=second.OrderNo{t.Fatalf("duplicate request did not resolve to same upstream order: %+v %+v",first,second)}
	if debits!=1{t.Fatalf("supplier debit count=%d, want exactly one",debits)}
}

func TestVerifyDujiaoCallbackSignatureAndTimestamp(t *testing.T) {
	body:=[]byte(`{"order_id":77,"downstream_order_no":"TC-ORDER-77","status":"delivered"}`)
	now:=time.Unix(1_800_000_000,0).UTC()
	ts:=strconv.FormatInt(now.Unix(),10)
	headers:=make(http.Header)
	headers.Set("Dujiao-Next-Api-Key","api-key")
	headers.Set("Dujiao-Next-Timestamp",ts)
	headers.Set("Dujiao-Next-Signature",DujiaoSignature(http.MethodPost,DujiaoCallbackPath,ts,body,"api-secret"))
	if err:=VerifyDujiaoCallback("api-key","api-secret",headers,body,now);err!=nil{t.Fatalf("valid callback rejected: %v",err)}
	headers.Set("Dujiao-Next-Signature","invalid")
	if err:=VerifyDujiaoCallback("api-key","api-secret",headers,body,now);err==nil{t.Fatal("invalid signature accepted")}
	headers.Set("Dujiao-Next-Signature",DujiaoSignature(http.MethodPost,DujiaoCallbackPath,strconv.FormatInt(now.Unix()-61,10),body,"api-secret"))
	headers.Set("Dujiao-Next-Timestamp",strconv.FormatInt(now.Unix()-61,10))
	if err:=VerifyDujiaoCallback("api-key","api-secret",headers,body,now);err==nil{t.Fatal("stale callback timestamp accepted")}
}
