package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Promotion struct {
	ID string `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
	DiscountType string `json:"discountType"`
	DiscountValue int64 `json:"discountValue"`
	MinimumOrderMinor int64 `json:"minimumOrderMinor"`
	MaximumDiscountMinor int64 `json:"maximumDiscountMinor"`
	StartsAt string `json:"startsAt"`
	EndsAt string `json:"endsAt"`
	UsageLimit int `json:"usageLimit"`
	PerUserLimit int `json:"perUserLimit"`
	UsageCount int `json:"usageCount"`
	Status int `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func normalizePromotionCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func ListPromotions(ctx context.Context, db *sql.DB) ([]Promotion, error) {
	rows, err := db.QueryContext(ctx, `SELECT id,name,code,discount_type,discount_value,minimum_order_minor,maximum_discount_minor,starts_at,ends_at,usage_limit,per_user_limit,usage_count,status,created_at,updated_at FROM promotions ORDER BY created_at DESC LIMIT 500`)
	if err != nil { return nil, err }
	defer rows.Close()
	items := make([]Promotion, 0)
	for rows.Next() {
		var p Promotion
		if err := rows.Scan(&p.ID,&p.Name,&p.Code,&p.DiscountType,&p.DiscountValue,&p.MinimumOrderMinor,&p.MaximumDiscountMinor,&p.StartsAt,&p.EndsAt,&p.UsageLimit,&p.PerUserLimit,&p.UsageCount,&p.Status,&p.CreatedAt,&p.UpdatedAt); err != nil { return nil, err }
		items = append(items,p)
	}
	return items, rows.Err()
}

func SavePromotion(ctx context.Context, db *sql.DB, driver string, p Promotion) (Promotion, error) {
	p.Name = strings.TrimSpace(p.Name)
	p.Code = normalizePromotionCode(p.Code)
	p.DiscountType = strings.TrimSpace(p.DiscountType)
	if p.Name == "" || len(p.Name) > 120 || len(p.Code) < 4 || len(p.Code) > 40 { return Promotion{}, fmt.Errorf("活动名称或优惠码长度无效") }
	if p.DiscountType != "fixed" && p.DiscountType != "percent" { return Promotion{}, fmt.Errorf("优惠类型无效") }
	if p.DiscountValue <= 0 || (p.DiscountType == "percent" && p.DiscountValue > 90) { return Promotion{}, fmt.Errorf("优惠额度无效") }
	if p.MinimumOrderMinor < 0 || p.MaximumDiscountMinor < 0 || p.UsageLimit < 0 || p.PerUserLimit < 1 || p.PerUserLimit > 100 || (p.Status != 0 && p.Status != 1) { return Promotion{}, fmt.Errorf("活动限制参数无效") }
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if p.ID == "" { id, err := randomID(); if err != nil { return Promotion{}, err }; p.ID = id; p.CreatedAt = now }
	p.UpdatedAt = now
	if p.StartsAt != "" { if _,err:=time.Parse(time.RFC3339,p.StartsAt);err!=nil{return Promotion{},fmt.Errorf("开始时间必须使用 RFC3339 格式")} }
	if p.EndsAt != "" { if _,err:=time.Parse(time.RFC3339,p.EndsAt);err!=nil{return Promotion{},fmt.Errorf("结束时间必须使用 RFC3339 格式")} }
	if p.StartsAt != "" && p.EndsAt != "" && p.EndsAt <= p.StartsAt { return Promotion{},fmt.Errorf("结束时间必须晚于开始时间") }
	var exists int
	if err:=db.QueryRowContext(ctx,"SELECT COUNT(1) FROM promotions WHERE id="+placeholder(driver,1),p.ID).Scan(&exists);err!=nil{return Promotion{},err}
	var query string
	if exists > 0 {
		query = "UPDATE promotions SET name="+placeholder(driver,1)+",code="+placeholder(driver,2)+",discount_type="+placeholder(driver,3)+",discount_value="+placeholder(driver,4)+",minimum_order_minor="+placeholder(driver,5)+",maximum_discount_minor="+placeholder(driver,6)+",starts_at="+placeholder(driver,7)+",ends_at="+placeholder(driver,8)+",usage_limit="+placeholder(driver,9)+",per_user_limit="+placeholder(driver,10)+",status="+placeholder(driver,11)+",updated_at="+placeholder(driver,12)+" WHERE id="+placeholder(driver,13)
		_,err:=db.ExecContext(ctx,query,p.Name,p.Code,p.DiscountType,p.DiscountValue,p.MinimumOrderMinor,p.MaximumDiscountMinor,p.StartsAt,p.EndsAt,p.UsageLimit,p.PerUserLimit,p.Status,p.UpdatedAt,p.ID)
		if err!=nil{return Promotion{},fmt.Errorf("保存优惠活动失败，优惠码可能已存在")}
	} else {
		query = "INSERT INTO promotions(id,name,code,discount_type,discount_value,minimum_order_minor,maximum_discount_minor,starts_at,ends_at,usage_limit,per_user_limit,status,created_at,updated_at) VALUES ("+strings.Join([]string{placeholder(driver,1),placeholder(driver,2),placeholder(driver,3),placeholder(driver,4),placeholder(driver,5),placeholder(driver,6),placeholder(driver,7),placeholder(driver,8),placeholder(driver,9),placeholder(driver,10),placeholder(driver,11),placeholder(driver,12),placeholder(driver,13),placeholder(driver,14)},",")+")"
		_,err:=db.ExecContext(ctx,query,p.ID,p.Name,p.Code,p.DiscountType,p.DiscountValue,p.MinimumOrderMinor,p.MaximumDiscountMinor,p.StartsAt,p.EndsAt,p.UsageLimit,p.PerUserLimit,0,p.Status,p.CreatedAt,p.UpdatedAt)
		if err!=nil{return Promotion{},fmt.Errorf("创建优惠活动失败，优惠码可能已存在")}
	}
	return p,nil
}

func DeletePromotion(ctx context.Context, db *sql.DB, driver, id string) error {
	result,err:=db.ExecContext(ctx,"DELETE FROM promotions WHERE id="+placeholder(driver,1),id)
	if err!=nil{return err}
	n,err:=result.RowsAffected();if err!=nil{return err};if n==0{return sql.ErrNoRows};return nil
}

// CalculatePromotionDiscount validates an active promotion and computes a capped discount.
// The caller must reserve the promotion in the same transaction as order creation.
func CalculatePromotionDiscount(ctx context.Context, tx *sql.Tx, driver, code, telegramUserID string, subtotal int64, now time.Time) (Promotion, int64, error) {
	code=normalizePromotionCode(code)
	if code=="" { return Promotion{},0,nil }
	var p Promotion
	query:="SELECT id,name,code,discount_type,discount_value,minimum_order_minor,maximum_discount_minor,starts_at,ends_at,usage_limit,per_user_limit,usage_count,status,created_at,updated_at FROM promotions WHERE code="+placeholder(driver,1)
	err:=tx.QueryRowContext(ctx,query,code).Scan(&p.ID,&p.Name,&p.Code,&p.DiscountType,&p.DiscountValue,&p.MinimumOrderMinor,&p.MaximumDiscountMinor,&p.StartsAt,&p.EndsAt,&p.UsageLimit,&p.PerUserLimit,&p.UsageCount,&p.Status,&p.CreatedAt,&p.UpdatedAt)
	if err!=nil{return Promotion{},0,fmt.Errorf("优惠码不存在或不可用")}
	if p.Status!=1{return Promotion{},0,fmt.Errorf("优惠码已停用")}
	if p.StartsAt!="" && now.Format(time.RFC3339Nano)<p.StartsAt{return Promotion{},0,fmt.Errorf("优惠活动尚未开始")}
	if p.EndsAt!="" && now.Format(time.RFC3339Nano)>p.EndsAt{return Promotion{},0,fmt.Errorf("优惠活动已结束")}
	if subtotal<p.MinimumOrderMinor{return Promotion{},0,fmt.Errorf("订单金额未达到优惠门槛")}
	if p.UsageLimit>0 && p.UsageCount>=p.UsageLimit{return Promotion{},0,fmt.Errorf("优惠码领取次数已用完")}
	var used int
	countQuery:="SELECT COUNT(1) FROM promotion_redemptions WHERE promotion_id="+placeholder(driver,1)+" AND telegram_user_id="+placeholder(driver,2)+" AND status IN ('reserved','redeemed')"
	if err:=tx.QueryRowContext(ctx,countQuery,p.ID,telegramUserID).Scan(&used);err!=nil{return Promotion{},0,err}
	if used>=p.PerUserLimit{return Promotion{},0,fmt.Errorf("你已达到该优惠码的使用次数上限")}
	discount:=int64(0)
	if p.DiscountType=="fixed"{discount=p.DiscountValue}else{discount=(subtotal*p.DiscountValue+50)/100}
	if p.MaximumDiscountMinor>0 && discount>p.MaximumDiscountMinor{discount=p.MaximumDiscountMinor}
	if discount>subtotal{discount=subtotal}
	if discount<=0{return Promotion{},0,fmt.Errorf("优惠金额无效")}
	return p,discount,nil
}

func ReservePromotion(ctx context.Context, tx *sql.Tx, driver string, p Promotion, orderID, telegramUserID string, discount int64, now time.Time) error {
	if p.ID=="" { return nil }
	id,err:=randomID();if err!=nil{return err}
	query:="INSERT INTO promotion_redemptions(id,promotion_id,order_id,telegram_user_id,discount_minor,status,created_at) VALUES ("+strings.Join([]string{placeholder(driver,1),placeholder(driver,2),placeholder(driver,3),placeholder(driver,4),placeholder(driver,5),placeholder(driver,6),placeholder(driver,7)},",")+")"
	if _,err:=tx.ExecContext(ctx,query,id,p.ID,orderID,telegramUserID,discount,"reserved",now.UTC().Format(time.RFC3339Nano));err!=nil{return fmt.Errorf("优惠码使用记录创建失败")}
	update:="UPDATE promotions SET usage_count=usage_count+1,updated_at="+placeholder(driver,1)+" WHERE id="+placeholder(driver,2)+" AND (usage_limit=0 OR usage_count<usage_limit)"
	result,err:=tx.ExecContext(ctx,update,now.UTC().Format(time.RFC3339Nano),p.ID);if err!=nil{return err}
	n,err:=result.RowsAffected();if err!=nil{return err};if n!=1{return fmt.Errorf("优惠码使用次数已达上限")}
	return nil
}

func RedeemOrderPromotion(ctx context.Context, tx *sql.Tx, driver, orderID string, now time.Time) error {
	query:="UPDATE promotion_redemptions SET status='redeemed',redeemed_at="+placeholder(driver,1)+" WHERE order_id="+placeholder(driver,2)+" AND status='reserved'"
	_,err:=tx.ExecContext(ctx,query,now.UTC().Format(time.RFC3339Nano),orderID);return err
}

func ReleaseOrderPromotion(ctx context.Context, tx *sql.Tx, driver, orderID string, now time.Time) error {
	var promotionID string
	query:="SELECT promotion_id FROM promotion_redemptions WHERE order_id="+placeholder(driver,1)+" AND status='reserved'"
	err:=tx.QueryRowContext(ctx,query,orderID).Scan(&promotionID)
	if err==sql.ErrNoRows{return nil};if err!=nil{return err}
	update:="UPDATE promotion_redemptions SET status='released' WHERE order_id="+placeholder(driver,1)+" AND status='reserved'"
	if _,err:=tx.ExecContext(ctx,update,orderID);err!=nil{return err}
	_,err=tx.ExecContext(ctx,"UPDATE promotions SET usage_count=CASE WHEN usage_count>0 THEN usage_count-1 ELSE 0 END,updated_at="+placeholder(driver,1)+" WHERE id="+placeholder(driver,2),now.UTC().Format(time.RFC3339Nano),promotionID)
	return err
}
