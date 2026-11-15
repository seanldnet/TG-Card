package database
import("context";"database/sql";"time")
func GetSiteSettings(ctx context.Context,db *sql.DB)(string,error){var value string;err:=db.QueryRowContext(ctx,"SELECT settings_json FROM site_settings WHERE id = 1").Scan(&value);if err==sql.ErrNoRows{return "{}",nil};return value,err}
func SaveSiteSettings(ctx context.Context,db *sql.DB,driver,value string)error{now:=time.Now().UTC().Format(time.RFC3339Nano);q:="INSERT INTO site_settings(id,settings_json,updated_at) VALUES ("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+") ON CONFLICT(id) DO UPDATE SET settings_json=excluded.settings_json,updated_at=excluded.updated_at";_,err:=db.ExecContext(ctx,q,1,value,now);return err}
