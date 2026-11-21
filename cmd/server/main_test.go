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
 "github.com/seanldnet/TG-Card/internal/secret"
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

func TestRetryOrderDeliveryBlocksUnsafeExternalSideEffectRetries(t *testing.T) {
 cases := []struct{name, deliveryType, status string}{
  {name:"uncertain invite link", deliveryType:"invite_link", status:"delivery_uncertain"},
  {name:"uncertain file card", deliveryType:"file_card", status:"delivery_uncertain"},
  {name:"delivered file card re-delivery", deliveryType:"file_card", status:"delivered"},
 }
 for _, tc := range cases {
  t.Run(tc.name, func(t *testing.T) {
   deliveryType := tc.deliveryType
   ctx:=context.Background()
   db,err:=database.Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
   if err!=nil{t.Fatal(err)}
   defer db.Close()
   if err:=database.Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}
   if _,err:=database.EnsureAdmin(ctx,db,"sqlite","admin","a-strong-test-password");err!=nil{t.Fatal(err)}
   tokens,err:=auth.NewManager("01234567890123456789012345678901",t.TempDir())
   if err!=nil{t.Fatal(err)}
   app:=&application{db:db,driver:"sqlite",tokens:tokens,loginFailures:make(map[string][]time.Time)}
   body,_:=json.Marshal(loginRequest{Username:"admin",Password:"a-strong-test-password"})
   loginReq:=httptest.NewRequest(http.MethodPost,"/api/auth/login",bytes.NewReader(body))
   loginReq.RemoteAddr="192.0.2.10:12345"
   loginResponse:=httptest.NewRecorder()
   app.login(loginResponse,loginReq)
   if loginResponse.Code!=http.StatusOK{t.Fatalf("login failed: status=%d body=%s",loginResponse.Code,loginResponse.Body.String())}
   var envelope struct{Data map[string]string `json:"data"`}
   if err:=json.Unmarshal(loginResponse.Body.Bytes(),&envelope);err!=nil{t.Fatal(err)}
   token:=envelope.Data["accessToken"]
   product,err:=database.CreateProduct(ctx,db,"sqlite",database.Product{Name:"Uncertain delivery",PriceMinor:100,Currency:"USDT",DeliveryType:deliveryType,DeliveryConfigJSON:`{"chatId":"-100123","fileId":"test-file-id"}`,Status:1})
   if err!=nil{t.Fatal(err)}
   if deliveryType=="file_card" {
    if _,err:=database.ImportProductKeys(ctx,db,"sqlite",product.ID,[]database.StockSecret{{Ciphertext:"test-ciphertext",Fingerprint:"test-fingerprint"}});err!=nil{t.Fatal(err)}
   }
   order,err:=database.CreateOrder(ctx,db,"sqlite","bot-a","user-a","123456",product.ID,time.Minute)
   if err!=nil{t.Fatal(err)}
   if _,err:=db.ExecContext(ctx,"UPDATE orders SET status=? WHERE id=?",tc.status,order.ID);err!=nil{t.Fatal(err)}
   req:=httptest.NewRequest(http.MethodPost,"/api/orders/"+order.ID+"/retry-delivery",nil)
   req.SetPathValue("id",order.ID)
   req.Header.Set("Authorization","Bearer "+token)
   req.RemoteAddr="192.0.2.10:12345"
   response:=httptest.NewRecorder()
   app.retryOrderDelivery(response,req)
   if response.Code!=http.StatusConflict{t.Fatalf("retry should be blocked: status=%d body=%s",response.Code,response.Body.String())}
   saved,err:=database.GetOrderByID(ctx,db,"sqlite",order.ID)
   if err!=nil{t.Fatal(err)}
   if saved.Status!=tc.status{t.Fatalf("blocked retry changed order state to %q, want %q",saved.Status,tc.status)}
  })
 }
}


func TestAdminLoginRequiresValidTOTPWhenEnabled(t *testing.T) {
	ctx:=context.Background()
	db,err:=database.Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"});if err!=nil{t.Fatal(err)};defer db.Close()
	if err:=database.Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}
	created,err:=database.EnsureAdmin(ctx,db,"sqlite","totp-admin","a-strong-test-password");if err!=nil{t.Fatal(err)}
	secretValue,err:=auth.GenerateTOTPSecret();if err!=nil{t.Fatal(err)}
	secrets,err:=secret.New(t.TempDir());if err!=nil{t.Fatal(err)}
	ciphertext,err:=secrets.Encrypt(secretValue);if err!=nil{t.Fatal(err)}
	if err:=database.SetAdminTOTPPendingSecret(ctx,db,"sqlite",created.ID,ciphertext);err!=nil{t.Fatal(err)}
	if err:=database.EnableAdminTOTP(ctx,db,"sqlite",created.ID);err!=nil{t.Fatal(err)}
	tokens,err:=auth.NewManager("01234567890123456789012345678901",t.TempDir());if err!=nil{t.Fatal(err)}
	app:=&application{db:db,driver:"sqlite",tokens:tokens,secrets:secrets,loginFailures:make(map[string][]time.Time)}
	login:=func(code string)int{
		body,_:=json.Marshal(loginRequest{Username:"totp-admin",Password:"a-strong-test-password",TOTPCode:code})
		req:=httptest.NewRequest(http.MethodPost,"/api/auth/login",bytes.NewReader(body));req.RemoteAddr="192.0.2.11:12345"
		response:=httptest.NewRecorder();app.login(response,req);return response.Code
	}
	if status:=login("");status!=http.StatusUnauthorized{t.Fatalf("missing TOTP code status=%d want 401",status)}
	if status:=login("123456");status!=http.StatusUnauthorized{t.Fatalf("invalid TOTP code status=%d want 401",status)}
	code,err:=auth.TOTPCode(secretValue,time.Now());if err!=nil{t.Fatal(err)}
	if status:=login(code);status!=http.StatusOK{t.Fatalf("valid TOTP login status=%d want 200",status)}
}
