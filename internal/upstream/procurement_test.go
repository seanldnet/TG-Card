package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestDujiaoProcurementClientClassifiesHTTPBusinessErrors(t *testing.T) {
	client:=DujiaoProcurementClient{
		BaseURL:"https://supplier.example.com",
		APIKey:"key",
		APISecret:"secret",
		request:func(_ context.Context,_ string,method,path string,_ url.Values,_ []byte,_ string,_ string)([]byte,int,error){
			if method!=http.MethodPost||path!=dujiaoAPIPrefix+"/orders"{t.Fatalf("unexpected request %s %s",method,path)}
			return []byte(`{"ok":false,"order_id":101,"order_no":"DJ101","status":"canceled","error_code":"insufficient_balance","error_message":"wallet balance insufficient"}`),http.StatusPaymentRequired,nil
		},
	}
	result,err:=client.CreateOrder(context.Background(),ProcurementRequest{SKUID:1,Quantity:1,DownstreamOrderNo:"LOCAL-1"})
	if result.OrderID!=101{t.Fatalf("expected rejected supplier order ID to be preserved, got %d",result.OrderID)}
	var businessErr *ProcurementBusinessError
	if !errors.As(err,&businessErr){t.Fatalf("expected ProcurementBusinessError, got %v",err)}
	if businessErr.Code!="insufficient_balance"||!strings.Contains(businessErr.Message,"insufficient"){t.Fatalf("unexpected business error: %+v",businessErr)}
}

func TestDujiaoProcurementClientClassifiesHTTPOrderLookupErrors(t *testing.T) {
	client:=DujiaoProcurementClient{
		BaseURL:"https://supplier.example.com",
		APIKey:"key",
		APISecret:"secret",
		request:func(_ context.Context,_ string,method,path string,_ url.Values,_ []byte,_ string,_ string)([]byte,int,error){
			if method!=http.MethodGet||path!=dujiaoAPIPrefix+"/orders/404"{t.Fatalf("unexpected request %s %s",method,path)}
			return []byte(`{"ok":false,"error_code":"order_not_found","error_message":"order not found"}`),http.StatusNotFound,nil
		},
	}
	_,err:=client.GetOrder(context.Background(),"404")
	var businessErr *ProcurementBusinessError
	if !errors.As(err,&businessErr)||businessErr.Code!="order_not_found"{t.Fatalf("expected order_not_found business error, got %v",err)}
}

func TestManualFormDataIsForwardedInProcurementRequest(t *testing.T) {
	request:=ProcurementRequest{SKUID:22,Quantity:1,DownstreamOrderNo:"LOCAL-MANUAL-1",ManualFormData:map[string]any{"account":"buyer-123","region":"asia"}}
	payload,err:=json.Marshal(request);if err!=nil{t.Fatal(err)}
	text:=string(payload)
	if !strings.Contains(text,`"manual_form_data":{"account":"buyer-123","region":"asia"}`){t.Fatalf("manual form data missing from request: %s",text)}
	autoPayload,err:=json.Marshal(ProcurementRequest{SKUID:23,Quantity:1,DownstreamOrderNo:"LOCAL-AUTO-1"});if err!=nil{t.Fatal(err)}
	if strings.Contains(string(autoPayload),"manual_form_data"){t.Fatalf("auto procurement must omit manual form data: %s",autoPayload)}
}
