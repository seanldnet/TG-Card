package telegrambot

import "strings"

const (
	languageChinese = "zh"
	languageEnglish = "en"
)

func normalizeLanguage(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch {
	case value == "zh" || strings.HasPrefix(value, "zh-"):
		return languageChinese
	case value == "en" || strings.HasPrefix(value, "en-"):
		return languageEnglish
	default:
		return ""
	}
}

func detectLanguage(languageCode string) string {
	if language := normalizeLanguage(languageCode); language != "" {
		return language
	}
	return languageChinese
}

var botMessages = map[string]map[string]string{
	"language.title": {
		languageChinese: "🌐 请选择机器人语言",
		languageEnglish: "🌐 Choose your language",
	},
	"language.saved": {
		languageChinese: "语言已切换为简体中文。",
		languageEnglish: "Language changed to English.",
	},
	"language.button.zh": {
		languageChinese: "🇨🇳 简体中文",
		languageEnglish: "🇨🇳 简体中文",
	},
	"language.button.en": {
		languageChinese: "🇬🇧 English",
		languageEnglish: "🇬🇧 English",
	},
	"language.current": {
		languageChinese: "当前语言：简体中文",
		languageEnglish: "Current language: English",
	},
	"home.welcome": {
		languageChinese: "🎉 欢迎来到我们的商城！",
		languageEnglish: "🎉 Welcome to our store!",
	},
	"home.user": {
		languageChinese: "用户",
		languageEnglish: "Customer",
	},
	"home.balance": {
		languageChinese: "💰 当前余额：",
		languageEnglish: "💰 Balance: ",
	},
	"home.telegram_id": {
		languageChinese: "👤 Telegram ID：",
		languageEnglish: "👤 Telegram ID: ",
	},
	"home.purchase_notice": {
		languageChinese: "📌 购买须知：\n• 请确认商品信息及库存后再下单\n• 虚拟商品发货后请妥善保管卡密",
		languageEnglish: "📌 Before purchasing:\n• Check product details and stock before ordering\n• Keep delivered digital codes secure",
	},
	"home.persistent_menu": {
		languageChinese: "👇 也可以使用输入框下方的常驻菜单快速操作。",
		languageEnglish: "👇 You can also use the menu below the message box.",
	},
	"menu.shop": {
		languageChinese: "🛍️ 商品中心",
		languageEnglish: "🛍️ Store",
	},
	"menu.search": {
		languageChinese: "🔎 搜索商品",
		languageEnglish: "🔎 Search products",
	},
	"menu.cart": {
		languageChinese: "🛒 购物车",
		languageEnglish: "🛒 Cart",
	},
	"menu.orders": {
		languageChinese: "📦 我的订单",
		languageEnglish: "📦 My orders",
	},
	"menu.balance": {
		languageChinese: "💳 我的余额",
		languageEnglish: "💳 My balance",
	},
	"menu.recharge": {
		languageChinese: "💰 余额充值",
		languageEnglish: "💰 Top up",
	},
	"menu.support": {
		languageChinese: "🧾 申请售后",
		languageEnglish: "🧾 Support",
	},
	"menu.help": {
		languageChinese: "❓ 使用帮助",
		languageEnglish: "❓ Help",
	},
	"menu.home": {
		languageChinese: "🏠 主菜单",
		languageEnglish: "🏠 Main menu",
	},
	"shop.title": {
		languageChinese: "🛍️ 商品中心\n请选择商品分类：",
		languageEnglish: "🛍️ Store\nChoose a category:",
	},
	"shop.empty": {
		languageChinese: "目前还没有上架商品。",
		languageEnglish: "There are no products available yet.",
	},
	"shop.load_error": {
		languageChinese: "商品分类暂时无法加载。",
		languageEnglish: "Product categories are temporarily unavailable.",
	},
	"orders.empty": {
		languageChinese: "你还没有订单。",
		languageEnglish: "You don't have any orders yet.",
	},
	"orders.title": {
		languageChinese: "📦 我的订单（最近 20 笔）",
		languageEnglish: "📦 My orders (latest 20)",
	},
	"orders.number": {
		languageChinese: "订单号：",
		languageEnglish: "Order: ",
	},
	"orders.product": {
		languageChinese: "商品：",
		languageEnglish: "Product: ",
	},
	"orders.quantity": {
		languageChinese: "数量：",
		languageEnglish: "Quantity: ",
	},
	"orders.amount": {
		languageChinese: "金额：",
		languageEnglish: "Amount: ",
	},
	"orders.status": {
		languageChinese: "状态：",
		languageEnglish: "Status: ",
	},
	"orders.detail": {
		languageChinese: "查看详情",
		languageEnglish: "Details",
	},
	"orders.back": {
		languageChinese: "⬅️ 返回商城",
		languageEnglish: "⬅️ Back to store",
	},
	"balance.unavailable": {
		languageChinese: "暂时无法查询余额，请稍后再试。",
		languageEnglish: "Balance is temporarily unavailable. Please try again later.",
	},
	"support.choose_order": {
		languageChinese: "请选择需要申请售后的订单：",
		languageEnglish: "Choose the order you need help with:",
	},
	"support.no_orders": {
		languageChinese: "目前没有查到可直接选择的已付款订单。你可以手动发送订单号，核实后继续申请售后。",
		languageEnglish: "No eligible paid orders were found. You can enter an order number manually to continue.",
	},
	"help.body": {
		languageChinese: "可用功能：\n🛍️ 商城 - 浏览商品并创建订单\n📦 我的订单 - 查询付款状态\n💳 我的余额 - 查询站内余额\n💰 余额充值 - 选择金额即可创建充值单\n🧾 申请售后 - 选择订单并描述问题\n\n直接点击下方按钮即可使用各项功能。",
		languageEnglish: "Available features:\n🛍️ Store - Browse products and place orders\n📦 My orders - Check payment and delivery status\n💳 My balance - View your store balance\n💰 Top up - Create a balance top-up order\n🧾 Support - Choose an order and describe your issue\n\nUse the buttons below to continue.",
	},
	"help.link": {
		languageChinese: "\n\nTelegram 客服：",
		languageEnglish: "\n\nTelegram support: ",
	},
	"search.prompt": {
		languageChinese: "请输入商品名称或关键词，我会直接帮你筛选商品。",
		languageEnglish: "Enter a product name or keyword and I'll search the store for you.",
	},
	"support.query_error": {
		languageChinese: "暂时无法查询你的订单，请稍后再试。",
		languageEnglish: "Your orders are temporarily unavailable. Please try again later.",
	},
	"support.ticket_choose": {
		languageChinese: "请选择需要申请售后的订单：",
		languageEnglish: "Choose the order you need help with:",
	},
	"support.ticket_none": {
		languageChinese: "目前没有查到可直接选择的已付款订单。你可以手动发送订单号，核实后继续申请售后。",
		languageEnglish: "No eligible paid orders were found. You can enter an order number manually to continue.",
	},
}

func botText(language, key string) string {
	language = normalizeLanguage(language)
	if language == "" {
		language = languageChinese
	}
	if translations, ok := botMessages[key]; ok {
		if value, ok := translations[language]; ok {
			return value
		}
		if value, ok := translations[languageChinese]; ok {
			return value
		}
	}
	return key
}

func orderStatusText(language, status string) string {
	if normalizeLanguage(language) != languageEnglish {
		return orderStatusLabel(status)
	}
	switch status {
	case "pending_payment": return "Awaiting payment"
	case "paid", "delivering", "procurement_pending": return "Processing (procurement/delivery)"
	case "procurement_failed": return "Procurement failed; manual handling required"
	case "delivery_failed": return "Delivery issue; retrying"
	case "delivered": return "Delivered"
	case "expired": return "Closed"
	case "refunded": return "Refunded"
	default: return status
	}
}

func languageKeyboard(language string) map[string]any {
	return map[string]any{"inline_keyboard": [][]map[string]string{
		{{"text": botText(language, "language.button.zh"), "callback_data": "language:set:zh"}},
		{{"text": botText(language, "language.button.en"), "callback_data": "language:set:en"}},
	}}
}
