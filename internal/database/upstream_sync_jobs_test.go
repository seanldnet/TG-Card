package database

import (
 "context"
 "testing"

 "github.com/seanldnet/TG-Card/internal/config"
)

func TestUpstreamSyncJobHasSingleActiveSlotAndCanResume(t *testing.T) {
 ctx:=context.Background();cfg:=config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"}
 db,err:=Open(ctx,cfg);if err!=nil{t.Fatal(err)};defer db.Close()
 if err:=Migrate(ctx,db,cfg.DBDriver);err!=nil{t.Fatal(err)}
 first:=UpstreamSyncJob{ID:"job-one",Status:"running",BatchSize:25,DelayMS:1000,TriggerType:"manual"}
 if err:=CreateUpstreamSyncJob(ctx,db,cfg.DBDriver,first);err!=nil{t.Fatal(err)}
 second:=UpstreamSyncJob{ID:"job-two",Status:"queued",BatchSize:25,DelayMS:1000,TriggerType:"scheduled"}
 if err:=CreateUpstreamSyncJob(ctx,db,cfg.DBDriver,second);err==nil{t.Fatal("database must reject a second active upstream job")}
 active,err:=GetActiveUpstreamSyncJob(ctx,db,cfg.DBDriver);if err!=nil{t.Fatal(err)}
 if active.ID!="job-one"||active.Status!="running"{t.Fatalf("unexpected active job: %+v",active)}
 if err:=UpdateUpstreamSyncJobProgress(ctx,db,cfg.DBDriver,active.ID,500,125,5,1,0,"page 4 retry recovered");err!=nil{t.Fatal(err)}
 active,err=GetActiveUpstreamSyncJob(ctx,db,cfg.DBDriver);if err!=nil{t.Fatal(err)}
 if active.TotalItems!=500||active.ProcessedItems!=125||active.CurrentPage!=5||active.ErrorCount!=1{t.Fatalf("progress did not persist: %+v",active)}
 if err:=RequestUpstreamSyncJobCancel(ctx,db,cfg.DBDriver,active.ID);err!=nil{t.Fatal(err)}
 active,err=GetActiveUpstreamSyncJob(ctx,db,cfg.DBDriver);if err!=nil||!active.CancelRequested{t.Fatalf("cancel request must persist across app instances: %+v %v",active,err)}
 if err:=SetUpstreamSyncJobStatus(ctx,db,cfg.DBDriver,active.ID,"cancelled");err!=nil{t.Fatal(err)}
 if _,err:=GetActiveUpstreamSyncJob(ctx,db,cfg.DBDriver);err==nil{t.Fatal("completed job must not be returned as active")}
}
