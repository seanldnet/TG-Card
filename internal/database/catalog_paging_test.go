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
