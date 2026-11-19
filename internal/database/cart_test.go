package database

import (
	"context"
	"strings"
	"testing"
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


func TestShoppingCartRejectsNonKeyEntitlements(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{DBDriver: "sqlite", DatabaseDSN: ":memory:"}
	db, err := Open(ctx, cfg)
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := Migrate(ctx, db, cfg.DBDriver); err != nil { t.Fatal(err) }
	product, err := CreateProduct(ctx, db, cfg.DBDriver, Product{Name: "Fixed text", PriceMinor: 500, Currency: "USDT", DeliveryType: "fixed_text", Status: 1})
	if err != nil { t.Fatal(err) }
	if err := AddCartItem(ctx, db, cfg.DBDriver, "bot-a", "user-a", product.ID, 1); err == nil {
		t.Fatal("non-key delivery must be rejected from the cart")
	}
	items, err := ListCartItems(ctx, db, cfg.DBDriver, "bot-a", "user-a")
	if err != nil { t.Fatal(err) }
	if len(items) != 0 { t.Fatalf("non-key product unexpectedly entered cart: %+v", items) }
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
	if len(saved)!=2||saved[0].ProductName!="Card A"||saved[1].ProductName!="Card B"||saved[0].LineNumber!=0||saved[1].LineNumber!=1{t.Fatalf("expected ordered product line snapshots, got %+v",saved)}
	paid,err:=ConfirmOrderPayment(ctx,db,cfg.DBDriver,order.ID,"tron","cart-checkout-tx")
	if err!=nil{t.Fatal(err)}
	if paid.Order.Status!="paid"||len(strings.Split(paid.SecretCiphertext,"\n"))!=2{t.Fatalf("unified payment did not include both product keys: %+v",paid)}
	groups,err:=ListCartCheckoutDeliveryGroups(ctx,db,cfg.DBDriver,order.ID);if err!=nil{t.Fatal(err)}
	if len(groups)!=2||groups[0].ProductName==""||len(groups[0].SecretCiphertexts)!=1||len(groups[1].SecretCiphertexts)!=1{t.Fatalf("product-to-key delivery grouping was not preserved: %+v",groups)}
	var sold int
	if err:=db.QueryRowContext(ctx,"SELECT COUNT(1) FROM product_keys WHERE order_id=? AND status=2",order.ID).Scan(&sold);err!=nil{t.Fatal(err)}
	if sold!=2{t.Fatalf("expected both keys marked sold after one payment, got %d",sold)}
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




func TestCartCheckoutInsufficientStockIsAtomic(t *testing.T) {
	ctx:=context.Background()
	cfg:=config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"}
	db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)};defer db.Close()
	if err:=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}
	a,err:=CreateProduct(ctx,db,cfg.DBDriver,Product{Name:"Enough",PriceMinor:100,Currency:"USDT",DeliveryType:"card",Status:1});if err!=nil{t.Fatal(err)}
	b,err:=CreateProduct(ctx,db,cfg.DBDriver,Product{Name:"Short",PriceMinor:200,Currency:"USDT",DeliveryType:"card",Status:1});if err!=nil{t.Fatal(err)}
	if _,err:=ImportProductKeys(ctx,db,cfg.DBDriver,a.ID,[]StockSecret{{Ciphertext:"a",Fingerprint:"a"}});err!=nil{t.Fatal(err)}
	if err:=AddCartItem(ctx,db,cfg.DBDriver,"bot","user",a.ID,1);err!=nil{t.Fatal(err)}
	if err:=AddCartItem(ctx,db,cfg.DBDriver,"bot","user",b.ID,1);err!=nil{t.Fatal(err)}
	if _,_,err:=CreateCartCheckout(ctx,db,cfg.DBDriver,"bot","user","chat",20*time.Minute);err==nil{t.Fatal("checkout should fail when any line lacks stock")}
	items,err:=ListCartItems(ctx,db,cfg.DBDriver,"bot","user");if err!=nil{t.Fatal(err)}
	if len(items)!=2{t.Fatalf("failed checkout must retain all cart lines, got %d",len(items))}
	var reserved int
	if err:=db.QueryRowContext(ctx,"SELECT COUNT(1) FROM product_keys WHERE status=1").Scan(&reserved);err!=nil{t.Fatal(err)}
	if reserved!=0{t.Fatalf("failed checkout partially reserved %d keys",reserved)}
	var orders int
	if err:=db.QueryRowContext(ctx,"SELECT COUNT(1) FROM orders").Scan(&orders);err!=nil{t.Fatal(err)}
	if orders!=0{t.Fatalf("failed checkout created %d orders",orders)}
}

