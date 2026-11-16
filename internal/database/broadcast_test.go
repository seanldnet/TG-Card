package database

import (
	"context"
	"testing"
	"github.com/seanldnet/TG-Card/internal/config"
)

func TestBroadcastTaskExcludesOptedOutUsersAndTracksDelivery(t *testing.T) {
	ctx:=context.Background()
	cfg:=config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"}
	db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)};defer db.Close()
	if err:=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}
	if err:=UpsertTelegramUser(ctx,db,cfg.DBDriver,"1001","one","One","","broadcast-bot");err!=nil{t.Fatal(err)}
	if err:=UpsertTelegramUser(ctx,db,cfg.DBDriver,"1002","two","Two","","broadcast-bot");err!=nil{t.Fatal(err)}
	if err:=SetBroadcastOptOut(ctx,db,cfg.DBDriver,"broadcast-bot","1002",true);err!=nil{t.Fatal(err)}
	task,err:=CreateBroadcastTask(ctx,db,cfg.DBDriver,"broadcast-bot","Test","Hello","","admin")
	if err!=nil{t.Fatal(err)}
	if task.TotalCount!=1{t.Fatalf("expected one eligible recipient, got %d",task.TotalCount)}
	ids,err:=ClaimDueBroadcastTasks(ctx,db,cfg.DBDriver,5);if err!=nil{t.Fatal(err)}
	if len(ids)!=1||ids[0]!=task.ID{t.Fatalf("unexpected claimed tasks: %#v",ids)}
	recipients,err:=ListPendingBroadcastRecipients(ctx,db,cfg.DBDriver,task.ID);if err!=nil{t.Fatal(err)}
	if len(recipients)!=1||recipients[0].TelegramUserID!="1001"{t.Fatalf("unexpected recipients: %#v",recipients)}
	if err:=UpdateBroadcastRecipient(ctx,db,cfg.DBDriver,recipients[0].ID,"sent","",1);err!=nil{t.Fatal(err)}
	sent,failed,err:=CountBroadcastRecipients(ctx,db,cfg.DBDriver,task.ID);if err!=nil{t.Fatal(err)}
	if sent!=1||failed!=0{t.Fatalf("unexpected counts: sent=%d failed=%d",sent,failed)}
	if err:=FinishBroadcastTask(ctx,db,cfg.DBDriver,task.ID,"completed",sent,failed);err!=nil{t.Fatal(err)}
	items,err:=ListBroadcastTasks(ctx,db,cfg.DBDriver);if err!=nil{t.Fatal(err)}
	if len(items)!=1||items[0].Status!="completed"{t.Fatalf("unexpected task state: %#v",items)}
}
