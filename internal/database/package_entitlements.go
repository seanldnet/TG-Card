package database

import (
 "context"
 "database/sql"
)

type TelegramPackageEntitlement struct {
 PackageName string
 Quantity int
 GrantedDays int
 ExpiresAt string
 CreatedAt string
}
func ListTelegramPackageEntitlements(ctx context.Context,db *sql.DB,driver,telegramUserID,botID string)([]TelegramPackageEntitlement,error){
 q:="SELECT package_name,quantity,granted_days,expires_at,created_at FROM telegram_package_entitlements WHERE telegram_user_id="+placeholder(driver,1)+" AND bot_id="+placeholder(driver,2)+" AND (expires_at='' OR julianday(expires_at)>julianday('now')) ORDER BY created_at DESC"
 if driver=="postgres"{q="SELECT package_name,quantity,granted_days,expires_at,created_at FROM telegram_package_entitlements WHERE telegram_user_id="+placeholder(driver,1)+" AND bot_id="+placeholder(driver,2)+" AND (expires_at='' OR CAST(expires_at AS TIMESTAMPTZ)>NOW()) ORDER BY created_at DESC"}
 rows,err:=db.QueryContext(ctx,q,telegramUserID,botID);if err!=nil{return nil,err};defer rows.Close()
 out:=make([]TelegramPackageEntitlement,0);for rows.Next(){var v TelegramPackageEntitlement;if err:=rows.Scan(&v.PackageName,&v.Quantity,&v.GrantedDays,&v.ExpiresAt,&v.CreatedAt);err!=nil{return nil,err};out=append(out,v)}
 return out,rows.Err()
}
