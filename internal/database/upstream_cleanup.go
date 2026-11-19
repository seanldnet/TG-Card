package database

import (
 "context"
 "database/sql"
 "fmt"
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
 if err:=tx.Commit();err!=nil{return counts,err}
 return counts,nil
}
