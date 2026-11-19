package database

import (
 "context"
 "database/sql"
 "fmt"
 "time"
)

type UpstreamSyncJob struct {
 ID string
 Status string
 TotalItems int
 ProcessedItems int
 CurrentPage int
 BatchSize int
 DelayMS int
 ErrorCount int
 ConsecutiveFailures int
 CancelRequested bool
 LastError string
 TriggerType string
 CreatedAt string
 UpdatedAt string
 FinishedAt string
}

func CreateUpstreamSyncJob(ctx context.Context, db *sql.DB, driver string, job UpstreamSyncJob) error {
 if job.ID == "" { return fmt.Errorf("upstream sync job ID is required") }
 now := time.Now().UTC().Format(time.RFC3339Nano)
 if job.Status == "" { job.Status = "queued" }
 if job.BatchSize < 1 || job.BatchSize > 100 { return fmt.Errorf("upstream sync batch size must be 1–100") }
 if job.DelayMS < 0 || job.DelayMS > 30000 { return fmt.Errorf("upstream sync delay must be 0–30000 ms") }
 if job.TriggerType == "" { job.TriggerType = "manual" }
 q := "INSERT INTO upstream_sync_jobs(id,status,total_items,processed_items,current_page,batch_size,delay_ms,error_count,consecutive_failures,cancel_requested,last_error,trigger_type,created_at,updated_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+","+placeholder(driver,8)+","+placeholder(driver,9)+","+placeholder(driver,10)+","+placeholder(driver,11)+","+placeholder(driver,12)+","+placeholder(driver,13)+","+placeholder(driver,14)+")"
 _, err := db.ExecContext(ctx,q,job.ID,job.Status,job.TotalItems,job.ProcessedItems,job.CurrentPage,job.BatchSize,job.DelayMS,job.ErrorCount,job.ConsecutiveFailures,boolInt(job.CancelRequested),job.LastError,job.TriggerType,now,now)
 return err
}

func GetUpstreamSyncJob(ctx context.Context, db *sql.DB, driver, id string) (UpstreamSyncJob, error) {
 var job UpstreamSyncJob
 var cancel int
 var finished sql.NullString
 q := "SELECT id,status,total_items,processed_items,current_page,batch_size,delay_ms,error_count,consecutive_failures,cancel_requested,last_error,trigger_type,created_at,updated_at,finished_at FROM upstream_sync_jobs WHERE id="+placeholder(driver,1)
 err := db.QueryRowContext(ctx,q,id).Scan(&job.ID,&job.Status,&job.TotalItems,&job.ProcessedItems,&job.CurrentPage,&job.BatchSize,&job.DelayMS,&job.ErrorCount,&job.ConsecutiveFailures,&cancel,&job.LastError,&job.TriggerType,&job.CreatedAt,&job.UpdatedAt,&finished)
 job.CancelRequested = cancel != 0
 if finished.Valid { job.FinishedAt = finished.String }
 return job, err
}

func UpdateUpstreamSyncJobProgress(ctx context.Context, db *sql.DB, driver, id string, total, processed, page, errors, consecutiveFailures int, lastError string) error {
 if len(lastError) > 800 { lastError = lastError[:800] }
 now := time.Now().UTC().Format(time.RFC3339Nano)
 q := "UPDATE upstream_sync_jobs SET total_items="+placeholder(driver,1)+",processed_items="+placeholder(driver,2)+",current_page="+placeholder(driver,3)+",error_count="+placeholder(driver,4)+",consecutive_failures="+placeholder(driver,5)+",last_error="+placeholder(driver,6)+",updated_at="+placeholder(driver,7)+" WHERE id="+placeholder(driver,8)
 res, err := db.ExecContext(ctx,q,total,processed,page,errors,consecutiveFailures,lastError,now,id)
 if err != nil { return err }
 n, err := res.RowsAffected()
 if err != nil { return err }
 if n != 1 { return fmt.Errorf("upstream sync job not found") }
 return nil
}

func SetUpstreamSyncJobStatus(ctx context.Context, db *sql.DB, driver, id, status string) error {
 if status != "queued" && status != "running" && status != "success" && status != "failed" && status != "cancelled" && status != "paused" && status != "cleaning" { return fmt.Errorf("invalid upstream sync job status") }
 now := time.Now().UTC().Format(time.RFC3339Nano)
 finished := any(nil)
 if status == "success" || status == "failed" || status == "cancelled" || status == "paused" { finished = now }
 q := "UPDATE upstream_sync_jobs SET status="+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+",finished_at="+placeholder(driver,3)+" WHERE id="+placeholder(driver,4)
 _, err := db.ExecContext(ctx,q,status,now,finished,id)
 return err
}

func RequestUpstreamSyncJobCancel(ctx context.Context, db *sql.DB, driver, id string) error {
 q := "UPDATE upstream_sync_jobs SET cancel_requested=1,updated_at="+placeholder(driver,1)+" WHERE id="+placeholder(driver,2)+" AND status IN ('queued','running')"
 res, err := db.ExecContext(ctx,q,time.Now().UTC().Format(time.RFC3339Nano),id)
 if err != nil { return err }
 n, err := res.RowsAffected()
 if err != nil { return err }
 if n != 1 { return fmt.Errorf("同步任务不存在或已结束") }
 return nil
}


func RecoverInterruptedUpstreamSyncJobs(ctx context.Context,db *sql.DB,driver string) error {
 now:=time.Now().UTC().Format(time.RFC3339Nano)
 q:="UPDATE upstream_sync_jobs SET status='failed',last_error='服务进程重启，任务已中断；请重新执行',updated_at="+placeholder(driver,1)+",finished_at="+placeholder(driver,2)+" WHERE status IN ('queued','running','cleaning')"
 _,err:=db.ExecContext(ctx,q,now,now)
 return err
}


func GetActiveUpstreamSyncJob(ctx context.Context,db *sql.DB,driver string)(UpstreamSyncJob,error){
 var id string
 q:="SELECT id FROM upstream_sync_jobs WHERE status IN ('queued','running','cleaning') ORDER BY created_at DESC LIMIT 1"
 if err:=db.QueryRowContext(ctx,q).Scan(&id);err!=nil{return UpstreamSyncJob{},err}
 return GetUpstreamSyncJob(ctx,db,driver,id)
}
