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

	"product.unavailable": {languageChinese:"商品不存在、已下架或暂不可购买。",languageEnglish:"This product is unavailable, unpublished, or cannot be purchased right now."},
	"product.stock": {languageChinese:"库存：",languageEnglish:"Stock: "},
	"product.unlimited": {languageChinese:"不限量交付",languageEnglish:"Unlimited"},
	"product.upstream_stock": {languageChinese:"上游库存：",languageEnglish:"Supplier stock: "},
	"product.upstream_unlimited": {languageChinese:"不限量",languageEnglish:"Unlimited"},
	"product.price": {languageChinese:"价格：",languageEnglish:"Price: "},
	"product.bulk": {languageChinese:"批量优惠：",languageEnglish:"Bulk discounts:"},
	"product.bulk_min": {languageChinese:"满 ",languageEnglish:"Buy "},
	"product.bulk_discount": {languageChinese:" 件优惠 ",languageEnglish:"+ units: "},
	"product.bulk_percent": {languageChinese:"%",languageEnglish:"% off"},
	"product.upstream_empty": {languageChinese:"⚠️ 上游库存不足。",languageEnglish:"⚠️ Supplier is out of stock."},
	"product.manual_unavailable": {languageChinese:"⚠️ 该商品的资料表单暂不可用，请联系管理员。",languageEnglish:"⚠️ The required information form is unavailable. Please contact support."},
	"product.manual_buy": {languageChinese:"填写资料并购买",languageEnglish:"Enter details & buy"},
	"product.buy": {languageChinese:"立即购买",languageEnglish:"Buy now"},
	"product.coupon_buy": {languageChinese:"🎟️ 使用优惠码购买",languageEnglish:"🎟️ Buy with coupon"},
	"product.restock": {languageChinese:"🔔 到货通知我",languageEnglish:"🔔 Notify me when available"},
	"product.buy_quantity": {languageChinese:"购买 ",languageEnglish:"Buy "},
	"product.quantity_suffix": {languageChinese:" 件",languageEnglish:""},
	"product.cart_add": {languageChinese:"🛒 加入购物车",languageEnglish:"🛒 Add to cart"},
	"product.cart_show": {languageChinese:"🛒 查看购物车",languageEnglish:"🛒 View cart"},
	"product.back": {languageChinese:"返回商品列表",languageEnglish:"Back to products"},
	"product.unsupported": {languageChinese:"⚠️ 该上游商品履约类型暂不支持自助购买。",languageEnglish:"⚠️ Self-service purchase is not supported for this supplier fulfillment type."},
	"order.detail.title": {languageChinese:"订单详情",languageEnglish:"Order details"},
	"order.detail.number": {languageChinese:"订单号：",languageEnglish:"Order: "},
	"order.detail.product": {languageChinese:"商品：",languageEnglish:"Product: "},
	"order.detail.quantity": {languageChinese:"数量：",languageEnglish:"Quantity: "},
	"order.detail.amount": {languageChinese:"金额：",languageEnglish:"Amount: "},
	"order.detail.status": {languageChinese:"状态：",languageEnglish:"Status: "},
	"order.detail.created": {languageChinese:"创建时间：",languageEnglish:"Created: "},
	"order.detail.items": {languageChinese:"商品明细：",languageEnglish:"Items:"},
	"order.detail.invite_note": {languageChinese:"该商品使用一次性邀请链接交付。为避免重复发放入群名额，请联系客服核实后处理。",languageEnglish:"This item uses a one-time invite link. Contact support to verify before requesting another delivery."},
	"order.detail.file_note": {languageChinese:"该商品包含文件与卡密两个交付步骤。为避免重复发送文件，请联系客服核实后处理。",languageEnglish:"This item includes both a file and a code. Contact support to verify before requesting another delivery."},
	"order.detail.retry_note": {languageChinese:"系统正在自动重试发货；如果长时间未收到，请申请售后。",languageEnglish:"Delivery is being retried automatically. Contact support if it does not arrive soon."},
	"order.detail.pay": {languageChinese:"💳 继续付款",languageEnglish:"💳 Continue payment"},
	"order.detail.support_verify": {languageChinese:"🧾 联系客服申请核实",languageEnglish:"🧾 Ask support to verify"},
	"order.detail.support": {languageChinese:"🧾 申请售后",languageEnglish:"🧾 Contact support"},
	"order.detail.redeliver": {languageChinese:"📥 再次获取商品",languageEnglish:"📥 Get item again"},
	"order.detail.back": {languageChinese:"⬅️ 返回订单列表",languageEnglish:"⬅️ Back to orders"},
	"order.unavailable": {languageChinese:"订单不存在或无权查看。",languageEnglish:"Order not found or access denied."},
	"payment.unavailable": {languageChinese:"该订单不存在、已付款或已关闭。",languageEnglish:"This order was not found, is already paid, or has been closed."},
	"payment.title": {languageChinese:"继续支付订单",languageEnglish:"Continue payment"},
	"payment.number": {languageChinese:"订单号：",languageEnglish:"Order: "},
	"payment.product": {languageChinese:"商品：",languageEnglish:"Product: "},
	"payment.amount": {languageChinese:"应付金额：",languageEnglish:"Amount due: "},
	"payment.exact": {languageChinese:"链上准确转账金额：",languageEnglish:"Exact on-chain transfer amount: "},
	"payment.deadline": {languageChinese:"支付截止：",languageEnglish:"Payment deadline: "},
	"payment.wallet": {languageChinese:"💳 使用站内余额支付",languageEnglish:"💳 Pay with store balance"},
	"payment.auto": {languageChinese:"🔄 自动选择支付方式",languageEnglish:"🔄 Automatically choose payment method"},
	"payment.view_order": {languageChinese:"查看订单详情",languageEnglish:"View order details"},
	"payment.back": {languageChinese:"⬅️ 返回订单列表",languageEnglish:"⬅️ Back to orders"},

	"cart.unavailable": {languageChinese:"购物车暂时不可用，请稍后重试。",languageEnglish:"The cart is temporarily unavailable. Please try again later."},
	"cart.load_error": {languageChinese:"购物车暂时无法读取，请稍后重试。",languageEnglish:"The cart could not be loaded. Please try again later."},
	"cart.empty": {languageChinese:"🛒 购物车还是空的。\\n去商品中心挑选本地卡密商品，点击“加入购物车”即可。",languageEnglish:"🛒 Your cart is empty.\\nBrowse the store and add products to your cart."},
	"cart.shop": {languageChinese:"🛍️ 去逛商城",languageEnglish:"🛍️ Browse store"},
	"cart.stock": {languageChinese:"库存 ",languageEnglish:"Stock "},
	"cart.stock_short": {languageChinese:"（库存不足）",languageEnglish:" (insufficient stock)"},
	"cart.quantity_suffix": {languageChinese:" 件",languageEnglish:" pcs"},
	"cart.remove": {languageChinese:"删除",languageEnglish:"Remove"},
	"cart.clear": {languageChinese:"🧹 清空购物车",languageEnglish:"🧹 Clear cart"},
	"cart.continue": {languageChinese:"🛍️ 继续选购",languageEnglish:"🛍️ Continue shopping"},
	"cart.checkout": {languageChinese:"✅ 统一结算（一次付款）",languageEnglish:"✅ Checkout (one payment)"},
	"cart.title": {languageChinese:"🛒 我的购物车",languageEnglish:"🛒 My cart"},
	"cart.subtotal": {languageChinese:"商品小计（未扣批量优惠）：",languageEnglish:"Subtotal (before bulk discounts):"},
	"cart.validation": {languageChinese:"符合条件的商品批量折扣会在结算时自动计算；库存、价格和商品状态也会在结算时重新校验。",languageEnglish:"Eligible bulk discounts are applied at checkout. Stock, prices, and product status are revalidated before the order is created."},
	"cart.creating": {languageChinese:"正在创建统一订单",languageEnglish:"Creating your combined order"},
	"cart.checkout_error_short": {languageChinese:"暂时无法结算",languageEnglish:"Checkout is temporarily unavailable"},
	"cart.checkout_error": {languageChinese:"购物车结算失败：",languageEnglish:"Cart checkout failed: "},
	"cart.retry": {languageChinese:"购物车内容已保留，你可以调整商品后重试。",languageEnglish:"Your cart has been kept. Adjust the items and try again."},
	"cart.created": {languageChinese:"✅ 购物车统一订单已创建！",languageEnglish:"✅ Combined cart order created!"},
	"cart.order_number": {languageChinese:"订单号：",languageEnglish:"Order: "},
	"cart.amount_due": {languageChinese:"应付总额：",languageEnglish:"Total due: "},
	"cart.exact_amount": {languageChinese:"链上准确付款金额：",languageEnglish:"Exact on-chain payment amount: "},
	"cart.expiry": {languageChinese:"支付有效期：20 分钟",languageEnglish:"Payment window: 20 minutes"},
	"cart.balance": {languageChinese:"当前站内余额：",languageEnglish:"Store balance: "},
	"cart.pay_wallet": {languageChinese:"💳 使用站内余额一次支付",languageEnglish:"💳 Pay once with store balance"},
	"cart.pay_chain": {languageChinese:"💵 使用 TRC20 USDT 一次支付",languageEnglish:"💵 Pay once with TRC20 USDT"},
	"cart.pay_hosted": {languageChinese:"🌐 使用 Epusdt GMPay 收银台",languageEnglish:"🌐 Pay via Epusdt / GMPay"},
	"cart.topup": {languageChinese:"💰 充值余额",languageEnglish:"💰 Top up balance"},
	"cart.orders": {languageChinese:"📦 查看我的订单",languageEnglish:"📦 View my orders"},
	"cart.notify_created": {languageChinese:"购物车统一订单：",languageEnglish:"Combined cart order: "},
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
