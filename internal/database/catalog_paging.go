package database

import (
 "context"
 "database/sql"
 "fmt"
 "strings"
)

const visibleProductColumns = `p.id,COALESCE(p.category_id,''),COALESCE(c.name,''),p.name,p.description,p.price_minor,p.currency,p.delivery_type,p.status,COALESCE(p.bulk_discount_json,'[]'),COALESCE(p.delivery_config_json,'{}'),
 (SELECT COUNT(1) FROM product_keys k WHERE k.product_id=p.id AND k.status=0),COALESCE(p.source_type,'local'),COALESCE(p.upstream_product_id,''),COALESCE(p.upstream_sku_id,''),COALESCE(p.upstream_sku_code,''),COALESCE(p.upstream_spec_values_json,'{}'),COALESCE(p.upstream_fulfillment_type,''),COALESCE(p.upstream_manual_form_schema_json,'{}'),COALESCE(p.upstream_tags_json,'[]'),COALESCE(p.upstream_slug,''),COALESCE(p.upstream_price_snapshot_minor,0),COALESCE(p.upstream_currency,''),COALESCE(p.upstream_stock_quantity,0),COALESCE(p.upstream_stock_status,''),COALESCE(p.image_paths_json,'[]'),p.created_at,p.updated_at,COALESCE(p.stock_auto_unlisted,0)`

func scanVisibleProducts(rows *sql.Rows) ([]Product,error) {
 items:=make([]Product,0)
 for rows.Next() {
  var p Product; var autoUnlisted int
  if err:=rows.Scan(&p.ID,&p.CategoryID,&p.CategoryName,&p.Name,&p.Description,&p.PriceMinor,&p.Currency,&p.DeliveryType,&p.Status,&p.BulkDiscountJSON,&p.DeliveryConfigJSON,&p.StockCount,&p.SourceType,&p.UpstreamProductID,&p.UpstreamSKUID,&p.UpstreamSKUCode,&p.UpstreamSpecValuesJSON,&p.UpstreamFulfillmentType,&p.UpstreamManualFormSchemaJSON,&p.UpstreamTagsJSON,&p.UpstreamSlug,&p.UpstreamPriceSnapshotMinor,&p.UpstreamCurrency,&p.UpstreamStockQuantity,&p.UpstreamStockStatus,&p.ImagePathsJSON,&p.CreatedAt,&p.UpdatedAt,&autoUnlisted);err!=nil{return nil,err}
  p.StockAutoUnlisted=autoUnlisted==1; items=append(items,p)
 }
 return items,rows.Err()
}

func ListVisibleProductsPage(ctx context.Context, db *sql.DB, driver string, limit, offset int, categoryID, search string) ([]Product,error) {
 if limit<1 {limit=8}; if limit>100 {limit=100}; if offset<0 {offset=0}
 source,err:=currentCatalogSource(ctx,db);if err!=nil{return nil,err}
 search=strings.TrimSpace(search)
 if len(search)>100 {search=search[:100]}
 pattern:="%"+strings.ToLower(search)+"%"
 q:="SELECT "+visibleProductColumns+" FROM products p LEFT JOIN product_categories c ON c.id=p.category_id WHERE COALESCE(p.source_type,'local')="+placeholder(driver,1)+" AND p.status=1 AND ("+placeholder(driver,2)+"='' OR COALESCE(p.category_id,'')="+placeholder(driver,3)+") AND ("+placeholder(driver,4)+"='' OR LOWER(p.name || ' ' || p.description) LIKE "+placeholder(driver,5)+") ORDER BY p.created_at DESC LIMIT "+placeholder(driver,6)+" OFFSET "+placeholder(driver,7)
 rows,err:=db.QueryContext(ctx,q,source,categoryID,categoryID,search,pattern,limit,offset);if err!=nil{return nil,err};defer rows.Close()
 return scanVisibleProducts(rows)
}

func GetVisibleProductByID(ctx context.Context, db *sql.DB, driver, id string) (Product,error) {
 source,err:=currentCatalogSource(ctx,db);if err!=nil{return Product{},err}
 q:="SELECT "+visibleProductColumns+" FROM products p LEFT JOIN product_categories c ON c.id=p.category_id WHERE p.id="+placeholder(driver,1)+" AND COALESCE(p.source_type,'local')="+placeholder(driver,2)+" AND p.status=1"
 rows,err:=db.QueryContext(ctx,q,id,source);if err!=nil{return Product{},err};defer rows.Close()
 items,err:=scanVisibleProducts(rows);if err!=nil{return Product{},err};if len(items)==0{return Product{},sql.ErrNoRows};if len(items)>1{return Product{},fmt.Errorf("duplicate product ID")};return items[0],nil
}

func ListVisibleCategoriesPage(ctx context.Context, db *sql.DB, driver string, limit, offset int) ([]Category,error) {
 if limit<1 {limit=8};if limit>100 {limit=100};if offset<0 {offset=0}
 source,err:=currentCatalogSource(ctx,db);if err!=nil{return nil,err}
 q:="SELECT id,name,sort_order,status,COALESCE(source_type,'local'),COALESCE(upstream_category_id,''),COALESCE(upstream_parent_id,''),COALESCE(upstream_icon,''),created_at,updated_at FROM product_categories WHERE COALESCE(source_type,'local')="+placeholder(driver,1)+" AND status=1 ORDER BY sort_order,name LIMIT "+placeholder(driver,2)+" OFFSET "+placeholder(driver,3)
 rows,err:=db.QueryContext(ctx,q,source,limit,offset);if err!=nil{return nil,err};defer rows.Close()
 items:=make([]Category,0)
 for rows.Next(){var item Category;if err:=rows.Scan(&item.ID,&item.Name,&item.SortOrder,&item.Status,&item.SourceType,&item.UpstreamCategoryID,&item.UpstreamParentID,&item.UpstreamIcon,&item.CreatedAt,&item.UpdatedAt);err!=nil{return nil,err};items=append(items,item)}
 return items,rows.Err()
}
