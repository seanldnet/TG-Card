package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/seanldnet/TG-Card/internal/auth"
	"github.com/seanldnet/TG-Card/internal/config"
	"github.com/seanldnet/TG-Card/internal/database"
	"github.com/seanldnet/TG-Card/internal/secret"
)

func TestAdminActionRequestHashCanonicalizesAndBindsTarget(t *testing.T) {
	first,err:=adminActionRequestHash(http.MethodPost,"/api/users/u1/balance",[]byte(`{"amountAtomic":100,"direction":"credit"}`));if err!=nil{t.Fatal(err)}
	reordered,err:=adminActionRequestHash(http.MethodPost,"/api/users/u1/balance",[]byte(`{"direction":"credit","amountAtomic":100}`));if err!=nil{t.Fatal(err)}
	if first!=reordered{t.Fatal("JSON key order must not change the confirmation hash")}
	changedPayload,err:=adminActionRequestHash(http.MethodPost,"/api/users/u1/balance",[]byte(`{"amountAtomic":101,"direction":"credit"}`));if err!=nil{t.Fatal(err)}
	if first==changedPayload{t.Fatal("changed action payload must produce a different hash")}
	changedPath,err:=adminActionRequestHash(http.MethodPost,"/api/users/u2/balance",[]byte(`{"amountAtomic":100,"direction":"credit"}`));if err!=nil{t.Fatal(err)}
	if first==changedPath{t.Fatal("changed target path must produce a different hash")}
	if _,err:=adminActionRequestHash(http.MethodGet,"/api/users/u1/balance",nil);err==nil{t.Fatal("GET must not be eligible for step-up confirmation")}
}

func TestAdminActionConfirmationChallengeAndOneTimeMiddleware(t *testing.T) {
	ctx:=context.Background()
	db,err:=database.Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"});if err!=nil{t.Fatal(err)};defer db.Close()
	if err:=database.Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}
	if _,err:=database.EnsureAdmin(ctx,db,"sqlite","step-up-admin","a-strong-test-password");err!=nil{t.Fatal(err)}
	tokens,err:=auth.NewManager("01234567890123456789012345678901",t.TempDir());if err!=nil{t.Fatal(err)}
	secrets,err:=secret.New(t.TempDir());if err!=nil{t.Fatal(err)}
	app:=&application{db:db,driver:"sqlite",tokens:tokens,secrets:secrets,loginFailures:make(map[string][]time.Time)}
	loginBody,_:=json.Marshal(loginRequest{Username:"step-up-admin",Password:"a-strong-test-password"})
	loginResp:=httptest.NewRecorder();app.login(loginResp,httptest.NewRequest(http.MethodPost,"/api/auth/login",bytes.NewReader(loginBody)))
	if loginResp.Code!=http.StatusOK{t.Fatalf("login status=%d body=%s",loginResp.Code,loginResp.Body.String())}
	var loginEnvelope struct{Data struct{AccessToken string `json:"accessToken"`} `json:"data"`}
	if err:=json.Unmarshal(loginResp.Body.Bytes(),&loginEnvelope);err!=nil{t.Fatal(err)}
	accessToken:=loginEnvelope.Data.AccessToken;if accessToken==""{t.Fatal("missing access token")}
	payload:=json.RawMessage(`{"amountAtomic":100,"direction":"credit","note":"step-up test"}`)
	confirmBody,_:=json.Marshal(adminActionConfirmationRequest{Password:"a-strong-test-password",ActionKey:"wallet.balance.adjust",Method:http.MethodPost,Path:"/api/users/user-1/balance",Payload:payload})
	confirmReq:=httptest.NewRequest(http.MethodPost,"/api/admin/security/confirm-action",bytes.NewReader(confirmBody));confirmReq.Header.Set("Authorization","Bearer "+accessToken)
	confirmResp:=httptest.NewRecorder();app.confirmAdminAction(confirmResp,confirmReq)
	if confirmResp.Code!=http.StatusOK{t.Fatalf("confirm status=%d body=%s",confirmResp.Code,confirmResp.Body.String())}
	var confirmationEnvelope struct{Data struct{Token string `json:"token"`} `json:"data"`}
	if err:=json.Unmarshal(confirmResp.Body.Bytes(),&confirmationEnvelope);err!=nil{t.Fatal(err)}
	proof:=confirmationEnvelope.Data.Token;if proof==""{t.Fatal("missing action confirmation token")}
	requestBody:=[]byte(`{"note":"step-up test","direction":"credit","amountAtomic":100}`)
	handlerCalls:=0
	next:=http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){handlerCalls++;body,_:=io.ReadAll(r.Body);if len(body)==0{t.Error("request body was not restored after confirmation validation")};writeSuccess(w,http.StatusOK,map[string]bool{"ok":true})})
	wrapped:=app.requireAdminActionConfirmation("wallet.balance.adjust",next)
	newRequest:=func()*http.Request{req:=httptest.NewRequest(http.MethodPost,"/api/users/user-1/balance",bytes.NewReader(requestBody));req.Header.Set("Authorization","Bearer "+accessToken);req.Header.Set(adminActionConfirmationHeader,proof);return req}
	firstResp:=httptest.NewRecorder();wrapped(firstResp,newRequest())
	if firstResp.Code!=http.StatusOK||handlerCalls!=1{t.Fatalf("first protected request status=%d handlerCalls=%d body=%s",firstResp.Code,handlerCalls,firstResp.Body.String())}
	secondResp:=httptest.NewRecorder();wrapped(secondResp,newRequest())
	if secondResp.Code!=http.StatusPreconditionRequired||handlerCalls!=1{t.Fatalf("reused confirmation status=%d handlerCalls=%d",secondResp.Code,handlerCalls)}
}
