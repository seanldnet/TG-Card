package upstream

import "testing"

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