func TestCartRejectsUpstreamAndNonCardProducts(t *testing.T) {
	ctx:=context.Background()
	cfg:=config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"}
	db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)};defer db.Close()
	if err:=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}
	upstream,err:=CreateProduct(ctx,db,cfg.DBDriver,Product{Name:"Supplier item",PriceMinor:100,Currency:"USDT",DeliveryType:"card",Status:1});if err!=nil{t.Fatal(err)}
	if _,err:=db.ExecContext(ctx,"UPDATE products SET source_type='upstream' WHERE id=?",upstream.ID);err!=nil{t.Fatal(err)}
	if err:=AddCartItem(ctx,db,cfg.DBDriver,"bot","user",upstream.ID,1);err==nil{t.Fatal("upstream products must not enter local cart checkout")}
	fixed,err:=CreateProduct(ctx,db,cfg.DBDriver,Product{Name:"Fixed text",PriceMinor:100,Currency:"USDT",DeliveryType:"fixed_text",Status:1});if err!=nil{t.Fatal(err)}
	if err:=AddCartItem(ctx,db,cfg.DBDriver,"bot","user",fixed.ID,1);err==nil{t.Fatal("non-card delivery must not enter cart checkout")}
}

func TestAdjustCartItemQuantityIsAtomicAndBounded(t *testing.T) {
	ctx:=context.Background()
	cfg:=config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"}
	db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)};defer db.Close()
	if err:=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}
	p,err:=CreateProduct(ctx,db,cfg.DBDriver,Product{Name:"Card",PriceMinor:100,Currency:"USDT",DeliveryType:"card",Status:1});if err!=nil{t.Fatal(err)}
	if err:=AddCartItem(ctx,db,cfg.DBDriver,"bot","user",p.ID,1);err!=nil{t.Fatal(err)}
	if err:=AdjustCartItemQuantity(ctx,db,cfg.DBDriver,"bot","user",p.ID,1);err!=nil{t.Fatal(err)}
	items,err:=ListCartItems(ctx,db,cfg.DBDriver,"bot","user");if err!=nil{t.Fatal(err)}
	if len(items)!=1||items[0].Quantity!=2{t.Fatalf("increment failed: %+v",items)}
	if err:=AdjustCartItemQuantity(ctx,db,cfg.DBDriver,"bot","user",p.ID,-1);err!=nil{t.Fatal(err)}
	if err:=AdjustCartItemQuantity(ctx,db,cfg.DBDriver,"bot","user",p.ID,-1);err!=nil{t.Fatal(err)}
	items,err=ListCartItems(ctx,db,cfg.DBDriver,"bot","user");if err!=nil{t.Fatal(err)}
	if len(items)!=0{t.Fatalf("decrement at one should remove item: %+v",items)}
}


func TestCartCheckoutRejectsNonUSDTInvoiceCurrency(t *testing.T) {
	ctx:=context.Background()
	cfg:=config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"}
	db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)};defer db.Close()
	if err:=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}
	p,err:=CreateProduct(ctx,db,cfg.DBDriver,Product{Name:"USD card",PriceMinor:100,Currency:"USD",DeliveryType:"card",Status:1});if err!=nil{t.Fatal(err)}
	if _,err:=ImportProductKeys(ctx,db,cfg.DBDriver,p.ID,[]StockSecret{{Ciphertext:"usd-secret",Fingerprint:"usd-fp"}});err!=nil{t.Fatal(err)}
	if err:=AddCartItem(ctx,db,cfg.DBDriver,"bot","user",p.ID,1);err!=nil{t.Fatal(err)}
	if _,_,err:=CreateCartCheckout(ctx,db,cfg.DBDriver,"bot","user","chat",20*time.Minute);err==nil{t.Fatal("non-USDT cart checkout must be rejected")}
	items,err:=ListCartItems(ctx,db,cfg.DBDriver,"bot","user");if err!=nil{t.Fatal(err)}
	if len(items)!=1{t.Fatalf("rejected checkout must retain cart, got %+v",items)}
}


