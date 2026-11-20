package database

import (
 "context"
 "database/sql"
 "fmt"
 "strconv"
 "strings"
 "time"
)

type FinanceException struct {
 ID string `json:"id"`
 SourceKey string `json:"sourceKey"`
 Category string `json:"category"`
 Severity string `json:"severity"`
 EntityType string `json:"entityType"`
 EntityID string `json:"entityId"`
 Title string `json:"title"`
 Detail string `json:"detail"`
 Status string `json:"status"`
 ResolutionNote string `json:"resolutionNote"`
 ResolvedBy string `json:"resolvedBy"`
 CreatedAt string `json:"createdAt"`
 UpdatedAt string `json:"updatedAt"`
 ResolvedAt string `json:"resolvedAt"`
}

type SystemException = FinanceException

// CreateSystemException stores a single operational exception in the canonical registry.
// Callers must not include credentials, one-time tokens, or full payment secrets in detail.
func CreateSystemException(ctx context.Context, db *sql.DB, driver, sourceKey, category, severity, entityType, entityID, title, detail string) error {
	if strings.TrimSpace(sourceKey) == "" {
		id, err := randomID()
		if err != nil { return err }
		sourceKey = "event:" + id
	}
	if strings.TrimSpace(category) == "" || strings.TrimSpace(title) == "" {
		return fmt.Errorf("exception category and title are required")
	}
	if severity != "info" && severity != "warning" && severity != "error" && severity != "critical" {
		return fmt.Errorf("invalid exception severity")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil { return err }
	defer tx.Rollback()
	if err := upsertFinanceExceptionTx(ctx, tx, driver, sourceKey, category, severity, entityType, entityID, title, detail); err != nil { return err }
	return tx.Commit()
}

// ListSystemExceptions supports the global exception center filters.
func ListSystemExceptions(ctx context.Context, db *sql.DB, driver, status, category, severity, from, to string, limit int) ([]SystemException, error) {
	if limit < 1 || limit > 500 { limit = 100 }
	q := "SELECT id,source_key,category,severity,entity_type,entity_id,title,detail,status,resolution_note,resolved_by,created_at,updated_at,resolved_at FROM system_exceptions WHERE 1=1"
	args := []any{}
	add := func(column, value string) {
		if value == "" { return }
		args = append(args, value)
		q += " AND " + column + "=" + placeholder(driver, len(args))
	}
	add("status", status)
	add("category", category)
	add("severity", severity)
	if from != "" { args=append(args,from); q += " AND created_at >= " + placeholder(driver,len(args)) }
	if to != "" { args=append(args,to); q += " AND created_at <= " + placeholder(driver,len(args)) }
	q += " ORDER BY CASE severity WHEN 'critical' THEN 0 WHEN 'error' THEN 1 WHEN 'warning' THEN 2 ELSE 3 END,created_at DESC LIMIT " + fmt.Sprint(limit)
	rows, err := db.QueryContext(ctx,q,args...)
	if err != nil { return nil,err }
	defer rows.Close()
	out:=make([]SystemException,0)
	for rows.Next() { var x SystemException; if err:=rows.Scan(&x.ID,&x.SourceKey,&x.Category,&x.Severity,&x.EntityType,&x.EntityID,&x.Title,&x.Detail,&x.Status,&x.ResolutionNote,&x.ResolvedBy,&x.CreatedAt,&x.UpdatedAt,&x.ResolvedAt);err!=nil{return nil,err};out=append(out,x) }
	return out,rows.Err()
}



type SupplierFundCheck struct {
 LocalOrderID string `json:"localOrderId"`
 LocalOrderNo string `json:"localOrderNo"`
 ExternalOrderID string `json:"externalOrderId"`
 Currency string `json:"currency"`
 BalanceBefore string `json:"balanceBefore"`
 BalanceAfter string `json:"balanceAfter"`
 ExpectedDebit string `json:"expectedDebit"`
 ObservedDebit string `json:"observedDebit"`
 Difference string `json:"difference"`
 Status string `json:"status"`
 Detail string `json:"detail"`
 CheckedAt string `json:"checkedAt"`
}

func upsertFinanceExceptionTx(ctx context.Context, tx *sql.Tx, driver, sourceKey, category, severity, entityType, entityID, title, detail string) error {
 now:=time.Now().UTC().Format(time.RFC3339Nano)
 id,err:=randomID();if err!=nil{return err}
 q:="INSERT INTO system_exceptions(id,source_key,category,severity,entity_type,entity_id,title,detail,status,created_at,updated_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+","+placeholder(driver,8)+",'open',"+placeholder(driver,9)+","+placeholder(driver,10)+") ON CONFLICT(source_key) DO UPDATE SET category=excluded.category,severity=excluded.severity,title=excluded.title,detail=excluded.detail,status=CASE WHEN system_exceptions.status='resolved' THEN 'open' ELSE system_exceptions.status END,resolution_note=CASE WHEN system_exceptions.status='resolved' THEN '' ELSE system_exceptions.resolution_note END,resolved_by=CASE WHEN system_exceptions.status='resolved' THEN '' ELSE system_exceptions.resolved_by END,updated_at=excluded.updated_at,resolved_at=CASE WHEN system_exceptions.status='resolved' THEN '' ELSE system_exceptions.resolved_at END"
 _,err=tx.ExecContext(ctx,q,id,sourceKey,category,severity,entityType,entityID,title,detail,now,now);return err
}

