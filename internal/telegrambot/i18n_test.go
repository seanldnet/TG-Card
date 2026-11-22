package telegrambot

import "testing"

func TestNormalizeLanguage(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"zh", languageChinese},
		{"zh-CN", languageChinese},
		{"zh-hant", languageChinese},
		{"en", languageEnglish},
		{"en-US", languageEnglish},
		{"EN-gb", languageEnglish},
		{"fr", ""},
		{"", ""},
	} {
		if got := normalizeLanguage(tc.input); got != tc.want {
			t.Errorf("normalizeLanguage(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestDetectLanguageFallsBackToChinese(t *testing.T) {
	if got := detectLanguage("fr-FR"); got != languageChinese {
		t.Fatalf("unsupported Telegram language should fall back to Chinese, got %q", got)
	}
	if got := detectLanguage("en-GB"); got != languageEnglish {
		t.Fatalf("English Telegram language should be detected, got %q", got)
	}
}

func TestBotTextFallsBackToChineseForMissingTranslation(t *testing.T) {
	if got := botText(languageEnglish, "home.welcome"); got != "🎉 Welcome to our store!" {
		t.Fatalf("English translation mismatch: %q", got)
	}
	if got := botText(languageEnglish, "unknown.key"); got != "unknown.key" {
		t.Fatalf("unknown translation key should remain visible for diagnosis, got %q", got)
	}
}

func TestMainReplyKeyboardUsesSelectedLanguageAndCanonicalizesLabels(t *testing.T) {
	markup := mainReplyKeyboardForLanguage(false, languageEnglish)
	rows, ok := markup["keyboard"].([][]map[string]string)
	if !ok || len(rows) < 4 {
		t.Fatalf("unexpected reply keyboard shape: %#v", markup)
	}
	if got := rows[0][0]["text"]; got != "🛍️ Store" {
		t.Fatalf("English reply keyboard store label = %q", got)
	}
	if got := rows[3][1]["text"]; got != "🏠 Main menu" {
		t.Fatalf("English reply keyboard home label = %q", got)
	}
	if got := canonicalReplyKeyboardText("📦 My orders"); got != "📦 我的订单" {
		t.Fatalf("English reply keyboard label was not routed to the existing action: %q", got)
	}
	if got := canonicalReplyKeyboardText("🔎 搜索商品"); got != "🔎 搜索商品" {
		t.Fatalf("Chinese reply keyboard label should remain unchanged: %q", got)
	}
}

func TestCommerceScreenTranslations(t *testing.T) {
	for _, tc := range []struct{ language, key, want string }{
		{languageChinese, "product.price", "价格："},
		{languageEnglish, "product.price", "Price: "},
		{languageChinese, "product.cart_add", "🛒 加入购物车"},
		{languageEnglish, "product.cart_add", "🛒 Add to cart"},
		{languageChinese, "order.detail.title", "订单详情"},
		{languageEnglish, "order.detail.title", "Order details"},
		{languageChinese, "payment.title", "继续支付订单"},
		{languageEnglish, "payment.title", "Continue payment"},
	} {
		if got := botText(tc.language, tc.key); got != tc.want {
			t.Errorf("botText(%q, %q) = %q, want %q", tc.language, tc.key, got, tc.want)
		}
	}
	if got := orderStatusText(languageEnglish, "pending_payment"); got != "Awaiting payment" {
		t.Errorf("English order status = %q", got)
	}
}
