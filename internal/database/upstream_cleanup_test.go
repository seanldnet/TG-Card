package database

import (
 "context"
 "database/sql"
 "testing"

 "github.com/seanldnet/TG-Card/internal/config"
)

func TestClearUpstreamCatalogPreservesLocalRowsAndOrderSnapshots(t *testing.T) {
 ctx:=context.Background()
 cfg:=config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"}
 db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)};defer db.Close()
 if err:=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}
 localCat,err:=CreateCategory(ctx,db,cfg.DBDriver,"Local category",1);if err!=nil{t.Fatal(err)}
 local,err:=CreateProduct(ctx,db,cfg.DBDriver,Product{CategoryID:localCat.ID,Name:"Local item",PriceMinor:123,Currency:"USDT",DeliveryType:"card",Status:1});if err!=nil{t.Fatal(err)}
 if err:=SaveUpstreamSettings(ctx,db,cfg.DBDriver,UpstreamSettings{Provider:"dujiao_next",BaseURL:"https://supplier.example.com",Enabled:true,CatalogStale:true});err!=nil{t.Fatal(err)}
 if err:=SyncUpstreamCatalog(ctx,db,cfg.DBDriver,"upstream-batch",[]UpstreamCategoryRecord{{UpstreamID:"cat-x",Name:"Upstream category"}},[]UpstreamProductRecord{{ProductID:"up-x",SKUID:"sku-x",CategoryID:"cat-x",Name:"Upstream item",PriceMinor:456,Currency:"USDT",Active:true}});err!=nil{t.Fatal(err)}
 var upstreamID string
 if err:=db.QueryRowContext(ctx,"SELECT id FROM products WHERE source_type='upstream'").Scan(&upstreamID);err!=nil{t.Fatal(err)}
 _,err=db.ExecContext(ctx,"INSERT INTO orders(id,order_no,bot_id,telegram_user_id,telegram_chat_id,product_id,product_name,unit_price_minor,quantity,amount_minor,currency,status,expires_at,created_at,updated_at) VALUES('order-x','ORDER-X','','tg-user','tg-chat',?,'Historical upstream item',456,1,456,'USDT','paid','2030-01-01T00:00:00Z','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')",upstreamID)
 if err!=nil{t.Fatal(err)}
 counts,err:=ClearUpstreamCatalogRows(ctx,db,cfg.DBDriver);if err!=nil{t.Fatal(err)}
 if counts.Categories!=1||counts.Products!=1||counts.SKUs!=1{t.Fatalf("unexpected cleanup counts: %+v",counts)}
 settingsAfterRows,err:=GetUpstreamSettings(ctx,db,cfg.DBDriver);if err!=nil{t.Fatal(err)};if !settingsAfterRows.Enabled{t.Fatal("cleanup row deletion must not switch source off before image cleanup completes")}
 var localCount int
 if err:=db.QueryRowContext(ctx,"SELECT COUNT(1) FROM products WHERE id=? AND source_type='local' AND status=1",local.ID).Scan(&localCount);err!=nil{t.Fatal(err)}
 if localCount!=1{t.Fatal("cleanup touched local product")}
 var productID sql.NullString;var productName string;var unitPrice,amount int64
 if err:=db.QueryRowContext(ctx,"SELECT product_id,product_name,unit_price_minor,amount_minor FROM orders WHERE id='order-x'").Scan(&productID,&productName,&unitPrice,&amount);err!=nil{t.Fatal(err)}
 if productID.Valid||productName!="Historical upstream item"||unitPrice!=456||amount!=456{t.Fatalf("historical order snapshot was not preserved: product=%+v name=%s unit=%d amount=%d",productID,productName,unitPrice,amount)}
}