func CreateFinanceException(ctx context.Context,db *sql.DB,driver,sourceKey,category,severity,entityType,entityID,title,detail string) error {
 tx,err:=db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
 if err:=upsertFinanceExceptionTx(ctx,tx,driver,sourceKey,category,severity,entityType,entityID,title,detail);err!=nil{return err}
 return tx.Commit()
}

func ListFinanceExceptions(ctx context.Context,db *sql.DB,driver,status,category string,limit int)([]FinanceException,error){
 if limit<1||limit>500{limit=100}
 q:="SELECT id,source_key,category,severity,entity_type,entity_id,title,detail,status,resolution_note,resolved_by,created_at,updated_at,resolved_at FROM system_exceptions WHERE 1=1"
 args:=[]any{}
 if status!=""{q+=" AND status="+placeholder(driver,len(args)+1);args=append(args,status)}
 if category!=""{q+=" AND category="+placeholder(driver,len(args)+1);args=append(args,category)}
 q+=" ORDER BY CASE severity WHEN 'critical' THEN 0 WHEN 'error' THEN 1 ELSE 2 END,created_at DESC LIMIT "+fmt.Sprint(limit)
 rows,err:=db.QueryContext(ctx,q,args...);if err!=nil{return nil,err};defer rows.Close()
 out:=make([]FinanceException,0)
 for rows.Next(){var x FinanceException;if err:=rows.Scan(&x.ID,&x.SourceKey,&x.Category,&x.Severity,&x.EntityType,&x.EntityID,&x.Title,&x.Detail,&x.Status,&x.ResolutionNote,&x.ResolvedBy,&x.CreatedAt,&x.UpdatedAt,&x.ResolvedAt);err!=nil{return nil,err};out=append(out,x)}
 return out,rows.Err()
}

func CountOpenFinanceExceptions(ctx context.Context,db *sql.DB)(int64,error){
 var n int64;err:=db.QueryRowContext(ctx,"SELECT COUNT(1) FROM system_exceptions WHERE status IN ('open','processing')").Scan(&n);return n,err
}

func UpdateFinanceException(ctx context.Context,db *sql.DB,driver,id,adminID,status,note string)error{
 if status!="processing"&&status!="resolved"&&status!="ignored"{return fmt.Errorf("invalid finance exception status")}
 if status=="resolved"&&strings.TrimSpace(note)==""{return fmt.Errorf("处理完成时必须填写核实结论")}
 now:=time.Now().UTC().Format(time.RFC3339Nano)
 resolvedAt:="";if status=="resolved"||status=="ignored"{resolvedAt=now}
 res,err:=db.ExecContext(ctx,"UPDATE system_exceptions SET status="+placeholder(driver,1)+",resolution_note="+placeholder(driver,2)+",resolved_by="+placeholder(driver,3)+",updated_at="+placeholder(driver,4)+",resolved_at="+placeholder(driver,5)+" WHERE id="+placeholder(driver,6),status,note,adminID,now,resolvedAt,id)
 if err!=nil{return err};n,err:=res.RowsAffected();if err!=nil{return err};if n!=1{return sql.ErrNoRows};return nil
}

