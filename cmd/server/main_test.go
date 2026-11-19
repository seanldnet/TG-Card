package main

import (
 "bytes"
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "testing"
 "time"

 "github.com/seanldnet/TG-Card/internal/auth"
 "github.com/seanldnet/TG-Card/internal/config"
 "github.com/seanldnet/TG-Card/internal/database"
)

func TestLogoutRevokesOnlyCurrentAdminBearerSession(t *testing.T) {
 ctx:=context.Background()
 db,err:=database.Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
 if err!=nil{t.Fatal(err)}
 defer db.Close()
 if err:=database.Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}
 if _,err:=database.EnsureAdmin(ctx,db,"sqlite","admin","a-strong-test-password");err!=nil{t.Fatal(err)}
 tokens,err:=auth.NewManager("01234567890123456789012345678901",t.TempDir())
 if err!=nil{t.Fatal(err)}
 app:=&application{db:db,driver:"sqlite",tokens:tokens,loginFailures:make(map[string][]time.Time)}
 login:=func()string{
  t.Helper()
  body,_:=json.Marshal(loginRequest{Username:"admin",Password:"a-strong-test-password"})
  req:=httptest.NewRequest(http.MethodPost,"/api/auth/login",bytes.NewReader(body))
  req.RemoteAddr="192.0.2.10:12345"
  response:=httptest.NewRecorder()
  app.login(response,req)
  if response.Code!=http.StatusOK{t.Fatalf("login failed: status=%d body=%s",response.Code,response.Body.String())}
  var envelope struct{Data map[string]string `json:"data"`}
  if err:=json.Unmarshal(response.Body.Bytes(),&envelope);err!=nil{t.Fatal(err)}
  token:=envelope.Data["accessToken"]
  if token==""{t.Fatal("login response did not include accessToken")}
  return token
 }
 tokenA:=login()
 tokenB:=login()
 authenticate:=func(token string)bool{
  t.Helper()
  req:=httptest.NewRequest(http.MethodGet,"/api/user/info",nil)
  req.Header.Set("Authorization","Bearer "+token)
  response:=httptest.NewRecorder()
  _,ok:=app.authenticate(response,req)
  return ok
 }
 if !authenticate(tokenA)||!authenticate(tokenB){t.Fatal("both freshly issued sessions should authenticate")}
 logoutReq:=httptest.NewRequest(http.MethodPost,"/api/auth/logout",nil)
 logoutReq.Header.Set("Authorization","Bearer "+tokenA)
 logoutReq.RemoteAddr="192.0.2.10:12345"
 logoutResponse:=httptest.NewRecorder()
 app.logout(logoutResponse,logoutReq)
 if logoutResponse.Code!=http.StatusOK{t.Fatalf("logout failed: status=%d body=%s",logoutResponse.Code,logoutResponse.Body.String())}
 if authenticate(tokenA){t.Fatal("logged-out bearer token must be rejected")}
 if !authenticate(tokenB){t.Fatal("logging out one device must not revoke the other active session")}
}
