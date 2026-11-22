package llm

import (
	"strings"
	"testing"

	"github.com/seanldnet/TG-Card/internal/database"
)

func TestDecideSupportHandlingEscalatesSensitiveRequests(t *testing.T) {
	cases := []string{
		"我要退款", "请转人工客服", "这笔付款有争议", "我的账号被盗了",
		"我要投诉", "I want a refund", "I need a human agent", "chargeback",
	}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			got := DecideSupportHandling(input)
			if got.AutoReply || got.Reason != "sensitive_or_human_request" {
				t.Fatalf("expected escalation for %q, got %+v", input, got)
			}
		})
	}
}

func TestDecideSupportHandlingAllowsRoutineQuestions(t *testing.T) {
	got := DecideSupportHandling("订单里的卡密在哪里查看？")
	if !got.AutoReply || got.Reason != "routine_support" {
		t.Fatalf("expected routine support, got %+v", got)
	}
	if got := DecideSupportHandling("   "); got.AutoReply || got.Reason != "empty_message" {
		t.Fatalf("expected empty message to fail closed, got %+v", got)
	}
}

func TestBuildSupportPromptBoundsAndLabelsContext(t *testing.T) {
	ticket := database.SupportTicket{TicketNo: "T-1", OrderNo: "O-1", Category: "other", Description: "请帮我查看订单"}
	messages := []database.SupportTicketMessage{
		{SenderType: "user", Message: "之前的问题"},
		{SenderType: "admin", Message: "请提供订单号"},
		{SenderType: "system", SenderID: "internal", Message: "内部系统记录不应进入对话"},
		{SenderType: "system", SenderID: "ai-assistant", Message: "AI 售后预处理：你可以在订单详情中查看卡密"},
		{SenderType: "user", Message: strings.Repeat("长", 2500)},
	}
	got := BuildSupportPrompt(ticket, messages, "当前补充问题")
	if len(got) < 3 || got[0].Role != "system" {
		t.Fatalf("unexpected prompt structure: %+v", got)
	}
	foundPreviousAI := false
	for _, item := range got {
		if strings.Contains(item.Content, "内部系统记录") {
			t.Fatal("internal system timeline events must not be passed to the model")
		}
		if len([]rune(item.Content)) > 2100 {
			t.Fatalf("context item exceeded bound: %d runes", len([]rune(item.Content)))
		}
		if item.Role == "assistant" && item.Content == "你可以在订单详情中查看卡密" {
			foundPreviousAI = true
		}
	}
	if !foundPreviousAI {
		t.Fatal("previous AI reply should be included as an assistant turn")
	}
	if got[len(got)-1].Role != "user" || got[len(got)-1].Content != "当前补充问题" {
		t.Fatalf("current message missing: %+v", got[len(got)-1])
	}
	if !strings.Contains(got[0].Content, "订单号：O-1") {
		t.Fatal("order context missing from system message")
	}
}

func TestBuildSupportPromptIncludesVerifiedFactsAndOperatorPrompt(t *testing.T) {
 ticket := database.SupportTicket{TicketNo:"T-2",OrderNo:"O-2",Category:"not_received",Description:"商品没有到账"}
 order := &SupportOrderContext{OrderNo:"O-2",ProductName:"Premium Card",Status:"delivered",Quantity:2,Currency:"CNY",AmountMinor:5000,PaidAt:"2026-11-22T10:00:00Z",DeliveredAt:"2026-11-22T10:01:00Z"}
 got := BuildSupportPromptWithContext(ticket,nil,"卡密在哪里？","请用简短步骤说明查看路径。",order)
 if len(got)!=2 { t.Fatalf("expected system and current user messages, got %d",len(got)) }
 system:=got[0].Content
 for _, want:=range []string{"Premium Card","delivered","购买数量：2","请用简短步骤说明查看路径","不得覆盖上述安全限制"} {
  if !strings.Contains(system,want) { t.Fatalf("system context missing %q",want) }
 }
 for _, forbidden:=range []string{"payment_reference","delivery_payload","API Key"} {
  if strings.Contains(system,forbidden) { t.Fatalf("sensitive field leaked into prompt: %q",forbidden) }
 }
}