// UpdateSystemExceptionWithAudit commits the exception state and its admin audit row atomically.
func UpdateSystemExceptionWithAudit(ctx context.Context, db *sql.DB, driver, id, adminID, status, note, ip, userAgent string) error {
	if status!="processing" && status!="resolved" && status!="ignored" { return fmt.Errorf("invalid system exception status") }
	if status=="resolved" && strings.TrimSpace(note)=="" { return fmt.Errorf("处理完成时必须填写核实结论") }
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	resolvedAt:="";if status=="resolved"||status=="ignored"{resolvedAt=now}
	res,err:=tx.ExecContext(ctx,"UPDATE system_exceptions SET status="+placeholder(driver,1)+",resolution_note="+placeholder(driver,2)+",resolved_by="+placeholder(driver,3)+",updated_at="+placeholder(driver,4)+",resolved_at="+placeholder(driver,5)+" WHERE id="+placeholder(driver,6),status,note,adminID,now,resolvedAt,id)
	if err!=nil{return err};affected,err:=res.RowsAffected();if err!=nil{return err};if affected!=1{return sql.ErrNoRows}
	auditID,err:=randomID();if err!=nil{return err}
	audit:="INSERT INTO audit_logs(id,admin_id,action,resource,ip_address,user_agent,details,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+","+placeholder(driver,8)+")"
	if _,err:=tx.ExecContext(ctx,audit,auditID,adminID,"system.exception.update","exception:"+id,ip,userAgent,status+": "+note,now);err!=nil{return err}
	return tx.Commit()
}



func parseFinanceMinor(s string)(int64,error){
 s=strings.TrimSpace(s);if s==""{return 0,fmt.Errorf("empty amount")}
 neg:=strings.HasPrefix(s,"-");s=strings.TrimPrefix(strings.TrimPrefix(s,"-"),"+")
 parts:=strings.Split(s,".");if len(parts)>2{return 0,fmt.Errorf("invalid amount")}
 whole,err:=strconv.ParseInt(parts[0],10,64);if err!=nil||whole>922337203685477{return 0,fmt.Errorf("invalid amount")}
 frac:="";if len(parts)==2{frac=parts[1]}
 if len(frac)>2{frac=frac[:2]}
 for len(frac)<2{frac+="0"}
 f:=int64(0);if frac!=""{f,err=strconv.ParseInt(frac,10,64);if err!=nil{return 0,err}}
 v:=whole*100+f;if neg{v=-v};return v,nil
}

func SaveSupplierProcurementBalanceBefore(ctx context.Context,db *sql.DB,driver,orderID,balance,currency string)error{
 balance=strings.TrimSpace(balance);currency=strings.ToUpper(strings.TrimSpace(currency));if balance==""||currency==""{return fmt.Errorf("supplier balance snapshot missing")}
 now:=time.Now().UTC().Format(time.RFC3339Nano)
 tx,err:=db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
 _,err=tx.ExecContext(ctx,"INSERT INTO upstream_procurement_fund_checks(local_order_id,local_order_no,currency,balance_before,status,created_at,updated_at) SELECT local_order_id,local_order_no,"+placeholder(driver,1)+","+placeholder(driver,2)+",'pending',"+placeholder(driver,3)+","+placeholder(driver,4)+" FROM upstream_procurements WHERE local_order_id="+placeholder(driver,5)+" ON CONFLICT(local_order_id) DO NOTHING",currency,balance,now,now,orderID)
 if err!=nil{return err}
 if err:=tx.Commit();err!=nil{return err};return nil
}

