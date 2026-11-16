package database

import (
	"context"
	"testing"

	"github.com/seanldnet/TG-Card/internal/config"
)

func TestShoppingCartIsScopedAndQuantityBounded(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"}
	db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)};defer db.Close()
	if err:=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}
	product,err:=CreateProduct(ctx,db,cfg.DBDriver,Product{Name:"Cart card",PriceMinor:250,Currency:"USDT",DeliveryType:"card",Status:1})
	if err!=nil{t.Fatal(err)}
	if _,err:=ImportProductKeys(ctx,db,cfg.DBDriver,product.ID,[]StockSecret{{Ciphertext:"secret-1",Fingerprint:"fp-1"},{Ciphertext:"secret-2",Fingerprint:"fp-2"}});err!=nil{t.Fatal(err)}
	if err:=AddCartItem(ctx,db,cfg.DBDriver,"bot-a","user-a",product.ID,2);err!=nil{t.Fatal(err)}
	if err:=AddCartItem(ctx,db,cfg.DBDriver,"bot-a","user-a",product.ID,1);err!=nil{t.Fatal(err)}
	items,err:=ListCartItems(ctx,db,cfg.DBDriver,"bot-a","user-a");if err!=nil{t.Fatal(err)}
	if len(items)!=1||items[0].Quantity!=3||items[0].LineTotalMinor!=750||items[0].StockCount!=2{t.Fatalf("unexpected cart item: %+v",items[0])}
	if err:=AddCartItem(ctx,db,cfg.DBDriver,"bot-b","user-a",product.ID,1);err!=nil{t.Fatal(err)}
	otherBot,err:=ListCartItems(ctx,db,cfg.DBDriver,"bot-b","user-a");if err!=nil{t.Fatal(err)}
	if len(otherBot)!=1||otherBot[0].Quantity!=1{t.Fatalf("bot cart was not isolated: %+v",otherBot)}
	if err:=AddCartItem(ctx,db,cfg.DBDriver,"bot-a","user-a",product.ID,98);err==nil{t.Fatal("cart quantity above 100 should be rejected")}
	if err:=SetCartItemQuantity(ctx,db,cfg.DBDriver,"bot-a","user-a",product.ID,4);err!=nil{t.Fatal(err)}
	if err:=RemoveCartItem(ctx,db,cfg.DBDriver,"bot-a","user-a",product.ID);err!=nil{t.Fatal(err)}
	items,err=ListCartItems(ctx,db,cfg.DBDriver,"bot-a","user-a");if err!=nil{t.Fatal(err)}
	if len(items)!=0{t.Fatalf("expected removed item, got %+v",items)}
	if err:=ClearCart(ctx,db,cfg.DBDriver,"bot-b","user-a");err!=nil{t.Fatal(err)}
}


func TestShoppingCartDoesNotStackNonKeyEntitlements(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{DBDriver: "sqlite", DatabaseDSN: ":memory:"}
	db, err := Open(ctx, cfg)
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := Migrate(ctx, db, cfg.DBDriver); err != nil { t.Fatal(err) }
	product, err := CreateProduct(ctx, db, cfg.DBDriver, Product{Name: "Fixed text", PriceMinor: 500, Currency: "USDT", DeliveryType: "fixed_text", Status: 1})
	if err != nil { t.Fatal(err) }
	if err := AddCartItem(ctx, db, cfg.DBDriver, "bot-a", "user-a", product.ID, 1); err != nil { t.Fatal(err) }
	if err := AddCartItem(ctx, db, cfg.DBDriver, "bot-a", "user-a", product.ID, 1); err == nil {
		t.Fatal("non-key entitlement must not be stacked in the cart")
	}
	items, err := ListCartItems(ctx, db, cfg.DBDriver, "bot-a", "user-a")
	if err != nil { t.Fatal(err) }
	if len(items) != 1 || items[0].Quantity != 1 || items[0].LineTotalMinor != 500 {
		t.Fatalf("unexpected non-key cart item: %+v", items)
	}
}
