package database

import (
 "context"
 "database/sql"
 "fmt"
 "math"
 "strings"
 "time"
)

type RedemptionResult struct {
 RewardType string
 AmountAtomic int64
 PackageName string
 PackageDays int
 PackageQuantity int
 ProductOrderID string
}

func RedeemTelegramCode(ctx context.Context,db *sql.DB,driver,botID,telegramUserID,telegramChatID,rawCode string)(RedemptionResult,error){
 code:=strings.ToUpper(strings.TrimSpace(rawCode));if len(code)<4||len(code)>40{return RedemptionResult{},fmt.Errorf("兑换码格式无效")}
 tx,err:=db.BeginTx(ctx,nil);if err!=nil{return RedemptionResult{},err};defer tx.Rollback()
 q:="SELECT id,reward_type,amount_atomic,target_id,target_name,quantity,duration_days,usage_limit,per_user_limit,usage_count,starts_at,ends_at,status FROM redemption_codes WHERE code="+placeholder(driver,1)
 if driver=="postgres"{q+=" FOR UPDATE"}
 var id,kind,targetID,targetName,starts,ends string;var amount int64;var quantity,duration,limit,perUser,used,status int
 if err:=tx.QueryRowContext(ctx,q,code).Scan(&id,&kind,&amount,&targetID,&targetName,&quantity,&duration,&limit,&perUser,&used,&starts,&ends,&status);err!=nil{return RedemptionResult{},fmt.Errorf("兑换码不存在或已失效")}
 now:=time.Now().UTC();stamp:=now.Format(time.RFC3339Nano)
 if status!=1{return RedemptionResult{},fmt.Errorf("兑换码已停用")}
 if starts!=""{t,e:=time.Parse(time.RFC3339,starts);if e!=nil||now.Before(t){return RedemptionResult{},fmt.Errorf("兑换码尚未生效")}}
 if ends!=""{t,e:=time.Parse(time.RFC3339,ends);if e!=nil||now.After(t){return RedemptionResult{},fmt.Errorf("兑换码已过期")}}
 if limit>0&&used>=limit{return RedemptionResult{},fmt.Errorf("兑换码次数已用完")}
 var perUserUsed int
 if err:=tx.QueryRowContext(ctx,"SELECT COUNT(1) FROM redemption_code_uses WHERE code_id="+placeholder(driver,1)+" AND telegram_user_id="+placeholder(driver,2),id,telegramUserID).Scan(&perUserUsed);err!=nil{return RedemptionResult{},err}
 if perUserUsed>=perUser{return RedemptionResult{},fmt.Errorf("你已达到该兑换码的个人兑换次数上限")}
 var result RedemptionResult;result.RewardType=kind
 switch kind{
 case "balance":
  if amount<1||amount>math.MaxInt64/2{return result,fmt.Errorf("兑换奖励金额无效")}
  update:="UPDATE telegram_users SET balance_atomic=balance_atomic+"+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+" WHERE telegram_user_id="+placeholder(driver,3)
  res,e:=tx.ExecContext(ctx,update,amount,stamp,telegramUserID);if e!=nil{return result,e};n,e:=res.RowsAffected();if e!=nil||n!=1{if e!=nil{return result,e};return result,fmt.Errorf("请先向机器人发送 /start")}
  var balance int64;if e=tx.QueryRowContext(ctx,"SELECT balance_atomic FROM telegram_users WHERE telegram_user_id="+placeholder(driver,1),telegramUserID).Scan(&balance);e!=nil{return result,e}
  ledgerID,e:=randomID();if e!=nil{return result,e}
  ledger:="INSERT INTO wallet_transactions(id,telegram_user_id,admin_id,direction,amount_atomic,balance_after_atomic,note,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+",'','credit',"+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+")"
  if _,e=tx.ExecContext(ctx,ledger,ledgerID,telegramUserID,amount,balance,"兑换码奖励 "+code,stamp);e!=nil{return result,e}
  result.AmountAtomic=amount
 case "package":
  entID,e:=randomID();if e!=nil{return result,e}
  expires:="";if duration>0{expires=now.Add(time.Duration(duration)*24*time.Hour).Format(time.RFC3339Nano)}
  insert:="INSERT INTO telegram_package_entitlements(id,telegram_user_id,bot_id,package_name,quantity,granted_days,expires_at,source_code_id,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+","+placeholder(driver,8)+","+placeholder(driver,9)+")"
  if _,e=tx.ExecContext(ctx,insert,entID,telegramUserID,botID,targetName,quantity,duration,expires,id,stamp);e!=nil{return result,e}
  result.PackageName=targetName;result.PackageDays=duration;result.PackageQuantity=quantity
 case "product":
  if targetID==""||quantity<1||quantity>100{return result,fmt.Errorf("商品兑换配置无效")}
  var productName,currency,deliveryType,deliveryConfig string;var price int64;var productStatus int
  productQuery:="SELECT name,price_minor,currency,status,COALESCE(delivery_type,'card'),COALESCE(delivery_config_json,'{}') FROM products WHERE id="+placeholder(driver,1)
  if e:=tx.QueryRowContext(ctx,productQuery,targetID).Scan(&productName,&price,&currency,&productStatus,&deliveryType,&deliveryConfig);e!=nil||productStatus!=1{return result,fmt.Errorf("兑换商品不存在或已下架")}
  if deliveryType!="card"&&deliveryType!="file_card"{return result,fmt.Errorf("当前兑换码商品奖励仅支持卡密或文件卡密交付")}
  keyQuery:="SELECT id FROM product_keys WHERE product_id="+placeholder(driver,1)+" AND status=0 ORDER BY created_at,id LIMIT "+fmt.Sprint(quantity)
  if driver=="postgres"{keyQuery+=" FOR UPDATE SKIP LOCKED"}
  rows,e:=tx.QueryContext(ctx,keyQuery,targetID);if e!=nil{return result,e}
  ids:=make([]string,0,quantity);for rows.Next(){var key string;if e=rows.Scan(&key);e!=nil{rows.Close();return result,e};ids=append(ids,key)}
  if e=rows.Err();e!=nil{rows.Close();return result,e};rows.Close()
  if len(ids)!=quantity{return result,fmt.Errorf("商品库存不足，暂时无法兑换")}
  orderID,e:=randomID();if e!=nil{return result,e};suffix,e:=randomID();if e!=nil{return result,e};digits,e:=randomDigits(6);if e!=nil{return result,e}
  orderNo:="TC"+now.Format("20060102150405")+digits
  paymentRef:="REDEEM"+suffix[:12]
  insertOrder:="INSERT INTO orders(id,order_no,bot_id,telegram_user_id,telegram_chat_id,product_id,product_name,unit_price_minor,quantity,amount_minor,currency,status,expires_at,created_at,updated_at,paid_at,payment_method,payment_reference,payment_amount_atomic,delivery_type,delivery_config_snapshot,delivery_payload_json) VALUES("
  for i:=1;i<=22;i++{if i>1{insertOrder+=","};insertOrder+=placeholder(driver,i)};insertOrder+=")"
  _,e=tx.ExecContext(ctx,insertOrder,orderID,orderNo,botID,telegramUserID,telegramChatID,targetID,productName,price,quantity,0,currency,"paid",stamp,stamp,stamp,stamp,"redemption_code",paymentRef,int64(0),deliveryType,deliveryConfig,"{}")
  if e!=nil{return result,e}
  for _,keyID:=range ids{u:="UPDATE product_keys SET status=2,order_id="+placeholder(driver,1)+",sold_at="+placeholder(driver,2)+" WHERE id="+placeholder(driver,3)+" AND status=0";res,e:=tx.ExecContext(ctx,u,orderID,stamp,keyID);if e!=nil{return result,e};n,e:=res.RowsAffected();if e!=nil||n!=1{if e!=nil{return result,e};return result,fmt.Errorf("商品库存状态已变化，请重试")}}
  result.ProductOrderID=orderID
 default:return result,fmt.Errorf("不支持的兑换奖励类型")
 }
 useID,err:=randomID();if err!=nil{return result,err}
 useQ:="INSERT INTO redemption_code_uses(id,code_id,telegram_user_id,bot_id,reward_type,reward_value,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+")"
 rewardValue:=fmt.Sprint(result.AmountAtomic);if kind=="package"{rewardValue=result.PackageName};if kind=="product"{rewardValue=result.ProductOrderID}
 if _,err=tx.ExecContext(ctx,useQ,useID,id,telegramUserID,botID,kind,rewardValue,stamp);err!=nil{return result,err}
 upd:="UPDATE redemption_codes SET usage_count=usage_count+1,updated_at="+placeholder(driver,1)+" WHERE id="+placeholder(driver,2)+" AND (usage_limit=0 OR usage_count<usage_limit)"
 res,err:=tx.ExecContext(ctx,upd,stamp,id);if err!=nil{return result,err};n,err:=res.RowsAffected();if err!=nil||n!=1{if err!=nil{return result,err};return result,fmt.Errorf("兑换次数已用完")}
 if err=tx.Commit();err!=nil{return result,err}
 return result,nil
}