func CompleteSupplierProcurementBalanceCheck(ctx context.Context,db *sql.DB,driver,orderID,externalID,expectedDebit,afterBalance,afterCurrency string) (SupplierFundCheck,error){
 var out SupplierFundCheck
 now:=time.Now().UTC().Format(time.RFC3339Nano)
 tx,err:=db.BeginTx(ctx,nil);if err!=nil{return out,err};defer tx.Rollback()
 var before,currency,orderNo string
 q:="SELECT balance_before,currency,local_order_no FROM upstream_procurement_fund_checks WHERE local_order_id="+placeholder(driver,1)
 if err:=tx.QueryRowContext(ctx,q,orderID).Scan(&before,&currency,&orderNo);err!=nil{return out,err}
 out.LocalOrderID=orderID;out.LocalOrderNo=orderNo;out.ExternalOrderID=externalID;out.Currency=currency;out.BalanceBefore=before;out.BalanceAfter=afterBalance;out.ExpectedDebit=expectedDebit;out.CheckedAt=now
 status,detail:="matched",""
 observed,diff:="",""
 if afterBalance==""||afterCurrency==""||!strings.EqualFold(currency,afterCurrency){status="unverifiable";detail="供应商余额后快照缺失或币种与前快照不一致"}
 if status!="unverifiable" {
  b,e1:=parseFinanceMinor(before);a,e2:=parseFinanceMinor(afterBalance);want,e3:=parseFinanceMinor(expectedDebit)
  if e1!=nil||e2!=nil||e3!=nil{status="unverifiable";detail="供应商余额或采购金额格式无法解析"}else{
   observedMinor:=b-a;observed=formatFinanceMinor(observedMinor)
   delta:=observedMinor-want;diff=formatFinanceMinor(delta)
   if absFinance(delta)>1{status="mismatch";detail="供应商余额变动与本地采购金额不一致（容差 0.01）"}
  }
 }
 out.ObservedDebit=observed;out.Difference=diff;out.Status=status;out.Detail=detail
 _,err=tx.ExecContext(ctx,"UPDATE upstream_procurement_fund_checks SET external_order_id="+placeholder(driver,1)+",balance_after="+placeholder(driver,2)+",expected_debit="+placeholder(driver,3)+",observed_debit="+placeholder(driver,4)+",difference="+placeholder(driver,5)+",status="+placeholder(driver,6)+",detail="+placeholder(driver,7)+",checked_at="+placeholder(driver,8)+",updated_at="+placeholder(driver,9)+" WHERE local_order_id="+placeholder(driver,10),externalID,afterBalance,expectedDebit,observed,diff,status,detail,now,now,orderID)
 if err!=nil{return out,err}
 if status=="mismatch"||status=="unverifiable"{if err:=upsertFinanceExceptionTx(ctx,tx,driver,"supplier-funds:"+orderID,"supplier_balance","error","procurement",orderID,"供应商采购资金核对异常",detail+"；订单："+orderNo+"；预期扣款："+expectedDebit+" "+currency+"；观测扣款："+observed+" "+currency+"；差额："+diff);err!=nil{return out,err}}
 if err:=tx.Commit();err!=nil{return out,err};return out,nil
}

func absFinance(v int64)int64{if v<0{return -v};return v}
func formatFinanceMinor(v int64)string{sign:="";if v<0{sign="-";v=absFinance(v)};return fmt.Sprintf("%s%d.%02d",sign,v/100,v%100)}

func MarkStaleSupplierFundChecks(ctx context.Context,db *sql.DB,driver string,olderThan time.Duration)(int,error){
 if olderThan<=0{olderThan=15*time.Minute}
 cutoff:=time.Now().UTC().Add(-olderThan).Format(time.RFC3339Nano)
 tx,err:=db.BeginTx(ctx,nil);if err!=nil{return 0,err};defer tx.Rollback()
 q:="SELECT local_order_id,local_order_no FROM upstream_procurement_fund_checks WHERE status='pending' AND julianday(created_at)<=julianday("+placeholder(driver,1)+")"
 if driver=="postgres"{q="SELECT local_order_id,local_order_no FROM upstream_procurement_fund_checks WHERE status='pending' AND CAST(created_at AS TIMESTAMPTZ)<=CAST("+placeholder(driver,1)+" AS TIMESTAMPTZ)"}
 rows,err:=tx.QueryContext(ctx,q,cutoff);if err!=nil{return 0,err}
 type item struct{id,no string};items:=[]item{}
 for rows.Next(){var x item;if err:=rows.Scan(&x.id,&x.no);err!=nil{rows.Close();return 0,err};items=append(items,x)}
 if err:=rows.Err();err!=nil{rows.Close();return 0,err};rows.Close()
 now:=time.Now().UTC().Format(time.RFC3339Nano)
 for _,x:=range items{
  if _,err:=tx.ExecContext(ctx,"UPDATE upstream_procurement_fund_checks SET status='unverifiable',detail='采购后余额快照未在规定时间内取得，需人工核对',checked_at="+placeholder(driver,1)+",updated_at="+placeholder(driver,2)+" WHERE local_order_id="+placeholder(driver,3)+" AND status='pending'",now,now,x.id);err!=nil{return 0,err}
  if err:=upsertFinanceExceptionTx(ctx,tx,driver,"supplier-funds:"+x.id,"supplier_balance","warning","procurement",x.id,"供应商采购资金核对未完成","订单 "+x.no+" 的采购后余额快照缺失，需人工核对");err!=nil{return 0,err}
 }
 if err:=tx.Commit();err!=nil{return 0,err};return len(items),nil
}

