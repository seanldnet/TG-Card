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
	for _, key := range []string{"cart.empty","cart.checkout","cart.created","cart.pay_wallet","cart.pay_chain","cart.validation"} {
		if botText(languageEnglish,key)==botText(languageChinese,key) { t.Errorf("cart translation %q is not localized",key) }
	}
	for _, key := range []string{"payment.wallet_success","payment.wallet_delivered","payment.gateway_paid","payment.gateway_pending","payment.trc20_assigned","payment.hosted_instructions","payment.refresh_status"} {
		if botText(languageEnglish,key)==botText(languageChinese,key) { t.Errorf("payment translation %q is not localized",key) }
	}
	for _, key := range []string{"delivery.order","delivery.keep_secret","delivery.invite_intro","delivery.invite_warning","delivery.upstream_payload","delivery.card_codes"} {
		if botText(languageEnglish,key)==botText(languageChinese,key) { t.Errorf("delivery translation %q is not localized",key) }
	}

	for _, key := range []string{"support.submit_success","support.status_pending","support.added","support.recreated","support.update_title","support.current_status","support.admin_reply"} {
		if botText(languageEnglish,key)==botText(languageChinese,key) { t.Errorf("support translation %q is not localized",key) }
	}
	if got:=supportStatusText(languageEnglish,"waiting_user");got!="Waiting for your reply"{t.Errorf("English support status = %q",got)}
	if got := orderStatusText(languageEnglish, "pending_payment"); got != "Awaiting payment" {
		t.Errorf("English order status = %q", got)
	}
}

func TestOwnerMenuTranslations(t *testing.T) {
	if got := botText(languageEnglish, "owner.menu"); got != "🛠️ Owner console" { t.Fatalf("English owner menu = %q", got) }
	if got := botText(languageEnglish, "owner.menu.stats"); got != "📊 Business overview" { t.Fatalf("English owner stats label = %q", got) }
	markup := ownerMenuKeyboardForLanguage(languageEnglish)
	rows, ok := markup["inline_keyboard"].([][]map[string]string)
	if !ok || len(rows) != 6 { t.Fatalf("unexpected English owner menu: %#v", markup) }
	if got := rows[0][0]["text"]; got != "📊 Business overview" { t.Fatalf("English owner menu first label = %q", got) }
}
