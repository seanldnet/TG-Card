package upstream

import (
	"encoding/json"
	"testing"
)

func TestParseMinorUnits(t *testing.T) {
	cases := map[string]int64{"7.90":790,"0.005":1,"12":1200,"0":0}
	for input,want:=range cases { got,err:=ParseMinorUnits(input);if err!=nil||got!=want{t.Errorf("ParseMinorUnits(%q)=%d,%v want %d",input,got,err,want)} }
	if _,err:=ParseMinorUnits("-1.00");err==nil{t.Fatal("negative price must be rejected")}
}

func TestConvertMinorToUSDT(t *testing.T) {
	// 7.90 CNY at 0.14 USDT/CNY -> 1.11 USDT after rounding to cents.
	got,err:=ConvertMinorToUSDT(790,140000)
	if err!=nil||got!=111{t.Fatalf("converted amount = %d, %v; want 111",got,err)}
	if _,err:=ConvertMinorToUSDT(100,0);err==nil{t.Fatal("zero FX rate must be rejected")}
}

func TestApplyMarkup(t *testing.T) {
	got,err:=ApplyMarkup(1000,1250,25)
	if err!=nil||got!=1150{t.Fatalf("markup = %d, %v; want 1150",got,err)}
	if _,err:=ApplyMarkup(1000,-1,0);err==nil{t.Fatal("negative markup must be rejected")}
}

func TestVariantName(t *testing.T) {
	sku:=DujiaoSKU{Code:"RED-128",SpecValues:map[string]any{"容量":"128GB","颜色":"红色"}}
	if got:=VariantName("手机",sku,2);got!="手机 · 容量:128GB / 颜色:红色"{t.Fatalf("unexpected variant name: %s",got)}
	if got:=VariantName("基础商品",DujiaoSKU{Code:"DEFAULT"},1);got!="基础商品"{t.Fatalf("default SKU should not duplicate title: %s",got)}
}

func TestOfficialDujiaoCatalogResponseShape(t *testing.T) {
	payload := []byte(`{
		"ok":true,
		"items":[{
			"id":101,"slug":"example","title":{"zh-CN":"示例商品"},
			"description":{"zh-CN":"商品摘要"},"content":{"zh-CN":"<p>商品详情</p>"},
			"images":["https://cdn.example.com/item.png"],"tags":["hot"],
			"price_amount":"7.90","fulfillment_type":"manual",
			"manual_form_schema":{"fields":[{"name":"account"}]},
			"is_active":true,"category_id":9,
			"skus":[{"id":201,"sku_code":"DEFAULT","spec_values":{"容量":"128GB"},"price_amount":"7.90","stock_status":"in_stock","stock_quantity":12,"is_active":true}]
		}],
		"total":1,"page":1,"page_size":100
	}`)
	var response productsEnvelope
	if err:=json.Unmarshal(payload,&response);err!=nil{t.Fatal(err)}
	if !response.OK||len(response.Items)!=1||response.Items[0].ID!=101||len(response.Items[0].SKUs)!=1{t.Fatalf("unexpected response: %+v",response)}
	product:=response.Items[0]
	if product.FulfillmentType!="manual"||product.SKUs[0].ID!=201||product.SKUs[0].StockQuantity!=12||product.SKUs[0].Code!="DEFAULT"{t.Fatalf("SKU/fulfillment metadata mismatch: %+v",product)}
	if LocalizedText(product.Title)!="示例商品"||CleanDescription(product.Description,product.Content)!="商品摘要"{t.Fatal("multilingual text normalization failed")}
}
