package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"
)

type CartCheckoutItem struct {
	LineNumber int `json:"lineNumber"`
	ProductID string `json:"productId"`
	ProductName string `json:"productName"`
	Quantity int `json:"quantity"`
	UnitPriceMinor int64 `json:"unitPriceMinor"`
	LineTotalMinor int64 `json:"lineTotalMinor"`
	Currency string `json:"currency"`
}

func CreateCartCheckout(ctx context.Context, db *sql.DB, driver, botID, telegramUserID, telegramChatID string, ttl time.Duration) (Order, []CartCheckoutItem, error) {
	if ttl <= 0 { ttl = 20*time.Minute }
	tx, err := db.BeginTx(ctx,nil); if err != nil { return Order{},nil,err }; defer tx.Rollback()
	q := `SELECT c.id,c.product_id,c.quantity,p.name,p.price_minor,p.currency,p.status,COALESCE(p.delivery_type,'card'),COALESCE(p.delivery_config_json,'{}'),COALESCE(p.bulk_discount_json,'[]'),COALESCE(p.source_type,'local')
		FROM shopping_cart_items c JOIN products p ON p.id=c.product_id
		WHERE c.bot_id=`+placeholder(driver,1)+` AND c.telegram_user_id=`+placeholder(driver,2)+` ORDER BY c.created_at,c.id`
	if driver=="postgres" { q += " FOR UPDATE OF c,p" }
	rows,err:=tx.QueryContext(ctx,q,botID,telegramUserID);if err!=nil{return Order{},nil,err}
	type liveItem struct{ CartCheckoutItem; cartItemID string; status int; deliveryType,deliveryConfig,discountJSON,sourceType string }
	live:=make([]liveItem,0);for rows.Next(){var x liveItem;if err:=rows.Scan(&x.cartItemID,&x.ProductID,&x.Quantity,&x.ProductName,&x.UnitPriceMinor,&x.Currency,&x.status,&x.deliveryType,&x.deliveryConfig,&x.discountJSON,&x.sourceType);err!=nil{rows.Close();return Order{},nil,err};live=append(live,x)}
	if err:=rows.Err();err!=nil{rows.Close();return Order{},nil,err};if err:=rows.Close();err!=nil{return Order{},nil,err}
	if len(live)==0{return Order{},nil,fmt.Errorf("购物车为空")}
	currency,deliveryType,deliveryConfig:=live[0].Currency,live[0].deliveryType,live[0].deliveryConfig
	if !strings.EqualFold(currency,"USDT") { return Order{},nil,fmt.Errorf("购物车统一结算目前仅支持 USDT 商品") }
	total:=int64(0);totalQty:=0
	for i:=range live {
		x:=&live[i]
		if x.status!=1{return Order{},nil,fmt.Errorf("商品 %s 已下架，请刷新购物车",x.ProductName)}
		if x.sourceType!="local" { return Order{},nil,fmt.Errorf("商品 %s 已切换为上游来源，请单独购买",x.ProductName) }
		if x.deliveryType!="card" && x.deliveryType!="file_card" { return Order{},nil,fmt.Errorf("商品 %s 的交付方式暂不支持购物车统一结算",x.ProductName) }
		if x.Currency!=currency{return Order{},nil,fmt.Errorf("购物车包含多种币种，暂不支持混合结算，请分币种购买")}
		if x.deliveryType!=deliveryType||x.deliveryConfig!=deliveryConfig{return Order{},nil,fmt.Errorf("购物车商品交付方式或交付配置不同，请按相同交付方式分开结算")}
		if len(live)>1 && deliveryType!="card" && deliveryType!="file_card" { return Order{},nil,fmt.Errorf("多商品统一结算目前仅支持卡密或文件卡密类商品；其他交付方式请单独购买") }
		if x.Quantity<1||x.Quantity>100||x.UnitPriceMinor<=0||x.UnitPriceMinor>(1<<63-1)/int64(x.Quantity){return Order{},nil,fmt.Errorf("商品 %s 的数量或金额无效",x.ProductName)}
		needsKeys:=deliveryType=="card"||deliveryType=="file_card"
		if !needsKeys&&x.Quantity!=1{return Order{},nil,fmt.Errorf("商品 %s 的交付方式每笔仅支持 1 件",x.ProductName)}
		line:=x.UnitPriceMinor*int64(x.Quantity)
		var tiers []orderDiscountTier
		if err:=json.Unmarshal([]byte(x.discountJSON),&tiers);err!=nil{return Order{},nil,fmt.Errorf("商品 %s 的批量折扣配置无效",x.ProductName)}
		bestMin,bestPct:=0,0;for _,tier:=range tiers{if tier.MinQuantity>=2&&x.Quantity>=tier.MinQuantity&&tier.DiscountPercent>=1&&tier.DiscountPercent<=90&&tier.MinQuantity>bestMin{bestMin,bestPct=tier.MinQuantity,tier.DiscountPercent}}
		if bestPct>0{factor:=int64(100-bestPct);line=(line/100)*factor+((line%100)*factor+50)/100}
		x.LineTotalMinor=line
		if total> (1<<63-1)-line{return Order{},nil,fmt.Errorf("购物车总金额超出范围")}
		total+=line;totalQty+=x.Quantity
	}
	if totalQty>500{return Order{},nil,fmt.Errorf("购物车总数量最多 500 件")}
	if total<=0||total> ((1<<63-1)-10000)/10000{return Order{},nil,fmt.Errorf("购物车总金额无效或超出付款金额范围")}
	now:=time.Now().UTC();expires:=now.Add(ttl)
	id,err:=randomID();if err!=nil{return Order{},nil,err};suffixID,err:=randomID();if err!=nil{return Order{},nil,err}
	var invoice int64
	base:=total*10000
	for attempt:=0;attempt<40;attempt++{suffix,e:=rand.Int(rand.Reader,big.NewInt(9999));if e!=nil{return Order{},nil,e};candidate:=base+suffix.Int64()+1
		var n,rn int
		if e=tx.QueryRowContext(ctx,"SELECT COUNT(1) FROM orders WHERE status='pending_payment' AND payment_amount_atomic="+placeholder(driver,1),candidate).Scan(&n);e!=nil{return Order{},nil,e}
		if e=tx.QueryRowContext(ctx,"SELECT COUNT(1) FROM wallet_recharges WHERE status='pending_payment' AND payment_amount_atomic="+placeholder(driver,1),candidate).Scan(&rn);e!=nil{return Order{},nil,e}
		if n==0&&rn==0{invoice=candidate;break}
	}
	if invoice==0{return Order{},nil,fmt.Errorf("暂时无法生成唯一付款金额，请稍后重试")}
	order:=Order{ID:id,OrderNo:"TC"+now.Format("20060102150405")+suffixID[:6],BotID:botID,TelegramUserID:telegramUserID,TelegramChatID:telegramChatID,ProductID:live[0].ProductID,ProductName:"购物车（"+fmt.Sprint(len(live))+" 种商品）",UnitPriceMinor:total,Quantity:totalQty,AmountMinor:total,PaymentAmountAtomic:invoice,Currency:currency,Status:"pending_payment",ExpiresAt:expires.Format(time.RFC3339Nano),CreatedAt:now.Format(time.RFC3339Nano),UpdatedAt:now.Format(time.RFC3339Nano),DeliveryType:deliveryType,DeliveryConfigSnapshot:deliveryConfig,DeliveryPayloadJSON:"{}"}
	insert:="INSERT INTO orders(id,order_no,bot_id,telegram_user_id,telegram_chat_id,product_id,product_name,unit_price_minor,quantity,amount_minor,currency,status,expires_at,created_at,updated_at,payment_amount_atomic,delivery_type,delivery_config_snapshot,delivery_payload_json) VALUES("
	for i:=1;i<=19;i++{if i>1{insert+=","};insert+=placeholder(driver,i)};insert+=")"
	if _,err=tx.ExecContext(ctx,insert,order.ID,order.OrderNo,order.BotID,order.TelegramUserID,order.TelegramChatID,order.ProductID,order.ProductName,order.UnitPriceMinor,order.Quantity,order.AmountMinor,order.Currency,order.Status,order.ExpiresAt,order.CreatedAt,order.UpdatedAt,order.PaymentAmountAtomic,order.DeliveryType,order.DeliveryConfigSnapshot,order.DeliveryPayloadJSON);err!=nil{return Order{},nil,err}
	items:=make([]CartCheckoutItem,0,len(live))
	for lineNumber,x:=range live {
		if deliveryType=="card"||deliveryType=="file_card"{
			keyQ:="SELECT id FROM product_keys WHERE product_id="+placeholder(driver,1)+" AND status=0 ORDER BY created_at,id LIMIT "+fmt.Sprint(x.Quantity)
			if driver=="postgres"{keyQ+=" FOR UPDATE SKIP LOCKED"}
			kr,e:=tx.QueryContext(ctx,keyQ,x.ProductID);if e!=nil{return Order{},nil,e};keys:=make([]string,0,x.Quantity)
			for kr.Next(){var kid string;if e=kr.Scan(&kid);e!=nil{kr.Close();return Order{},nil,e};keys=append(keys,kid)}
			if e=kr.Err();e!=nil{kr.Close();return Order{},nil,e};if e=kr.Close();e!=nil{return Order{},nil,e}
			if len(keys)!=x.Quantity{return Order{},nil,fmt.Errorf("商品 %s 库存不足：需要 %d 件，当前可售 %d 件",x.ProductName,x.Quantity,len(keys))}
			for _,kid:=range keys{res,e:=tx.ExecContext(ctx,"UPDATE product_keys SET status=1,order_id="+placeholder(driver,1)+" WHERE id="+placeholder(driver,2)+" AND status=0",order.ID,kid);if e!=nil{return Order{},nil,e};n,e:=res.RowsAffected();if e!=nil||n!=1{if e!=nil{return Order{},nil,e};return Order{},nil,fmt.Errorf("商品 %s 库存预留冲突",x.ProductName)}}
		}
		itemID,e:=randomID();if e!=nil{return Order{},nil,e}
		item:=CartCheckoutItem{LineNumber:lineNumber,ProductID:x.ProductID,ProductName:x.ProductName,Quantity:x.Quantity,UnitPriceMinor:x.UnitPriceMinor,LineTotalMinor:x.LineTotalMinor,Currency:x.Currency}
		_,e=tx.ExecContext(ctx,"INSERT INTO cart_checkout_items(id,order_id,line_number,product_id,product_name,quantity,unit_price_minor,line_total_minor,currency,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+","+placeholder(driver,8)+","+placeholder(driver,9)+","+placeholder(driver,10)+")",itemID,order.ID,lineNumber,item.ProductID,item.ProductName,item.Quantity,item.UnitPriceMinor,item.LineTotalMinor,item.Currency,now.Format(time.RFC3339Nano))
		if e!=nil{return Order{},nil,e};items=append(items,item)
	}
	for _,x:=range live {
		if _,err=tx.ExecContext(ctx,"DELETE FROM shopping_cart_items WHERE id="+placeholder(driver,1)+" AND bot_id="+placeholder(driver,2)+" AND telegram_user_id="+placeholder(driver,3),x.cartItemID,botID,telegramUserID);err!=nil{return Order{},nil,err}
	}
	if err=tx.Commit();err!=nil{return Order{},nil,err}
	return order,items,nil
}

