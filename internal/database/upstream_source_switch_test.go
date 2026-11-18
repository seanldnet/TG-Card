package database

import (
	"context"
	"testing"

	"github.com/seanldnet/TG-Card/internal/config"
)

func TestCatalogStalePreventsUnsafeSourceSwitch(t *testing.T) {
	ctx:=context.Background()
	cfg:=config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"}
	db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)}
	defer db.Close()
	if err:=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}
	category,err:=CreateCategory(ctx,db,cfg.DBDriver,"Local",1);if err!=nil{t.Fatal(err)}
	local,err:=CreateProduct(ctx,db,cfg.DBDriver,Product{CategoryID:category.ID,Name:"Local product",PriceMinor:100,Currency:"USDT",DeliveryType:"fixed_text",Status:1});if err!=nil{t.Fatal(err)}

	settings:=UpstreamSettings{Provider:"dujiao_next",BaseURL:"https://supplier.example.com",Enabled:true,CatalogStale:true,SyncStatus:"success"}
	if err:=SaveUpstreamSettings(ctx,db,cfg.DBDriver,settings);err!=nil{t.Fatal(err)}
	visible,err:=ListVisibleProducts(ctx,db);if err!=nil{t.Fatal(err)}
	if len(visible)!=1||visible[0].ID!=local.ID{t.Fatalf("stale upstream catalog must not hide local products: %+v",visible)}

	if err:=SyncUpstreamCatalog(ctx,db,cfg.DBDriver,"fresh",nil,[]UpstreamProductRecord{{ProductID:"p1",SKUID:"s1",Name:"Supplier product",PriceMinor:200,PriceSnapshotMinor:200,Currency:"USDT",Active:true}});err!=nil{t.Fatal(err)}
	loaded,err:=GetUpstreamSettings(ctx,db,cfg.DBDriver);if err!=nil{t.Fatal(err)}
	if loaded.CatalogStale{t.Fatal("successful sync should clear catalog stale state")}
	visible,err=ListVisibleProducts(ctx,db);if err!=nil{t.Fatal(err)}
	if len(visible)!=1||visible[0].SourceType!="upstream"{t.Fatalf("fresh enabled upstream catalog should replace local display: %+v",visible)}

	settings,err=GetUpstreamSettings(ctx,db,cfg.DBDriver);if err!=nil{t.Fatal(err)}
	settings.Enabled=false
	if err:=SaveUpstreamSettings(ctx,db,cfg.DBDriver,settings);err!=nil{t.Fatal(err)}
	visible,err=ListVisibleProducts(ctx,db);if err!=nil{t.Fatal(err)}
	if len(visible)!=1||visible[0].ID!=local.ID{t.Fatalf("disabling upstream should restore local catalog: %+v",visible)}
}
