package database

import (
	"context"
	"testing"
	"time"
	"time"

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


func TestCartCheckoutCreatesOneOrderAndConsumesCart(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{DBDriver:"sqlite", DatabaseDSN:":memory:"}
	db, err := Open(ctx,cfg); if err != nil { t.Fatal(err) }; defer db.Close()
	if err := Migrate(ctx,db,cfg.DBDriver); err != nil { t.Fatal(err) }
	first,err:=CreateProduct(ctx,db,cfg.DBDriver,Product{Name:"Card A",PriceMinor:250,Currency:"USDT",DeliveryType:"card",Status:1});if err!=nil{t.Fatal(err)}
	second,err:=CreateProduct(ctx,db,cfg.DBDriver,Product{Name:"Card B",PriceMinor:400,Currency:"USDT",DeliveryType:"card",Status:1});if err!=nil{t.Fatal(err)}
	if _,err:=ImportProductKeys(ctx,db,cfg.DBDriver,first.ID,[]StockSecret{{Ciphertext:"a1",Fingerprint:"a1"}});err!=nil{t.Fatal(err)}
	if _,err:=ImportProductKeys(ctx,db,cfg.DBDriver,second.ID,[]StockSecret{{Ciphertext:"b1",Fingerprint:"b1"}});err!=nil{t.Fatal(err)}
	if err:=AddCartItem(ctx,db,cfg.DBDriver,"bot-a","user-a",first.ID,1);err!=nil{t.Fatal(err)}
	if err:=AddCartItem(ctx,db,cfg.DBDriver,"bot-a","user-a",second.ID,1);err!=nil{t.Fatal(err)}
	order,items,err:=CreateCartCheckout(ctx,db,cfg.DBDriver,"bot-a","user-a","chat-a",20*time.Minute)
	if err!=nil{t.Fatal(err)}
	if len(items)!=2||order.Quantity!=2||order.AmountMinor!=650||order.Status!="pending_payment"{t.Fatalf("unexpected checkout: order=%+v items=%+v",order,items)}
	cart,err:=ListCartItems(ctx,db,cfg.DBDriver,"bot-a","user-a");if err!=nil{t.Fatal(err)}
	if len(cart)!=0{t.Fatalf("cart must be consumed after checkout: %+v",cart)}
	var reserved int
	if err:=db.QueryRowContext(ctx,"SELECT COUNT(1) FROM product_keys WHERE order_id=? AND status=1",order.ID).Scan(&reserved);err!=nil{t.Fatal(err)}
	if reserved!=2{t.Fatalf("expected both product keys reserved by one order, got %d",reserved)}
	saved,err:=ListCartCheckoutItems(ctx,db,cfg.DBDriver,order.ID);if err!=nil{t.Fatal(err)}
	if len(saved)!=2{t.Fatalf("expected two order line snapshots, got %+v",saved)}
}

func TestCartCheckoutRejectsMixedCurrenciesWithoutConsumingCart(t *testing.T) {
	ctx:=context.Background()
	cfg:=config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"}
	db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)};defer db.Close()
	if err:=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}
	a,err:=CreateProduct(ctx,db,cfg.DBDriver,Product{Name:"USDT item",PriceMinor:100,Currency:"USDT",DeliveryType:"card",Status:1});if err!=nil{t.Fatal(err)}
	b,err:=CreateProduct(ctx,db,cfg.DBDriver,Product{Name:"USD item",PriceMinor:100,Currency:"USD",DeliveryType:"card",Status:1});if err!=nil{t.Fatal(err)}
	if _,err:=ImportProductKeys(ctx,db,cfg.DBDriver,a.ID,[]StockSecret{{Ciphertext:"a",Fingerprint:"a"}});err!=nil{t.Fatal(err)}
	if _,err:=ImportProductKeys(ctx,db,cfg.DBDriver,b.ID,[]StockSecret{{Ciphertext:"b",Fingerprint:"b"}});err!=nil{t.Fatal(err)}
	if err:=AddCartItem(ctx,db,cfg.DBDriver,"bot-a","user-a",a.ID,1);err!=nil{t.Fatal(err)}
	if err:=AddCartItem(ctx,db,cfg.DBDriver,"bot-a","user-a",b.ID,1);err!=nil{t.Fatal(err)}
	if _,_,err:=CreateCartCheckout(ctx,db,cfg.DBDriver,"bot-a","user-a","chat-a",20*time.Minute);err==nil{t.Fatal("mixed currencies must be rejected")}
	cart,err:=ListCartItems(ctx,db,cfg.DBDriver,"bot-a","user-a");if err!=nil{t.Fatal(err)}
	if len(cart)!=2{t.Fatalf("failed checkout must retain cart items, got %+v",cart)}
}


func TestCartCheckoutCreatesOneInvoiceAndReservesAllProducts(t *testing.T) {
	ctx:=context.Background()
	cfg:=config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"}
	db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)};defer db.Close()
	if err=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}
	first,err:=CreateProduct(ctx,db,cfg.DBDriver,Product{Name:"First",PriceMinor:250,Currency:"USDT",DeliveryType:"card",Status:1});if err!=nil{t.Fatal(err)}
	second,err:=CreateProduct(ctx,db,cfg.DBDriver,Product{Name:"Second",PriceMinor:400,Currency:"USDT",DeliveryType:"card",Status:1});if err!=nil{t.Fatal(err)}
	if _,err=ImportProductKeys(ctx,db,cfg.DBDriver,first.ID,[]StockSecret{{Ciphertext:"a1",Fingerprint:"a1"},{Ciphertext:"a2",Fingerprint:"a2"}});err!=nil{t.Fatal(err)}
	if _,err=ImportProductKeys(ctx,db,cfg.DBDriver,second.ID,[]StockSecret{{Ciphertext:"b1",Fingerprint:"b1"}});err!=nil{t.Fatal(err)}
	if err=AddCartItem(ctx,db,cfg.DBDriver,"bot","user",first.ID,2);err!=nil{t.Fatal(err)}
	if err=AddCartItem(ctx,db,cfg.DBDriver,"bot","user",second.ID,1);err!=nil{t.Fatal(err)}
	order,lines,err:=CreateCartCheckout(ctx,db,cfg.DBDriver,"bot","user","123",20*time.Minute);if err!=nil{t.Fatal(err)}
	if order.Status!="pending_payment"||order.AmountMinor!=900||order.Quantity!=3||order.PaymentAmountAtomic<9_000_000{t.Fatalf("unexpected checkout order: %+v",order)}
	if len(lines)!=2{t.Fatalf("expected two checkout lines, got %d",len(lines))}
	var reserved int
	if err=db.QueryRowContext(ctx,"SELECT COUNT(1) FROM product_keys WHERE order_id=? AND status=1",order.ID).Scan(&reserved);err!=nil{t.Fatal(err)}
	if reserved!=3{t.Fatalf("expected 3 reserved keys, got %d",reserved)}
	cart,err:=ListCartItems(ctx,db,cfg.DBDriver,"bot","user");if err!=nil{t.Fatal(err)}
	if len(cart)!=0{t.Fatalf("cart should be consumed atomically, got %+v",cart)}
	loaded,err:=ListCartCheckoutItems(ctx,db,cfg.DBDriver,order.ID);if err!=nil{t.Fatal(err)}
	if len(loaded)!=2||loaded[0].LineTotalMinor+loaded[1].LineTotalMinor!=900{t.Fatalf("unexpected order lines: %+v",loaded)}
}


