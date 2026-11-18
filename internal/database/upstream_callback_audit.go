package database

import (
	"context"
	"database/sql"
	"time"
)

func RecordUpstreamCallbackSecurityEvent(ctx context.Context, db *sql.DB, driver, bodyHash, sourceIP, reason string) error {
	id,err:=randomID();if err!=nil{return err}
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	q:="INSERT INTO upstream_callback_security_events(id,body_sha256,source_ip,reason,received_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+")"
	_,err=db.ExecContext(ctx,q,id,bodyHash,sourceIP,reason,now)
	return err
}
