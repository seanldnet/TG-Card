package telegrambot

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/seanldnet/TG-Card/internal/database"
)

type manualFormField struct {
	Key string
	Label string
	Type string
	Required bool
	MinLength int
	MaxLength int
	Options []string
}

func parseManualFormFields(schemaJSON string) ([]manualFormField,error) {
	var schema map[string]any
	if err:=json.Unmarshal([]byte(schemaJSON),&schema);err!=nil{return nil,fmt.Errorf("表单结构无效")}
	raw,ok:=schema["fields"].([]any);if !ok||len(raw)==0||len(raw)>30{return nil,fmt.Errorf("上游没有提供有效的表单字段")}
	fields:=make([]manualFormField,0,len(raw))
	seen:=map[string]bool{}
	for _,entry:=range raw {
		m,ok:=entry.(map[string]any);if !ok{return nil,fmt.Errorf("表单字段格式无效")}
		key,_:=m["name"].(string);if key==""{key,_=m["key"].(string)}
		key=strings.TrimSpace(key);if key==""||len(key)>80||seen[key]{return nil,fmt.Errorf("表单字段名称无效或重复")};seen[key]=true
		label:=localizedManualLabel(m["label"]);if label==""{label=localizedManualLabel(m["title"])};if label==""{label=key}
		typ,_:=m["type"].(string);if typ==""{typ="text"};typ=strings.ToLower(typ)
		switch typ{case "text","textarea","email","url","number","select":default:return nil,fmt.Errorf("暂不支持上游字段类型：%s",typ)}
		required,_:=m["required"].(bool)
		minLen:=0;if n,ok:=m["min_length"].(float64);ok&&n>=0&&n<=500{minLen=int(n)}
		maxLen:=500;if n,ok:=m["max_length"].(float64);ok&&n>=1&&n<=500{maxLen=int(n)}
		if maxLen<minLen{return nil,fmt.Errorf("表单字段长度规则无效")}
		field:=manualFormField{Key:key,Label:label,Type:typ,Required:required,MinLength:minLen,MaxLength:maxLen}
		if rawOptions,ok:=m["options"].([]any);ok{for _,v:=range rawOptions{if text,ok:=v.(string);ok&&strings.TrimSpace(text)!=""&&len(text)<=100{field.Options=append(field.Options,text)}}}
		if typ=="select"&&len(field.Options)==0{return nil,fmt.Errorf("选择字段缺少有效选项")}
		fields=append(fields,field)
	}
	return fields,nil
}

func localizedManualLabel(value any) string {
	switch v:=value.(type){
	case string:return strings.TrimSpace(v)
	case map[string]any:
		for _,key:=range []string{"zh-CN","zh","zh-TW","en"}{if s,ok:=v[key].(string);ok&&strings.TrimSpace(s)!=""{return strings.TrimSpace(s)}}
		for _,item:=range v{if s,ok:=item.(string);ok&&strings.TrimSpace(s)!=""{return strings.TrimSpace(s)}}
	}
	return ""
}