func ListSupplierFundChecks(ctx context.Context,db *sql.DB,limit int)([]SupplierFundCheck,error){
 if limit<1||limit>500{limit=100}
 rows,err:=db.QueryContext(ctx,"SELECT local_order_id,local_order_no,external_order_id,currency,balance_before,balance_after,expected_debit,observed_debit,difference,status,detail,checked_at FROM upstream_procurement_fund_checks ORDER BY updated_at DESC LIMIT "+fmt.Sprint(limit));if err!=nil{return nil,err};defer rows.Close()
 out:=make([]SupplierFundCheck,0);for rows.Next(){var x SupplierFundCheck;if err:=rows.Scan(&x.LocalOrderID,&x.LocalOrderNo,&x.ExternalOrderID,&x.Currency,&x.BalanceBefore,&x.BalanceAfter,&x.ExpectedDebit,&x.ObservedDebit,&x.Difference,&x.Status,&x.Detail,&x.CheckedAt);err!=nil{return nil,err};out=append(out,x)};return out,rows.Err()
}

type UncertainProcurement struct { UpstreamProcurement; OrderStatus string `json:"orderStatus"`; AgeingSeconds int64 `json:"ageingSeconds"`; WaitSeconds int64 `json:"waitSeconds"` }

func ListUncertainProcurements(ctx context.Context,db *sql.DB,driver string,limit int)([]UncertainProcurement,error){
 if limit<1||limit>500{limit=100}
 q:="SELECT p.local_order_id,p.local_order_no,p.external_order_id,p.external_order_no,p.status,p.request_attempts,COALESCE(p.next_poll_at,''),p.deadline_at,p.last_error,p.created_at,p.updated_at,o.status FROM upstream_procurements p JOIN orders o ON o.id=p.local_order_id WHERE p.status='uncertain' ORDER BY p.created_at ASC LIMIT "+fmt.Sprint(limit)
 rows,err:=db.QueryContext(ctx,q);if err!=nil{return nil,err};defer rows.Close()
 out:=make([]UncertainProcurement,0);now:=time.Now().UTC()
 for rows.Next(){var x UncertainProcurement;if err:=rows.Scan(&x.LocalOrderID,&x.LocalOrderNo,&x.ExternalOrderID,&x.ExternalOrderNo,&x.Status,&x.RequestAttempts,&x.NextPollAt,&x.DeadlineAt,&x.LastError,&x.CreatedAt,&x.UpdatedAt,&x.OrderStatus);err!=nil{return nil,err};if created,e:=time.Parse(time.RFC3339Nano,x.CreatedAt);e==nil{x.AgeingSeconds=int64(now.Sub(created).Seconds());if x.AgeingSeconds<0{x.AgeingSeconds=0};x.WaitSeconds=x.AgeingSeconds};out=append(out,x)}
 return out,rows.Err()
}

