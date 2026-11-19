package database

import (
 "context"
 "testing"

 "github.com/seanldnet/TG-Card/internal/config"
)

func TestUpstreamCatalogBatchFinalizationAndVisiblePagination(t *testing.T) {
 ctx:=context.Background()
 cfg:=config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"}
 db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)};defer db.Close()
 if err:=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}
 localCategory,err:=CreateCategory(ctx,db,cfg.DBDriver,"Local",1);if err!=nil{t.Fatal(err)}
 localProduct,err:=CreateProduct(ctx,db,cfg.DBDriver,Product{CategoryID:localCategory.ID,Name:"Local item",PriceMinor:100,Currency:"USDT",DeliveryType:"card",Status:1});if err!=nil{t.Fatal(err)}
 settings:=UpstreamSettings{Provider:"dujiao_next",BaseURL:"https://supplier.example.com",Enabled:true,CatalogStale:true}
 if err:=SaveUpstreamSettings(ctx,db,cfg.DBDriver,settings);err!=nil{t.Fatal(err)}
 categories:=[]UpstreamCategoryRecord{{UpstreamID:"cat-1",Name:"Supplier category"}}
 first:=[]UpstreamProductRecord{{ProductID:"p-1",SKUID:"s-1",CategoryID:"cat-1",Name:"First supplier item",PriceMinor:100,Currency:"USDT",Active:true}}
 if err:=SyncUpstreamCatalogBatch(ctx,db,cfg.DBDriver,"batch-one",categories,first,true);err!=nil{t.Fatal(err)}
 if err:=SyncUpstreamCatalogBatch(ctx,db,cfg.DBDriver,"batch-two",categories,nil,false);err!=nil{t.Fatal(err)}
 second:=[]UpstreamProductRecord{{ProductID:"p-2",SKUID:"s-2",CategoryID:"cat-1",Name:"Second supplier item",PriceMinor:200,Currency:"USDT",Active:true}}
 if err:=SyncUpstreamCatalogBatch(ctx,db,cfg.DBDriver,"batch-two",nil,second,false);err!=nil{t.Fatal(err)}
 visible,err:=ListVisibleProductsPage(ctx,db,cfg.DBDriver,10,0,"","");if err!=nil{t.Fatal(err)}
 if len(visible)!=2 {t.Fatalf("intermediate batch must not hide unseen rows: got %d",len(visible))}
 for _,item:=range visible {if item.SourceType!="upstream"{t.Fatalf("local product leaked into upstream source: %+v",item)}}
 var secondItem Product
 for _,item:=range visible {if item.UpstreamProductID=="p-2"{secondItem=item}}
 if secondItem.ID==""||secondItem.CategoryID=="" {t.Fatalf("product batch failed to resolve category ID: %+v",secondItem)}
 detail,err:=GetVisibleProductByID(ctx,db,cfg.DBDriver,secondItem.ID);if err!=nil||detail.ID!=secondItem.ID{t.Fatalf("single product lookup failed: %+v %v",detail,err)}
 cats,err:=ListVisibleCategoriesPage(ctx,db,cfg.DBDriver,8,0);if err!=nil||len(cats)!=1{t.Fatalf("visible category pagination failed: %+v %v",cats,err)}
 if err:=SyncUpstreamCatalogBatch(ctx,db,cfg.DBDriver,"batch-two",nil,nil,true);err!=nil{t.Fatal(err)}
 visible,err=ListVisibleProductsPage(ctx,db,cfg.DBDriver,10,0,"","");if err!=nil{t.Fatal(err)}
 if len(visible)!=1||visible[0].UpstreamProductID!="p-2"{t.Fatalf("finalization must hide only upstream rows absent from successful run: %+v",visible)}
 all,err:=ListProducts(ctx,db);if err!=nil{t.Fatal(err)}
 for _,item:=range all {if item.ID==localProduct.ID&&item.Status!=1{t.Fatal("upstream batch changed local product status")}}
}

func TestMarkUpstreamProductSeenIfUnchanged(t *testing.T) {
 ctx:=context.Background();cfg:=config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"}
 db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)};defer db.Close()
 if err:=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}
 if err:=SaveUpstreamSettings(ctx,db,cfg.DBDriver,UpstreamSettings{Provider:"dujiao_next",BaseURL:"https://supplier.example.com",Enabled:true,CatalogStale:true});err!=nil{t.Fatal(err)}
 if err:=SyncUpstreamCatalog(ctx,db,cfg.DBDriver,"batch-old",nil,[]UpstreamProductRecord{{ProductID:"101",SKUID:"201",Name:"Fingerprint item",PriceMinor:100,Currency:"USDT",UpstreamUpdatedAt:"2026-11-20T10:00:00Z",UpstreamFingerprint:"fingerprint-a",Active:true}});err!=nil{t.Fatal(err)}
 unchanged,err:=MarkUpstreamProductSeenIfUnchanged(ctx,db,cfg.DBDriver,"101","fingerprint-a","batch-new",1);if err!=nil||!unchanged{t.Fatalf("unchanged product should be marked seen: %v %v",unchanged,err)}
 changed,err:=MarkUpstreamProductSeenIfUnchanged(ctx,db,cfg.DBDriver,"101","fingerprint-b","batch-new",1);if err!=nil||changed{t.Fatalf("changed fingerprint must require a rewrite: %v %v",changed,err)}
 if err:=SyncUpstreamCatalogBatch(ctx,db,cfg.DBDriver,"batch-new",nil,nil,true);err!=nil{t.Fatal(err)}
 visible,err:=ListVisibleProductsPage(ctx,db,cfg.DBDriver,10,0,"","");if err!=nil{t.Fatal(err)}
 if len(visible)!=1||visible[0].UpstreamProductID!="101"{t.Fatalf("unchanged product was hidden by finalization: %+v",visible)}
}

func TestMarkUpstreamCategorySeenIfUnchanged(t *testing.T) {
 ctx:=context.Background();cfg:=config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"}
 db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)};defer db.Close()
 if err:=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}
 if err:=SaveUpstreamSettings(ctx,db,cfg.DBDriver,UpstreamSettings{Provider:"dujiao_next",BaseURL:"https://supplier.example.com",Enabled:true,CatalogStale:true});err!=nil{t.Fatal(err)}
 categories:=[]UpstreamCategoryRecord{{UpstreamID:"cat-1",Name:"Stable category",Fingerprint:"category-fp-1"}}
 if err:=SyncUpstreamCatalog(ctx,db,cfg.DBDriver,"category-old",categories,nil);err!=nil{t.Fatal(err)}
 unchanged,err:=MarkUpstreamCategorySeenIfUnchanged(ctx,db,cfg.DBDriver,"cat-1","category-fp-1","category-new");if err!=nil||!unchanged{t.Fatalf("unchanged category should be marked seen: %v %v",unchanged,err)}
 changed,err:=MarkUpstreamCategorySeenIfUnchanged(ctx,db,cfg.DBDriver,"cat-1","category-fp-2","category-new");if err!=nil||changed{t.Fatalf("changed category fingerprint must require an update: %v %v",changed,err)}
 if err:=SyncUpstreamCatalogBatch(ctx,db,cfg.DBDriver,"category-new",nil,nil,true);err!=nil{t.Fatal(err)}
 visible,err:=ListVisibleCategoriesPage(ctx,db,cfg.DBDriver,8,0);if err!=nil||len(visible)!=1{t.Fatalf("unchanged category was hidden by finalization: %+v %v",visible,err)}
}