func (m *Manager) startUpstreamManualForm(ctx context.Context,cfg Config,query CallbackQuery,chatID int64,productID string) {
	product,err:=database.GetVisibleProductByID(ctx,m.db,m.driver,productID)
	if err!=nil||product.SourceType!="upstream"||product.UpstreamFulfillmentType!="manual"||product.UpstreamStockStatus=="out_of_stock" {
		_ = m.answerCallback(ctx,cfg.Token,query.ID,"商品当前不可购买");return
	}
	settings,err:=database.GetUpstreamSettings(ctx,m.db,m.driver)
	if err!=nil||!settings.Enabled||settings.CatalogStale||settings.CallbackURL==""{_ = m.answerCallback(ctx,cfg.Token,query.ID,"上游采购暂不可用");return}
	fields,err:=parseManualFormFields(product.UpstreamManualFormSchemaJSON)
	if err!=nil{_ = m.answerCallback(ctx,cfg.Token,query.ID,"商品表单暂不可用");_ = m.sendMessage(ctx,cfg.Token,chatID,"该商品的上游表单配置暂不受支持，请联系管理员处理。");return}
	m.mu.Lock();encrypt:=m.encrypt;m.mu.Unlock()
	if encrypt==nil{_ = m.answerCallback(ctx,cfg.Token,query.ID,"加密服务暂不可用");return}
	emptyCipher,err:=encrypt("{}");if err!=nil{_ = m.answerCallback(ctx,cfg.Token,query.ID,"无法安全保存表单");return}
	userID:=strconv.FormatInt(query.From.ID,10);chat:=strconv.FormatInt(chatID,10)
	session:=database.UpstreamManualFormSession{BotID:cfg.ID,TelegramUserID:userID,TelegramChatID:chat,ProductID:product.ID,SchemaJSON:product.UpstreamManualFormSchemaJSON,FormDataCiphertext:emptyCipher,FieldIndex:0,ExpiresAt:time.Now().UTC().Add(20*time.Minute).Format(time.RFC3339Nano)}
	if err:=database.SaveUpstreamManualFormSession(ctx,m.db,m.driver,session);err!=nil{_ = m.answerCallback(ctx,cfg.Token,query.ID,"无法创建表单");return}
	_ = m.answerCallback(ctx,cfg.Token,query.ID,"请填写购买资料")
	_ = m.sendMessage(ctx,cfg.Token,chatID,"商品："+product.Name+"\n请按提示填写购买资料（20 分钟内完成）。发送 /cancel 可取消。\n\n1/"+strconv.Itoa(len(fields))+"："+fields[0].Label)
}