func TestCartCheckoutWalletPaymentDebitsOnceAndSellsAllKeys(t *testing.T) {
	ctx:=context.Background()
	cfg:=config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"}
	db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)};defer db.Close()
	if err:=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}
	if _,err:=db.ExecContext(ctx,"INSERT INTO telegram_users(id,telegram_user_id,username,first_name,last_name,last_seen_bot_id,balance_atomic,created_at,updated_at) VALUES('u1','user-1','buyer','','','bot-1',10000000,'now','now')");err!=nil{t.Fatal(err)}
	a,err:=CreateProduct(ctx,db,cfg.DBDriver,Product{Name:"Wallet A",PriceMinor:250,Currency:"USDT",DeliveryType:"card",Status:1});if err!=nil{t.Fatal(err)}
	b,err:=CreateProduct(ctx,db,cfg.DBDriver,Product{Name:"Wallet B",PriceMinor:400,Currency:"USDT",DeliveryType:"card",Status:1});if err!=nil{t.Fatal(err)}
	if _,err:=ImportProductKeys(ctx,db,cfg.DBDriver,a.ID,[]StockSecret{{Ciphertext:"a",Fingerprint:"a"}});err!=nil{t.Fatal(err)}
	if _,err:=ImportProductKeys(ctx,db,cfg.DBDriver,b.ID,[]StockSecret{{Ciphertext:"b",Fingerprint:"b"}});err!=nil{t.Fatal(err)}
	if err:=AddCartItem(ctx,db,cfg.DBDriver,"bot-1","user-1",a.ID,1);err!=nil{t.Fatal(err)}
	if err:=AddCartItem(ctx,db,cfg.DBDriver,"bot-1","user-1",b.ID,1);err!=nil{t.Fatal(err)}
	order,_,err:=CreateCartCheckout(ctx,db,cfg.DBDriver,"bot-1","user-1","chat-1",20*time.Minute);if err!=nil{t.Fatal(err)}
	delivery,balance,err:=PayOrderWithWallet(ctx,db,cfg.DBDriver,order.ID);if err!=nil{t.Fatal(err)}
	if balance!=3500000||delivery.Order.Status!="paid"||len(strings.Split(delivery.SecretCiphertext,"\n"))!=2{t.Fatalf("unexpected wallet checkout result: balance=%d order=%+v delivery=%q",balance,delivery.Order,delivery.SecretCiphertext)}
	var debits int
	if err:=db.QueryRowContext(ctx,"SELECT COUNT(1) FROM wallet_transactions WHERE direction='debit' AND note LIKE ?", "商城订单 "+order.OrderNo).Scan(&debits);err!=nil{t.Fatal(err)}
	if debits!=1{t.Fatalf("expected exactly one wallet debit, got %d",debits)}
	var sold int
	if err:=db.QueryRowContext(ctx,"SELECT COUNT(1) FROM product_keys WHERE order_id=? AND status=2",order.ID).Scan(&sold);err!=nil{t.Fatal(err)}
	if sold!=2{t.Fatalf("expected both cart keys sold after one wallet payment, got %d",sold)}
}


func TestCartCheckoutRejectsInvoiceAmountOverflow(t *testing.T) {
	ctx:=context.Background()
	cfg:=config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"}
	db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)};defer db.Close()
	if err:=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}
	p,err:=CreateProduct(ctx,db,cfg.DBDriver,Product{Name:"Huge card",PriceMinor:922337203685477,Currency:"USDT",DeliveryType:"card",Status:1});if err!=nil{t.Fatal(err)}
	if _,err:=ImportProductKeys(ctx,db,cfg.DBDriver,p.ID,[]StockSecret{{Ciphertext:"huge-secret",Fingerprint:"huge-fp"}});err!=nil{t.Fatal(err)}
	if err:=AddCartItem(ctx,db,cfg.DBDriver,"bot","user",p.ID,1);err!=nil{t.Fatal(err)}
	if _,_,err:=CreateCartCheckout(ctx,db,cfg.DBDriver,"bot","user","chat",20*time.Minute);err==nil{t.Fatal("checkout must reject payment invoice overflow")}
	items,err:=ListCartItems(ctx,db,cfg.DBDriver,"bot","user");if err!=nil{t.Fatal(err)}
	if len(items)!=1{t.Fatalf("overflow rejection must retain cart, got %+v",items)}
}
