package database

import (
 "context"
 "database/sql"
 "fmt"
 "time"
)

type UpstreamCleanupCounts struct {
 Categories int
 Products int
 SKUs int
}

func ClearUpstreamCatalogRows(ctx context.Context,db *sql.DB,driver string)(UpstreamCleanupCounts,error){
 var counts UpstreamCleanupCounts
 tx,err:=db.BeginTx(ctx,nil);if err!=nil{return counts,err};defer tx.Rollback()
 if err:=tx.QueryRowContext(ctx,"SELECT COUNT(1) FROM product_categories WHERE source_type='upstream'").Scan(&counts.Categories);err!=nil{return counts,err}
 if err:=tx.QueryRowContext(ctx,"SELECT COUNT(1) FROM products WHERE source_type='upstream'").Scan(&counts.Products);err!=nil{return counts,err}
 counts.SKUs=counts.Products // each upstream product row represents one imported SKU
 if _,err:=tx.ExecContext(ctx,"DELETE FROM products WHERE source_type='upstream'");err!=nil{return counts,fmt.Errorf("delete upstream product/SKU rows: %w",err)}
 if _,err:=tx.ExecContext(ctx,"DELETE FROM product_categories WHERE source_type='upstream'");err!=nil{return counts,fmt.Errorf("delete upstream categories: %w",err)}
 if _,err:=tx.ExecContext(ctx,"UPDATE upstream_settings SET enabled=0,catalog_stale=1,updated_at="+placeholder(driver,1)+" WHERE id=1",time.Now().UTC().Format(time.RFC3339Nano));err!=nil{return counts,fmt.Errorf("finalize upstream source disable: %w",err)}
 if err:=tx.Commit();err!=nil{return counts,err}
 return counts,nil
}
