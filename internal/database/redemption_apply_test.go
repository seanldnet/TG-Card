package database

import (
 "context"
 "testing"

 "github.com/seanldnet/TG-Card/internal/config"
)

func TestRedeemTelegramCodeBalancePackageAndProduct(t *testing.T){
 ctx:=context.Background()
 cfg:=config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"}
 db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)};defer db.Close()
 if err:=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}
 if err:=UpsertTelegramUser(ctx,db,cfg.DBDriver,"10001","buyer","Buyer","","bot-a");err!=nil{t.Fatal(err)}
 balanceCode,err:=SaveRedemptionCode(ctx,db,cfg.DBDriver,RedemptionCode{Name:"Balance gift",Code:"BALANCE10",RewardType:"balance",AmountAtomic:10_000_000,Quantity:1,UsageLimit:1,PerUserLimit:1,Status:1})
 if err!=nil{t.Fatal(err)}
 result,err:=RedeemTelegramCode(ctx,db,cfg.DBDriver,"bot-a","10001","10001",balanceCode.Code)
 if err!=nil{t.Fatal(err)}
 if result.AmountAtomic!=10_000_000{t.Fatalf("unexpected balance reward: %+v",result)}
 balance,err:=GetTelegramWalletBalance(ctx,db,cfg.DBDriver,"10001");if err!=nil{t.Fatal(err)}
 if balance!=10_000_000{t.Fatalf("expected wallet credit, got %d",balance)}
 if _,err:=RedeemTelegramCode(ctx,db,cfg.DBDriver,"bot-a","10001","10001",balanceCode.Code);err==nil{t.Fatal("one-use code must not be redeemed twice")}

 packageCode,err:=SaveRedemptionCode(ctx,db,cfg.DBDriver,RedemptionCode{Name:"Pro package",Code:"PRO30DAY",RewardType:"package",TargetName:"Professional",Quantity:1,DurationDays:30,UsageLimit:5,PerUserLimit:1,Status:1})
 if err!=nil{t.Fatal(err)}
 packageResult,err:=RedeemTelegramCode(ctx,db,cfg.DBDriver,"bot-a","10001","10001",packageCode.Code);if err!=nil{t.Fatal(err)}
 if packageResult.PackageName!="Professional"||packageResult.PackageDays!=30{t.Fatalf("unexpected package reward: %+v",packageResult)}
 entitlements,err:=ListTelegramPackageEntitlements(ctx,db,cfg.DBDriver,"10001","bot-a");if err!=nil{t.Fatal(err)}
 if len(entitlements)!=1||entitlements[0].PackageName!="Professional"{t.Fatalf("package entitlement missing: %+v",entitlements)}

 product,err:=CreateProduct(ctx,db,cfg.DBDriver,Product{Name:"Redeemable card",PriceMinor:500,Currency:"USDT",DeliveryType:"card",Status:1});if err!=nil{t.Fatal(err)}
 if _,err:=ImportProductKeys(ctx,db,cfg.DBDriver,product.ID,[]StockSecret{{Ciphertext:"encrypted-card",Fingerprint:"fp-card"}});err!=nil{t.Fatal(err)}
 productCode,err:=SaveRedemptionCode(ctx,db,cfg.DBDriver,RedemptionCode{Name:"Free product",Code:"FREECARD",RewardType:"product",TargetID:product.ID,Quantity:1,UsageLimit:2,PerUserLimit:1,Status:1});if err!=nil{t.Fatal(err)}
 productResult,err:=RedeemTelegramCode(ctx,db,cfg.DBDriver,"bot-a","10001","10001",productCode.Code);if err!=nil{t.Fatal(err)}
 delivery,err:=GetOrderDelivery(ctx,db,cfg.DBDriver,productResult.ProductOrderID);if err!=nil{t.Fatal(err)}
 if delivery.Order.Status!="paid"||delivery.Order.AmountMinor!=0||delivery.SecretCiphertext!="encrypted-card"{t.Fatalf("product redemption did not create a paid zero-cost delivery: %+v",delivery)}
}