func ListCartCheckoutItems(ctx context.Context,db *sql.DB,driver,orderID string)([]CartCheckoutItem,error){
	q:="SELECT line_number,product_id,product_name,quantity,unit_price_minor,line_total_minor,currency FROM cart_checkout_items WHERE order_id="+placeholder(driver,1)+" ORDER BY line_number,created_at,id"
	rows,err:=db.QueryContext(ctx,q,orderID);if err!=nil{return nil,err};defer rows.Close()
	items:=make([]CartCheckoutItem,0);for rows.Next(){var x CartCheckoutItem;if err:=rows.Scan(&x.LineNumber,&x.ProductID,&x.ProductName,&x.Quantity,&x.UnitPriceMinor,&x.LineTotalMinor,&x.Currency);err!=nil{return nil,err};items=append(items,x)}
	return items,rows.Err()
}


type CartCheckoutDeliveryGroup struct {
	ProductID string
	ProductName string
	Quantity int
	SecretCiphertexts []string
}

// ListCartCheckoutDeliveryGroups preserves the product-to-key relationship for
// one paid cart order so the Bot can label each product's delivered keys.
func ListCartCheckoutDeliveryGroups(ctx context.Context,db *sql.DB,driver,orderID string)([]CartCheckoutDeliveryGroup,error){
	items,err:=ListCartCheckoutItems(ctx,db,driver,orderID)
	if err!=nil{return nil,err}
	if len(items)==0{return nil,nil}
	groups:=make([]CartCheckoutDeliveryGroup,0,len(items))
	for _,item:=range items{
		q:="SELECT secret_ciphertext FROM product_keys WHERE order_id="+placeholder(driver,1)+" AND product_id="+placeholder(driver,2)+" AND status=2 ORDER BY created_at,id"
		rows,err:=db.QueryContext(ctx,q,orderID,item.ProductID)
		if err!=nil{return nil,err}
		group:=CartCheckoutDeliveryGroup{ProductID:item.ProductID,ProductName:item.ProductName,Quantity:item.Quantity,SecretCiphertexts:make([]string,0,item.Quantity)}
		for rows.Next(){var secret string;if err:=rows.Scan(&secret);err!=nil{rows.Close();return nil,err};group.SecretCiphertexts=append(group.SecretCiphertexts,secret)}
		if err:=rows.Err();err!=nil{rows.Close();return nil,err}
		if err:=rows.Close();err!=nil{return nil,err}
		if len(group.SecretCiphertexts)!=item.Quantity{return nil,fmt.Errorf("cart order product %s key count mismatch: expected %d, got %d",item.ProductID,item.Quantity,len(group.SecretCiphertexts))}
		groups=append(groups,group)
	}
	return groups,nil
}
