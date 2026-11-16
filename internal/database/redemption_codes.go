package database

import (
 "context"
 "database/sql"
 "fmt"
 "strings"
 "time"
)

type RedemptionCode struct {
 ID string `json:"id"`
 Name string `json:"name"`
 Code string `json:"code"`
 RewardType string `json:"rewardType"`
 AmountAtomic int64 `json:"amountAtomic"`
 TargetID string `json:"targetId"`
 TargetName string `json:"targetName"`
 Quantity int `json:"quantity"`
 DurationDays int `json:"durationDays"`
 UsageLimit int `json:"usageLimit"`
 PerUserLimit int `json:"perUserLimit"`
 UsageCount int `json:"usageCount"`
 StartsAt string `json:"startsAt"`
 EndsAt string `json:"endsAt"`
 Status int `json:"status"`
 CreatedAt string `json:"createdAt"`
 UpdatedAt string `json:"updatedAt"`
}

func ListRedemptionCodes(ctx context.Context,db *sql.DB,driver string)([]RedemptionCode,error){
 rows,err:=db.QueryContext(ctx,"SELECT id,name,code,reward_type,amount_atomic,target_id,target_name,quantity,duration_days,usage_limit,per_user_limit,usage_count,starts_at,ends_at,status,created_at,updated_at FROM redemption_codes ORDER BY created_at DESC LIMIT 500")
 if err!=nil{return nil,err};defer rows.Close()
 out:=make([]RedemptionCode,0)
 for rows.Next(){var v RedemptionCode;if err:=rows.Scan(&v.ID,&v.Name,&v.Code,&v.RewardType,&v.AmountAtomic,&v.TargetID,&v.TargetName,&v.Quantity,&v.DurationDays,&v.UsageLimit,&v.PerUserLimit,&v.UsageCount,&v.StartsAt,&v.EndsAt,&v.Status,&v.CreatedAt,&v.UpdatedAt);err!=nil{return nil,err};out=append(out,v)}
 return out,rows.Err()
}
func SaveRedemptionCode(ctx context.Context,db *sql.DB,driver string,v RedemptionCode)(RedemptionCode,error){
 v.Name=strings.TrimSpace(v.Name);v.Code=strings.ToUpper(strings.TrimSpace(v.Code));v.TargetID=strings.TrimSpace(v.TargetID);v.TargetName=strings.TrimSpace(v.TargetName)
 if v.Name==""||len(v.Name)>120||len(v.Code)<4||len(v.Code)>40{return v,fmt.Errorf("名称或兑换码长度无效")}
 if v.RewardType!="balance"&&v.RewardType!="package"&&v.RewardType!="product"{return v,fmt.Errorf("奖励类型无效")}
 if v.RewardType=="balance"&&(v.AmountAtomic<1_000_000||v.AmountAtomic>1_000_000_000_000){return v,fmt.Errorf("USDT 奖励必须在 1 至 1,000,000 之间")}
 if v.RewardType=="package"&&(v.TargetName==""||v.DurationDays<0||v.DurationDays>36500||v.Quantity<1||v.Quantity>10000){return v,fmt.Errorf("套餐名称、数量或有效天数无效")}
 if v.RewardType=="product"&&(v.TargetID==""||v.Quantity<1||v.Quantity>100){return v,fmt.Errorf("请选择商品并设置有效兑换数量")}
 if v.UsageLimit<0||v.PerUserLimit<1||v.PerUserLimit>100||(v.Status!=0&&v.Status!=1){return v,fmt.Errorf("兑换次数或状态设置无效")}
 if v.StartsAt!=""{if _,e:=time.Parse(time.RFC3339,v.StartsAt);e!=nil{return v,fmt.Errorf("开始时间须为 RFC3339")}}
 if v.EndsAt!=""{if _,e:=time.Parse(time.RFC3339,v.EndsAt);e!=nil{return v,fmt.Errorf("结束时间须为 RFC3339")}}
 if v.StartsAt!=""&&v.EndsAt!=""&&v.EndsAt<=v.StartsAt{return v,fmt.Errorf("结束时间必须晚于开始时间")}
 now:=time.Now().UTC().Format(time.RFC3339Nano)
 if v.ID==""{id,e:=randomID();if e!=nil{return v,e};v.ID=id;v.CreatedAt=now}
 v.UpdatedAt=now
 args:=[]any{v.Name,v.Code,v.RewardType,v.AmountAtomic,v.TargetID,v.TargetName,v.Quantity,v.DurationDays,v.UsageLimit,v.PerUserLimit,v.StartsAt,v.EndsAt,v.Status,v.UpdatedAt,v.ID}
 q:="UPDATE redemption_codes SET name="+placeholder(driver,1)+",code="+placeholder(driver,2)+",reward_type="+placeholder(driver,3)+",amount_atomic="+placeholder(driver,4)+",target_id="+placeholder(driver,5)+",target_name="+placeholder(driver,6)+",quantity="+placeholder(driver,7)+",duration_days="+placeholder(driver,8)+",usage_limit="+placeholder(driver,9)+",per_user_limit="+placeholder(driver,10)+",starts_at="+placeholder(driver,11)+",ends_at="+placeholder(driver,12)+",status="+placeholder(driver,13)+",updated_at="+placeholder(driver,14)+" WHERE id="+placeholder(driver,15)
 res,e:=db.ExecContext(ctx,q,args...);if e!=nil{return v,fmt.Errorf("保存兑换码失败，可能存在重复兑换码")}
 n,e:=res.RowsAffected();if e!=nil{return v,e}
 if n==0{v.UsageCount=0;v.CreatedAt=now;insert:="INSERT INTO redemption_codes(id,name,code,reward_type,amount_atomic,target_id,target_name,quantity,duration_days,usage_limit,per_user_limit,usage_count,starts_at,ends_at,status,created_at,updated_at) VALUES("+strings.TrimRight(strings.Repeat("?,",17),",")+")";for i:=range args{_ = i};insert="INSERT INTO redemption_codes(id,name,code,reward_type,amount_atomic,target_id,target_name,quantity,duration_days,usage_limit,per_user_limit,usage_count,starts_at,ends_at,status,created_at,updated_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+","+placeholder(driver,8)+","+placeholder(driver,9)+","+placeholder(driver,10)+","+placeholder(driver,11)+","+placeholder(driver,12)+","+placeholder(driver,13)+","+placeholder(driver,14)+","+placeholder(driver,15)+","+placeholder(driver,16)+","+placeholder(driver,17)+")";_,e=db.ExecContext(ctx,insert,v.ID,v.Name,v.Code,v.RewardType,v.AmountAtomic,v.TargetID,v.TargetName,v.Quantity,v.DurationDays,v.UsageLimit,v.PerUserLimit,0,v.StartsAt,v.EndsAt,v.Status,v.CreatedAt,v.UpdatedAt);if e!=nil{return v,fmt.Errorf("创建兑换码失败，可能存在重复兑换码")}}
 return v,nil
}
func DeleteRedemptionCode(ctx context.Context,db *sql.DB,driver,id string)error{
 res,err:=db.ExecContext(ctx,"DELETE FROM redemption_codes WHERE id="+placeholder(driver,1),id);if err!=nil{return err};n,err:=res.RowsAffected();if err!=nil{return err};if n==0{return sql.ErrNoRows};return nil
}
