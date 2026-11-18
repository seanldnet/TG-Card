package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func boolInt(value bool) int { if value { return 1 }; return 0 }

func ensureLocalCategory(ctx context.Context, db *sql.DB, driver, categoryID string) error {
	if categoryID == "" { return nil }
	var source string
	err := db.QueryRowContext(ctx, "SELECT COALESCE(source_type,'local') FROM product_categories WHERE id="+placeholder(driver,1), categoryID).Scan(&source)
	if err != nil { return err }
	if source != "local" { return fmt.Errorf("local products cannot reference upstream categories") }
	return nil
}

type UpstreamCategoryRecord struct {
	UpstreamID string
	ParentID string
	Name string
	Icon string
	PreserveIcon bool
	SortOrder int
}

type UpstreamProductRecord struct {
	ProductID string
	SKUID string
	Slug string
	SKUCode string
	SpecValuesJSON string
	FulfillmentType string
	ManualFormSchemaJSON string
	TagsJSON string
	CategoryID string
	Name string
	Description string
	SKUCode string
	SpecValuesJSON string
	FulfillmentType string
	ManualFormSchemaJSON string
	TagsJSON string
	PriceMinor int64
	PriceSnapshotMinor int64
	Currency string
	StockQuantity int64
	StockStatus string
	ImagePathsJSON string
	Active bool
}