func (m *Manager) handleUpstreamManualFormInput(ctx context.Context,cfg Config,msg Message,text string) {
	userID:=strconv.FormatInt(msg.From.ID,10);chat:=strconv.FormatInt(msg.Chat.ID,10)
	session,err:=database.GetUpstreamManualFormSession(ctx,m.db,m.driver,cfg.ID,chat,userID)
	if err!=nil{_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"表单已过期或不存在，请重新打开商品购买。");return}
	fields,err:=parseManualFormFields(session.SchemaJSON);if err!=nil{_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"商品表单配置已失效，请联系管理员。");_ = database.DeleteUpstreamManualFormSession(ctx,m.db,m.driver,cfg.ID,chat,userID);return}
	if session.FieldIndex<0||session.FieldIndex>=len(fields){_ = database.DeleteUpstreamManualFormSession(ctx,m.db,m.driver,cfg.ID,chat,userID);_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"表单状态无效，请重新购买。");return}
	field:=fields[session.FieldIndex];value:=strings.TrimSpace(text)
	if value==""&&field.Required{_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"此项为必填，请填写："+field.Label);return}
	if len([]rune(value))<field.MinLength||len([]rune(value))>field.MaxLength{_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"输入长度不符合要求（"+strconv.Itoa(field.MinLength)+"-"+strconv.Itoa(field.MaxLength)+" 个字符），请重新填写："+field.Label);return}
	if field.Type=="email"&&!strings.Contains(value,"@"){_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"邮箱格式不正确，请重新填写："+field.Label);return}
	if field.Type=="url"&&value!=""&&(!strings.HasPrefix(value,"https://")&&!strings.HasPrefix(value,"http://")){_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"请输入有效的 http/https 地址："+field.Label);return}
	if field.Type=="number"&&value!=""{if _,e:=strconv.ParseFloat(value,64);e!=nil{_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"此项需要填写数字："+field.Label);return}}
	if field.Type=="select"&&value!=""{found:=false;for _,option:=range field.Options{if value==option{found=true;break}};if !found{_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"请从上游提供的选项中选择："+strings.Join(field.Options," / "));return}}
	m.mu.Lock();decrypt,encrypt:=m.decrypt,m.encrypt;m.mu.Unlock()
	if decrypt==nil||encrypt==nil{_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"加密服务暂不可用，请稍后重试。");return}
	plain,err:=decrypt(session.FormDataCiphertext);if err!=nil{_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"表单数据读取失败，请重新开始购买。");_ = database.DeleteUpstreamManualFormSession(ctx,m.db,m.driver,cfg.ID,chat,userID);return}
	data:=map[string]any{};if json.Unmarshal([]byte(plain),&data)!=nil{_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"表单数据无效，请重新开始购买。");_ = database.DeleteUpstreamManualFormSession(ctx,m.db,m.driver,cfg.ID,chat,userID);return}
	data[field.Key]=value
	encoded,err:=json.Marshal(data);if err!=nil{_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"表单保存失败，请重试。");return}
	ciphertext,err:=encrypt(string(encoded));if err!=nil{_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"表单加密失败，请重试。");return}
	session.FormDataCiphertext= ciphertext
	session.FieldIndex++
	if session.FieldIndex<len(fields){
		if err:=database.SaveUpstreamManualFormSession(ctx,m.db,m.driver,session);err!=nil{_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"表单保存失败，请稍后重试。");return}
		next:=fields[session.FieldIndex]
		prompt:="请填写："+next.Label
		if len(next.Options)>0{prompt+="\n可选："+strings.Join(next.Options," / ")}
		_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,strconv.Itoa(session.FieldIndex+1)+"/"+strconv.Itoa(len(fields))+"："+prompt)
		return
	}
	product,err:=database.GetVisibleProductByID(ctx,m.db,m.driver,session.ProductID)
	if err!=nil||product.SourceType!="upstream"||product.UpstreamFulfillmentType!="manual"||product.UpstreamManualFormSchemaJSON!=session.SchemaJSON||product.UpstreamStockStatus=="out_of_stock"{
		_ = database.DeleteUpstreamManualFormSession(ctx,m.db,m.driver,cfg.ID,chat,userID)
		_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"商品或表单信息已更新，请返回商品页重新开始购买。");return
	}
	settings,err:=database.GetUpstreamSettings(ctx,m.db,m.driver)
	if err!=nil||!settings.Enabled||settings.CatalogStale||settings.CallbackURL==""{_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"上游采购暂不可用，请稍后重试。");return}
	order,err:=database.CreateManualUpstreamOrderWithPromotion(ctx,m.db,m.driver,cfg.ID,userID,chat,product.ID,session.PromotionCode,session.FormDataCiphertext,20*time.Minute)
	if err!=nil{_ = m.sendMessage(ctx,cfg.Token,msg.Chat.ID,"订单创建失败："+err.Error()+"\n表单暂时保留，请稍后重试或发送 /cancel。");return}
	_ = database.DeleteUpstreamManualFormSession(ctx,m.db,m.driver,cfg.ID,chat,userID)
	balance,balanceErr:=database.GetTelegramWalletBalance(ctx,m.db,m.driver,userID)
	message:="订单已创建！\n订单号："+order.OrderNo+"\n商品："+order.ProductName+"\n数量：1\n金额："+formatPrice(order.AmountMinor,order.Currency)+"\n请选择付款方式。"
	if balanceErr==nil{message+="\n当前站内余额："+formatUSDTAtomic(balance)+" USDT"}
	buttons:=[][]map[string]string{{{"text":"💳 使用站内余额支付","callback_data":"shop:walletpay:"+order.ID}}}
	if m.paymentEnabled||m.gmpayEnabled{buttons=append(buttons,[]map[string]string{{"text":"🔄 自动选择支付方式","callback_data":"shop:autopay:"+order.ID}})}
	buttons=append(buttons,[]map[string]string{{"text":"💰 充值余额","callback_data":"wallet:recharge:10000000"}})
	_ = m.sendMessageMarkup(ctx,cfg.Token,msg.Chat.ID,message,map[string]any{"inline_keyboard":buttons})
	m.Notify("order_created","新 manual 订单："+order.OrderNo+"\n商品："+order.ProductName+"\n金额："+formatPrice(order.AmountMinor,order.Currency),strconv.FormatInt(msg.Chat.ID,10))
}