func ResolveUncertainProcurement(ctx context.Context,db *sql.DB,driver,orderID,adminID,resolution,externalID,externalNo,note string)error{
 resolution=strings.TrimSpace(resolution);note=strings.TrimSpace(note);externalID=strings.TrimSpace(externalID);externalNo=strings.TrimSpace(externalNo)
 if note==""||len(note)>500{return fmt.Errorf("必须填写 1-500 字符的核实说明")}
 if resolution!="accepted"&&resolution!="not_accepted"{return fmt.Errorf("resolution must be accepted or not_accepted")}
 if resolution=="accepted"&&externalID==""{return fmt.Errorf("确认上游已受理时必须填写上游订单 ID")}
 tx,err:=db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
 nowTime:=time.Now().UTC();now:=nowTime.Format(time.RFC3339Nano)
 var timeoutMinutes int
 timeoutMinutes=30;_ = tx.QueryRowContext(ctx,"SELECT procurement_timeout_minutes FROM upstream_settings WHERE id=1").Scan(&timeoutMinutes)
 if timeoutMinutes<1||timeoutMinutes>1440{timeoutMinutes=30}
 deadline:=nowTime.Add(time.Duration(timeoutMinutes)*time.Minute).Format(time.RFC3339Nano)
 var status,orderStatus string
 q:="SELECT p.status,o.status FROM upstream_procurements p JOIN orders o ON o.id=p.local_order_id WHERE p.local_order_id="+placeholder(driver,1);if driver=="postgres"{q+=" FOR UPDATE OF p,o"}
 if err:=tx.QueryRowContext(ctx,q,orderID).Scan(&status,&orderStatus);err!=nil{return err}
 if status!="uncertain"||orderStatus!="procurement_failed"{return fmt.Errorf("订单不处于可人工恢复的 uncertain 状态")}
 if resolution=="accepted"{
  res,err:=tx.ExecContext(ctx,"UPDATE upstream_procurements SET external_order_id="+placeholder(driver,1)+",external_order_no="+placeholder(driver,2)+",status='polling',next_poll_at="+placeholder(driver,3)+",deadline_at="+placeholder(driver,4)+",last_error='',lease_until=NULL,updated_at="+placeholder(driver,5)+" WHERE local_order_id="+placeholder(driver,6)+" AND status='uncertain'",externalID,externalNo,now,deadline,now,orderID);if err!=nil{return err};n,err:=res.RowsAffected();if err!=nil{return err};if n!=1{return fmt.Errorf("采购队列状态已变化")}
  res,err=tx.ExecContext(ctx,"UPDATE orders SET status='procurement_pending',procurement_error='',updated_at="+placeholder(driver,1)+" WHERE id="+placeholder(driver,2)+" AND status='procurement_failed'",now,orderID);if err!=nil{return err};n,err=res.RowsAffected();if err!=nil{return err};if n!=1{return fmt.Errorf("本地订单状态已变化，不能恢复采购查单")}
 }else{
  res,err:=tx.ExecContext(ctx,"UPDATE upstream_procurements SET status='failed',last_error="+placeholder(driver,1)+",next_poll_at=NULL,lease_until=NULL,updated_at="+placeholder(driver,2)+" WHERE local_order_id="+placeholder(driver,3)+" AND status='uncertain'",note,now,orderID);if err!=nil{return err};n,err:=res.RowsAffected();if err!=nil{return err};if n!=1{return fmt.Errorf("采购队列状态已变化")}
 }
 id,e:=randomID();if e!=nil{return e}
 if _,err:=tx.ExecContext(ctx,"INSERT INTO upstream_procurement_events(id,local_order_id,event_type,detail,actor,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+",'owner_resolution',"+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+")",id,orderID,resolution+": "+note,adminID,now);err!=nil{return err}
 if resolution=="not_accepted"{if err:=upsertFinanceExceptionTx(ctx,tx,driver,"procurement-failed:"+orderID,"procurement_failed","error","order",orderID,"上游采购失败",note);err!=nil{return err}}
 if _,err:=tx.ExecContext(ctx,"UPDATE system_exceptions SET status='resolved',resolution_note="+placeholder(driver,1)+",resolved_by="+placeholder(driver,2)+",resolved_at="+placeholder(driver,3)+",updated_at="+placeholder(driver,4)+" WHERE source_key="+placeholder(driver,5),note,adminID,now,now,"procurement-uncertain:"+orderID);err!=nil{return err}
 return tx.Commit()
}