func SyncUpstreamCatalog(ctx context.Context, db *sql.DB, driver, batchID string, categories []UpstreamCategoryRecord, products []UpstreamProductRecord) error {
	if batchID == "" { return fmt.Errorf("upstream sync batch ID is required") }
	tx, err := db.BeginTx(ctx, nil)
	if err != nil { return err }
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	categoryIDs := make(map[string]string, len(categories))

	for _, item := range categories {
		if item.UpstreamID == "" || item.Name == "" { return fmt.Errorf("upstream category ID and name are required") }
		var localID string
		err := tx.QueryRowContext(ctx, "SELECT id FROM product_categories WHERE source_type='upstream' AND upstream_category_id="+placeholder(driver,1), item.UpstreamID).Scan(&localID)
		if err == sql.ErrNoRows {
			localID, err = randomID()
			if err != nil { return err }
			query := "INSERT INTO product_categories(id,name,sort_order,status,source_type,upstream_category_id,upstream_parent_id,upstream_icon,upstream_sync_batch_id,created_at,updated_at) VALUES("+
				placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+",1,'upstream',"+placeholder(driver,4)+","+placeholder(driver,5)+","+placeholder(driver,6)+","+placeholder(driver,7)+","+placeholder(driver,8)+","+placeholder(driver,9)+")"
			if _, err = tx.ExecContext(ctx, query, localID, item.Name, item.SortOrder, item.UpstreamID, item.ParentID, item.Icon, batchID, now, now); err != nil { return fmt.Errorf("insert upstream category %s: %w", item.UpstreamID, err) }
		} else if err != nil { return err } else {
			query := "UPDATE product_categories SET name="+placeholder(driver,1)+",sort_order="+placeholder(driver,2)+",status=1,upstream_parent_id="+placeholder(driver,3)+",upstream_icon=CASE WHEN "+placeholder(driver,5)+"=1 THEN upstream_icon ELSE "+placeholder(driver,4)+" END,upstream_sync_batch_id="+placeholder(driver,6)+",updated_at="+placeholder(driver,7)+" WHERE id="+placeholder(driver,8)+" AND source_type='upstream'"
			if _, err := tx.ExecContext(ctx, query, item.Name, item.SortOrder, item.ParentID, item.Icon, boolInt(item.PreserveIcon), batchID, now, localID); err != nil { return fmt.Errorf("update upstream category %s: %w", item.UpstreamID, err) }
		}
		categoryIDs[item.UpstreamID] = localID
	}

	for _, item := range products {
		if item.ProductID == "" || item.SKUID == "" || item.Name == "" { return fmt.Errorf("upstream product and SKU IDs and product name are required") }
		if item.Currency == "" { return fmt.Errorf("upstream currency is required for product %s/%s", item.ProductID, item.SKUID) }
		if item.PriceMinor < 0 || item.PriceSnapshotMinor < 0 { return fmt.Errorf("invalid upstream price for product %s/%s", item.ProductID, item.SKUID) }
		var category any
		if id := categoryIDs[item.CategoryID]; id != "" { category = id }
		var localID, previousImages string
		err := tx.QueryRowContext(ctx, "SELECT id,COALESCE(image_paths_json,'[]') FROM products WHERE source_type='upstream' AND upstream_product_id="+placeholder(driver,1)+" AND upstream_sku_id="+placeholder(driver,2), item.ProductID, item.SKUID).Scan(&localID, &previousImages)
		if err == sql.ErrNoRows {
			localID, err = randomID()
			if err != nil { return err }
			images := item.ImagePathsJSON
			if images == "" { images = "[]" }
			query := "INSERT INTO products(id,category_id,name,description,price_minor,currency,delivery_type,status,bulk_discount_json,delivery_config_json,source_type,upstream_product_id,upstream_sku_id,upstream_sku_code,upstream_spec_values_json,upstream_fulfillment_type,upstream_manual_form_schema_json,upstream_tags_json,upstream_slug,upstream_price_snapshot_minor,upstream_currency,upstream_stock_quantity,upstream_stock_status,image_paths_json,upstream_sync_batch_id,created_at,updated_at) VALUES("+
				placeholder(driver,1)+","+placeholder(driver,2)+","+placeholder(driver,3)+","+placeholder(driver,4)+","+placeholder(driver,5)+",'USDT','upstream',"+placeholder(driver,6)+",'[]','{}','upstream',"+placeholder(driver,7)+","+placeholder(driver,8)+","+placeholder(driver,9)+","+placeholder(driver,10)+","+placeholder(driver,11)+","+placeholder(driver,12)+","+placeholder(driver,13)+","+placeholder(driver,14)+","+placeholder(driver,15)+","+placeholder(driver,16)+","+placeholder(driver,17)+","+placeholder(driver,18)+","+placeholder(driver,19)+","+placeholder(driver,20)+","+placeholder(driver,21)+","+placeholder(driver,22)+")"
			status := 0
			if item.Active { status = 1 }
			if _, err = tx.ExecContext(ctx, query, localID, category, item.Name, item.Description, item.PriceMinor, status, item.ProductID, item.SKUID, item.SKUCode, item.SpecValuesJSON, item.FulfillmentType, item.ManualFormSchemaJSON, item.TagsJSON, item.Slug, item.PriceSnapshotMinor, item.Currency, item.StockQuantity, item.StockStatus, images, batchID, now, now); err != nil { return fmt.Errorf("insert upstream product %s/%s: %w", item.ProductID, item.SKUID, err) }
		} else if err != nil { return err } else {
			images := item.ImagePathsJSON
			if images == "" { images = previousImages }
			status := 0
			if item.Active { status = 1 }
			query := "UPDATE products SET category_id="+placeholder(driver,1)+",name="+placeholder(driver,2)+",description="+placeholder(driver,3)+",price_minor="+placeholder(driver,4)+",status="+placeholder(driver,5)+",upstream_sku_code="+placeholder(driver,6)+",upstream_spec_values_json="+placeholder(driver,7)+",upstream_fulfillment_type="+placeholder(driver,8)+",upstream_manual_form_schema_json="+placeholder(driver,9)+",upstream_tags_json="+placeholder(driver,10)+",upstream_slug="+placeholder(driver,11)+",upstream_price_snapshot_minor="+placeholder(driver,12)+",upstream_currency="+placeholder(driver,13)+",upstream_stock_quantity="+placeholder(driver,14)+",upstream_stock_status="+placeholder(driver,15)+",image_paths_json="+placeholder(driver,16)+",upstream_sync_batch_id="+placeholder(driver,17)+",updated_at="+placeholder(driver,18)+" WHERE id="+placeholder(driver,19)+" AND source_type='upstream'"
			if _, err := tx.ExecContext(ctx, query, category, item.Name, item.Description, item.PriceMinor, status, item.SKUCode, item.SpecValuesJSON, item.FulfillmentType, item.ManualFormSchemaJSON, item.TagsJSON, item.Slug, item.PriceSnapshotMinor, item.Currency, item.StockQuantity, item.StockStatus, images, batchID, now, localID); err != nil { return fmt.Errorf("update upstream product %s/%s: %w", item.ProductID, item.SKUID, err) }
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE products SET status=0,updated_at="+placeholder(driver,1)+" WHERE source_type='upstream' AND (upstream_sync_batch_id IS NULL OR upstream_sync_batch_id<>"+placeholder(driver,2)+")", now, batchID); err != nil { return fmt.Errorf("hide upstream products missing from successful sync: %w", err) }
	if _, err := tx.ExecContext(ctx, "UPDATE product_categories SET status=0,updated_at="+placeholder(driver,1)+" WHERE source_type='upstream' AND (upstream_sync_batch_id IS NULL OR upstream_sync_batch_id<>"+placeholder(driver,2)+")", now, batchID); err != nil { return fmt.Errorf("hide upstream categories missing from successful sync: %w", err) }
	return tx.Commit()
}

func currentCatalogSource(ctx context.Context, db *sql.DB) (string, error) {
	settings, err := GetUpstreamSettings(ctx, db, "")
	if err == sql.ErrNoRows { return "local", nil }
	if err != nil { return "", err }
	if settings.Enabled { return "upstream", nil }
	return "local", nil
}

func ListVisibleProducts(ctx context.Context, db *sql.DB) ([]Product, error) {
	items, err := ListProducts(ctx, db)
	if err != nil { return nil, err }
	source, err := currentCatalogSource(ctx, db)
	if err != nil { return nil, err }
	visible := make([]Product, 0, len(items))
	for _, item := range items { if item.SourceType == source && item.Status == 1 { visible = append(visible, item) } }
	return visible, nil
}

func ListVisibleCategories(ctx context.Context, db *sql.DB) ([]Category, error) {
	items, err := ListCategories(ctx, db)
	if err != nil { return nil, err }
	source, err := currentCatalogSource(ctx, db)
	if err != nil { return nil, err }
	visible := make([]Category, 0, len(items))
	for _, item := range items { if item.SourceType == source && item.Status == 1 { visible = append(visible, item) } }
	return visible, nil
}
