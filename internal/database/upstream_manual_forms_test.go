package database

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/seanldnet/TG-Card/internal/config"
)

func TestUpstreamManualFormSessionPersistsAndExpires(t *testing.T) {
	ctx:=context.Background()
	db,err:=Open(ctx,config.Config{DBDriver:"sqlite",DatabaseDSN:":memory:"})
	if err!=nil{t.Fatal(err)}
	defer db.Close()
	if err:=Migrate(ctx,db,"sqlite");err!=nil{t.Fatal(err)}
	item:=UpstreamManualFormSession{BotID:"bot-1",TelegramUserID:"user-1",TelegramChatID:"chat-1",ProductID:"product-1",SchemaJSON:`{"fields":[{"name":"account"}]}`,FormDataCiphertext:"ciphertext",FieldIndex:0,ExpiresAt:time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)}
	if err:=SaveUpstreamManualFormSession(ctx,db,"sqlite",item);err!=nil{t.Fatal(err)}
	loaded,err:=GetUpstreamManualFormSession(ctx,db,"sqlite","bot-1","chat-1","user-1")
	if err!=nil{t.Fatal(err)}
	if loaded.ProductID!=item.ProductID||loaded.FormDataCiphertext!="ciphertext"||loaded.FieldIndex!=0{t.Fatalf("unexpected persisted session: %+v",loaded)}
	if _,err:=GetUpstreamManualFormSession(ctx,db,"sqlite","bot-1","chat-1","other-user");err!=sql.ErrNoRows{t.Fatalf("session must be isolated by Telegram user, got %v",err)}
	item.FormDataCiphertext="updated-ciphertext";item.FieldIndex=1
	if err:=SaveUpstreamManualFormSession(ctx,db,"sqlite",item);err!=nil{t.Fatal(err)}
	loaded,err=GetUpstreamManualFormSession(ctx,db,"sqlite","bot-1","chat-1","user-1")
	if err!=nil{t.Fatal(err)}
	if loaded.FormDataCiphertext!="updated-ciphertext"||loaded.FieldIndex!=1{t.Fatalf("session upsert failed: %+v",loaded)}
	item.ExpiresAt=time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if err:=SaveUpstreamManualFormSession(ctx,db,"sqlite",item);err!=nil{t.Fatal(err)}
	if _,err:=GetUpstreamManualFormSession(ctx,db,"sqlite","bot-1","chat-1","user-1");err!=sql.ErrNoRows{t.Fatalf("expired session must be rejected, got %v",err)}
}