// RunRefundLedgerIntegrityCheck is read-only: it reports broken links and never adjusts wallet balances.
func RunRefundLedgerIntegrityCheck(ctx context.Context,db *sql.DB,driver string)(int,error){
 q:="SELECT r.order_id,r.amount_atomic,o.status,w.direction,w.amount_atomic,w.admin_id,r.admin_id,w.telegram_user_id,o.telegram_user_id,o.amount_minor,r.reason FROM upstream_procurement_refunds r LEFT JOIN orders o ON o.id=r.order_id LEFT JOIN wallet_transactions w ON w.id=r.wallet_transaction_id"
 rows,err:=db.QueryContext(ctx,q);if err!=nil{return 0,err}
 type anomaly struct{id,detail string};bad:=[]anomaly{}
 for rows.Next(){var orderID,orderStatus,direction,ledgerAdmin,refundAdmin,ledgerUser,userID,reason sql.NullString;var amount,ledgerAmount,orderAmount sql.NullInt64
  if err:=rows.Scan(&orderID,&amount,&orderStatus,&direction,&ledgerAmount,&ledgerAdmin,&refundAdmin,&ledgerUser,&userID,&orderAmount,&reason);err!=nil{rows.Close();return 0,err}
  if !orderID.Valid||!orderStatus.Valid||orderStatus.String!="refunded"||!direction.Valid||direction.String!="credit"||!ledgerAmount.Valid||ledgerAmount.Int64!=amount.Int64||!ledgerAdmin.Valid||!refundAdmin.Valid||ledgerAdmin.String!=refundAdmin.String||!ledgerUser.Valid||!userID.Valid||ledgerUser.String!=userID.String||!orderAmount.Valid||amount.Int64!=orderAmount.Int64*10000||!reason.Valid||strings.TrimSpace(reason.String)==""{bad=append(bad,anomaly{orderID.String,"退款账本与订单金额/状态/管理员/买家及钱包 credit 流水关联不一致"})}
 }
 if err:=rows.Err();err!=nil{rows.Close();return 0,err};if err:=rows.Close();err!=nil{return 0,err}
 // A refunded procurement order must have exactly one independent refund row.
 missing,err:=db.QueryContext(ctx,"SELECT o.id FROM orders o JOIN upstream_procurements p ON p.local_order_id=o.id WHERE o.status='refunded' AND p.status='refunded' AND NOT EXISTS (SELECT 1 FROM upstream_procurement_refunds r WHERE r.order_id=o.id)")
 if err!=nil{return 0,err};for missing.Next(){var id string;if err:=missing.Scan(&id);err!=nil{missing.Close();return 0,err};bad=append(bad,anomaly{id,"订单/采购状态为已退款，但缺少独立退款账本"})};if err:=missing.Err();err!=nil{missing.Close();return 0,err};missing.Close()
 // Detect refund credits that are not referenced by the unique procurement refund ledger.
 orphan,err:=db.QueryContext(ctx,"SELECT w.id FROM wallet_transactions w WHERE w.direction='credit' AND w.note LIKE '上游采购失败退款：%' AND NOT EXISTS (SELECT 1 FROM upstream_procurement_refunds r WHERE r.wallet_transaction_id=w.id)")
 if err!=nil{return 0,err};for orphan.Next(){var id string;if err:=orphan.Scan(&id);err!=nil{orphan.Close();return 0,err};bad=append(bad,anomaly{id,"存在上游采购退款 credit，但没有对应的退款账本记录"})};if err:=orphan.Err();err!=nil{orphan.Close();return 0,err};orphan.Close()
 for _,x:=range bad{if err:=CreateFinanceException(ctx,db,driver,"refund-ledger:"+x.id,"refund_ledger","critical","order",x.id,"退款账本完整性异常",x.detail);err!=nil{return len(bad),err}}
 return len(bad),nil
}

func ClaimFinanceExceptionAlert(ctx context.Context,db *sql.DB,driver,alertKey,alertDate string)(bool,error){
 alertKey=strings.TrimSpace(alertKey);alertDate=strings.TrimSpace(alertDate)
 if alertKey==""||alertDate==""{return false,fmt.Errorf("alert key and date are required")}
 now:=time.Now().UTC().Format(time.RFC3339Nano)
 res,err:=db.ExecContext(ctx,"INSERT INTO finance_exception_alerts(alert_key,alert_date,created_at) VALUES("+placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+") ON CONFLICT(alert_key,alert_date) DO NOTHING",alertKey,alertDate,now)
 if err!=nil{return false,err}
 n,err:=res.RowsAffected();if err!=nil{return false,err}
 return n==1,nil
}
