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
		[]UpstreamCategoryRecord{{UpstreamID:"12",Name:"Supplier category",Icon:"/uploads/upstream/category.png",SortOrder:10}},
		[]UpstreamProductRecord{{ProductID:"34",SKUID:"56",Slug:"supplier-item",CategoryID:"12",Name:"Supplier item / 1 month",Description:"Upstream description",SKUCode:"MONTH",SpecValuesJSON:"{\"duration\":\"1 month\"}",FulfillmentType:"manual",ManualFormSchemaJSON:"{\"fields\":[]}",TagsJSON:"[\"hot\"]",PriceMinor:1200,PriceSnapshotMinor:1000,Currency:"CNY",StockQuantity:8,StockStatus:"in_stock",ImagePathsJSON:"[\"/uploads/upstream/item.png\"]",Active:true}},
	);err!=nil{t.Fatal(err)}

	all,err:=ListProducts(ctx,db);if err!=nil{t.Fatal(err)}
	if len(all)!=2{t.Fatalf("want local and upstream products, got %d",len(all))}
	var upstreamProduct *Product
	for i:=range all{if all[i].SourceType=="upstream"{upstreamProduct=&all[i]}}
	if upstreamProduct==nil{t.Fatal("upstream product not persisted")}
	if upstreamProduct.UpstreamProductID!="34"||upstreamProduct.UpstreamSKUID!="56"||upstreamProduct.UpstreamPriceSnapshotMinor!=1000||upstreamProduct.UpstreamCurrency!="CNY"||upstreamProduct.UpstreamSKUCode!="MONTH"||upstreamProduct.UpstreamSpecValuesJSON!=`{"duration":"1 month"}`||upstreamProduct.UpstreamFulfillmentType!="manual"||upstreamProduct.UpstreamManualFormSchemaJSON!=`{"fields":[]}`||upstreamProduct.UpstreamTagsJSON!=`["hot"]`||upstreamProduct.PriceMinor!=1200||upstreamProduct.UpstreamStockQuantity!=8{
		t.Fatalf("upstream product snapshot mismatch: %+v",upstreamProduct)
	}
	if upstreamProduct.ImagePathsJSON!="[\"/uploads/upstream/item.png\"]"{t.Fatalf("local image paths missing: %s",upstreamProduct.ImagePathsJSON)}
	categories,err:=ListCategories(ctx,db);if err!=nil{t.Fatal(err)}
	
	var upstreamCategory *Category
	for i:=range categories{if categories[i].SourceType=="upstream"{upstreamCategory=&categories[i]}}
	if upstreamCategory==nil||upstreamCategory.UpstreamIcon!="/uploads/upstream/category.png"{t.Fatalf("category icon was not stored locally: %+v",upstreamCategory)}

	// A failed sync transaction must not mutate the last successful catalog.
	if err:=SyncUpstreamCatalog(ctx,db,cfg.DBDriver,"bad-batch",nil,[]UpstreamProductRecord{{ProductID:"34",SKUID:"56",Name:"invalid",Currency:"",PriceMinor:1}});err==nil{t.Fatal("invalid catalog batch should fail")}
	all,err=ListProducts(ctx,db);if err!=nil{t.Fatal(err)}
	for _,p:=range all{if p.SourceType=="upstream"&&(p.Status!=1||p.PriceMinor!=1200){t.Fatalf("failed sync changed prior upstream product: %+v",p)}}

	// An icon/image download failure is represented by PreserveIcon/empty image path and keeps last known local assets.
	if err:=SyncUpstreamCatalog(ctx,db,cfg.DBDriver,"batch-2",
		[]UpstreamCategoryRecord{{UpstreamID:"12",Name:"Supplier category",PreserveIcon:true}},
		[]UpstreamProductRecord{{ProductID:"34",SKUID:"56",Name:"Supplier item / 1 month",PriceMinor:1200,PriceSnapshotMinor:1000,Currency:"CNY",ImagePathsJSON:""}},
	);err!=nil{t.Fatal(err)}
	categories,err=ListCategories(ctx,db);if err!=nil{t.Fatal(err)}
	for _,category:=range categories{if category.SourceType=="upstream"&&category.UpstreamIcon!="/uploads/upstream/category.png"{t.Fatalf("failed icon refresh erased prior icon: %+v",category)}}
	all,err=ListProducts(ctx,db);if err!=nil{t.Fatal(err)}
	for _,p:=range all{if p.SourceType=="upstream"&&p.ImagePathsJSON!="[\"/uploads/upstream/item.png\"]"{t.Fatalf("failed image refresh erased prior image: %+v",p)}}

	visible,err:=ListVisibleProducts(ctx,db);if err!=nil{t.Fatal(err)}
	if len(visible)!=1||visible[0].ID!=localProduct.ID{t.Fatalf("disabled upstream should expose local catalog: %+v",visible)}
	settings:=UpstreamSettings{Provider:"dujiao_next",BaseURL:"https://supplier.example.com",Enabled:true}
	if err:=SaveUpstreamSettings(ctx,db,cfg.DBDriver,settings);err!=nil{t.Fatal(err)}
	visible,err=ListVisibleProducts(ctx,db);if err!=nil{t.Fatal(err)}
	if len(visible)!=1||visible[0].SourceType!="upstream"{t.Fatalf("enabled upstream should expose upstream catalog only: %+v",visible)}

	if err:=SyncUpstreamCatalog(ctx,db,cfg.DBDriver,"batch-3",nil,nil);err!=nil{t.Fatal(err)}
	all,err=ListProducts(ctx,db);if err!=nil{t.Fatal(err)}
	for _,p:=range all{
		if p.SourceType=="local"&&p.ID==localProduct.ID&&p.Status!=1{t.Fatal("local product was changed by upstream sync")}
		if p.SourceType=="upstream"&&p.Status!=0{t.Fatalf("stale upstream product should be hidden: %+v",p)}
	}
}
