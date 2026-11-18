package database

import (
	"context"
	"testing"

	"github.com/seanldnet/TG-Card/internal/config"
)

func TestUpstreamCatalogSyncIsTransactionalAndSourceAware(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{DBDriver:"sqlite", DatabaseDSN:":memory:"}
	db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)}
	defer db.Close()
	if err:=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}

	localCategory,err:=CreateCategory(ctx,db,cfg.DBDriver,"Local category",1);if err!=nil{t.Fatal(err)}
	localProduct,err:=CreateProduct(ctx,db,cfg.DBDriver,Product{CategoryID:localCategory.ID,Name:"Local product",PriceMinor:500,Currency:"USDT",DeliveryType:"card",Status:1});if err!=nil{t.Fatal(err)}

	if err:=SyncUpstreamCatalog(ctx,db,cfg.DBDriver,"batch-1",
		[]UpstreamCategoryRecord{{UpstreamID:"12",Name:"Supplier category",SortOrder:10}},
		[]UpstreamProductRecord{{ProductID:"34",SKUID:"56",Slug:"supplier-item",CategoryID:"12",Name:"Supplier item / 1 month",Description:"Upstream description",SKUCode:"MONTH",SpecValuesJSON:"{\"duration\":\"1 month\"}",FulfillmentType:"manual",ManualFormSchemaJSON:"{\"fields\":[]}",TagsJSON:"[\"hot\"]",PriceMinor:1200,PriceSnapshotMinor:1000,Currency:"CNY",StockQuantity:8,StockStatus:"in_stock",ImagePathsJSON:"[\"/uploads/upstream/item.png\"]",Active:true}},
	);err!=nil{t.Fatal(err)}

	all,err:=ListProducts(ctx,db);if err!=nil{t.Fatal(err)}
	if len(all)!=2{t.Fatalf("want local and upstream products, got %d",len(all))}
	var upstreamProduct *Product
	for i:=range all{if all[i].SourceType=="upstream"{upstreamProduct=&all[i]}}
	if upstreamProduct==nil{t.Fatal("upstream product not persisted")}
	if upstreamProduct.UpstreamProductID!="34"||upstreamProduct.UpstreamSKUID!="56"||upstreamProduct.UpstreamPriceSnapshotMinor!=1000||upstreamProduct.UpstreamCurrency!="CNY"||upstreamProduct.UpstreamSKUCode!="MONTH"||upstreamProduct.UpstreamFulfillmentType!="manual"||upstreamProduct.PriceMinor!=1200||upstreamProduct.UpstreamStockQuantity!=8{
		t.Fatalf("upstream product snapshot mismatch: %+v",upstreamProduct)
	}
	if upstreamProduct.ImagePathsJSON!="[\"/uploads/upstream/item.png\"]"{t.Fatalf("local image paths missing: %s",upstreamProduct.ImagePathsJSON)}

	visible,err:=ListVisibleProducts(ctx,db);if err!=nil{t.Fatal(err)}
	if len(visible)!=1||visible[0].ID!=localProduct.ID{t.Fatalf("disabled upstream should expose local catalog: %+v",visible)}
	settings:=UpstreamSettings{Provider:"dujiao_next",BaseURL:"https://supplier.example.com",Enabled:true}
	if err:=SaveUpstreamSettings(ctx,db,cfg.DBDriver,settings);err!=nil{t.Fatal(err)}
	visible,err=ListVisibleProducts(ctx,db);if err!=nil{t.Fatal(err)}
	if len(visible)!=1||visible[0].SourceType!="upstream"{t.Fatalf("enabled upstream should expose upstream catalog only: %+v",visible)}

	if err:=SyncUpstreamCatalog(ctx,db,cfg.DBDriver,"batch-2",nil,nil);err!=nil{t.Fatal(err)}
	all,err=ListProducts(ctx,db);if err!=nil{t.Fatal(err)}
	for _,p:=range all{
		if p.SourceType=="local"&&p.ID==localProduct.ID&&p.Status!=1{t.Fatal("local product was changed by upstream sync")}
		if p.SourceType=="upstream"&&p.Status!=0{t.Fatalf("stale upstream product should be hidden: %+v",p)}
	}
}
