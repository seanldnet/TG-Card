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

func detectLanguageWithDefault(languageCode, defaultLanguage string) string {
	if language := normalizeLanguage(languageCode); language != "" { return language }
	if language := normalizeLanguage(defaultLanguage); language != "" { return language }
	return languageChinese
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
	"cart.empty": {languageChinese:"🛒 购物车还是空的。\n去商品中心挑选本地卡密商品，点击“加入购物车”即可。",languageEnglish:"🛒 Your cart is empty.\nBrowse the store and add products to your cart."},
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
	"cart.overflow": {languageChinese:"购物车金额超出可计算范围，请联系管理员。",languageEnglish:"The cart total exceeds the supported calculation range. Please contact support."},
	"cart.item_kinds": {languageChinese:"商品种类：",languageEnglish:"Product types: "},

	"cart.opening": {languageChinese:"正在打开购物车",languageEnglish:"Opening your cart"},
	"cart.adding": {languageChinese:"正在加入购物车",languageEnglish:"Adding to cart"},
	"cart.add_error": {languageChinese:"加入购物车失败：",languageEnglish:"Could not add item to cart: "},
	"cart.invalid_action": {languageChinese:"购物车操作无效",languageEnglish:"Invalid cart action"},
	"cart.updating": {languageChinese:"正在更新购物车",languageEnglish:"Updating cart"},
	"cart.update_error": {languageChinese:"购物车更新失败：",languageEnglish:"Could not update cart: "},
	"cart.clearing": {languageChinese:"正在清空购物车",languageEnglish:"Clearing cart"},
	"cart.clear_error": {languageChinese:"清空购物车失败：",languageEnglish:"Could not clear cart: "},
	"payment.unauthorized": {languageChinese:"无权操作此订单",languageEnglish:"You are not authorized to manage this order"},
	"payment.wallet_failed_short": {languageChinese:"余额支付失败",languageEnglish:"Balance payment failed"},
	"payment.wallet_failed": {languageChinese:"余额支付失败：",languageEnglish:"Balance payment failed: "},
	"payment.wallet_success": {languageChinese:"余额支付成功",languageEnglish:"Balance payment successful"},
	"payment.paid_queued": {languageChinese:"付款已成功，商品已进入后台发货队列，请稍后查看订单。",languageEnglish:"Payment succeeded. Your order is queued for delivery; check your orders shortly."},
	"payment.delivery_processing": {languageChinese:"付款已成功，发货正在处理中，请稍后查看订单。",languageEnglish:"Payment succeeded. Delivery is being processed; check your orders shortly."},
	"payment.delivery_load_error": {languageChinese:"付款已成功，但发货信息暂时无法读取，系统会自动重试。",languageEnglish:"Payment succeeded, but delivery details are temporarily unavailable. The system will retry automatically."},
	"payment.delivery_retrying": {languageChinese:"付款已成功，但自动发货遇到问题，系统会自动重试。",languageEnglish:"Payment succeeded, but automatic delivery encountered an issue. The system will retry."},
	"payment.wallet_delivered": {languageChinese:"站内余额支付成功，商品已发出。",languageEnglish:"Balance payment successful. Your item has been delivered."},
	"payment.order_processed": {languageChinese:"订单不存在或已处理",languageEnglish:"Order not found or already processed"},
	"payment.method_unavailable": {languageChinese:"支付方式暂不可用",languageEnglish:"Payment method is currently unavailable"},
	"payment.route_error": {languageChinese:"暂时无法分配支付方式，请稍后重试",languageEnglish:"A payment method could not be assigned. Please try again later"},
	"payment.trc20_unavailable": {languageChinese:"TRC20 收款暂不可用",languageEnglish:"TRC20 payments are currently unavailable"},
	"payment.provider_unavailable": {languageChinese:"分配到的支付方式暂不可用",languageEnglish:"The assigned payment method is unavailable"},
	"payment.connecting": {languageChinese:"正在连接支付收银台",languageEnglish:"Connecting to the payment gateway"},
	"payment.checkout_error": {languageChinese:"暂时无法创建在线收银台，请稍后重试；系统会先查询原支付单，避免重复创建。",languageEnglish:"The hosted checkout could not be created. Please try again later; the existing payment is checked first to avoid duplicates."},
	"payment.order_closed": {languageChinese:"订单不存在、已关闭或已付款",languageEnglish:"Order not found, closed, or already paid"},
	"payment.chain_unavailable": {languageChinese:"链上付款暂未启用",languageEnglish:"On-chain payments are not enabled"},
	"payment.chain_unavailable_help": {languageChinese:"链上付款暂未启用，请联系管理员。",languageEnglish:"On-chain payments are not enabled. Please contact support."},
	"payment.info_shown": {languageChinese:"已显示付款信息",languageEnglish:"Payment details displayed"},
	"payment.gmpay_unavailable": {languageChinese:"GMPay 暂未启用",languageEnglish:"GMPay is not enabled"},
	"payment.gmpay_error": {languageChinese:"暂时无法创建 GMPay 收银台，请稍后重试；如已提交过请求，系统会先查询原支付单，避免重复创建。",languageEnglish:"The GMPay checkout could not be created. Please try again later; the existing payment is checked first to avoid duplicates."},
	"payment.order_access_denied": {languageChinese:"订单不存在或无权查看",languageEnglish:"Order not found or access denied"},
	"payment.gateway_checking": {languageChinese:"正在向支付网关查询状态",languageEnglish:"Checking payment status with the gateway"},
	"payment.status_error": {languageChinese:"暂时无法查询支付状态，请稍后重试。",languageEnglish:"Payment status is temporarily unavailable. Please try again later."},
	"payment.gateway_paid": {languageChinese:"✅ 支付网关已确认到账，订单已进入自动发货流程。请在“我的订单”查看最新状态。",languageEnglish:"✅ The gateway confirmed payment. Your order is queued for automatic delivery; check My Orders for updates."},
	"payment.gateway_expired": {languageChinese:"该支付单已过期，请联系管理员处理或重新创建订单。",languageEnglish:"This payment has expired. Contact support or create a new order."},
	"payment.gateway_pending": {languageChinese:"支付网关暂未确认到账。请勿重复转账，稍后可再次刷新状态。",languageEnglish:"The gateway has not confirmed payment yet. Do not transfer again; refresh the status later."},
	"payment.info_title": {languageChinese:"付款信息",languageEnglish:"Payment details"},
	"payment.address": {languageChinese:"TRC20 USDT 收款地址：",languageEnglish:"TRC20 USDT address: "},
	"payment.instructions": {languageChinese:"请在有效期内完成转账，系统确认后自动发货。",languageEnglish:"Complete the transfer before the deadline. Delivery starts automatically after confirmation."},
	"payment.copy_address": {languageChinese:"📋 复制收款地址",languageEnglish:"📋 Copy payment address"},
	"payment.copy_exact": {languageChinese:"📋 复制准确金额",languageEnglish:"📋 Copy exact amount"},
	"payment.copy_amount": {languageChinese:"📋 复制订单金额",languageEnglish:"📋 Copy order amount"},
	"payment.copy_order": {languageChinese:"📋 复制订单号",languageEnglish:"📋 Copy order number"},
	"payment.view_orders": {languageChinese:"我的订单",languageEnglish:"My orders"},
	"payment.back_store": {languageChinese:"返回商城",languageEnglish:"Back to store"},
	"payment.trc20_assigned": {languageChinese:"已为本订单分配 TRC20 USDT",languageEnglish:"TRC20 USDT payment details assigned to this order"},
	"payment.qr_fallback": {languageChinese:"二维码无法显示，请复制地址付款。",languageEnglish:"The QR code could not be displayed. Copy the address to pay."},
	"payment.hosted_ready": {languageChinese:"订单 ",languageEnglish:"Order "},
	"payment.hosted_instructions": {languageChinese:" 的在线收银台已准备好。请完成付款，系统确认后自动发货。",languageEnglish:" checkout is ready. Complete payment and delivery will start after confirmation."},
	"payment.gmpay_instructions": {languageChinese:" 的在线收银台已准备好。请在支付有效期内完成付款，系统收到签名验证通过的回调后会自动确认并发货。",languageEnglish:" GMPay checkout is ready. Pay before it expires; delivery starts after the signed payment callback is verified."},
	"payment.open_checkout": {languageChinese:"🌐 打开收银台",languageEnglish:"🌐 Open checkout"},
	"payment.open_gmpay": {languageChinese:"🌐 打开 GMPay 收银台",languageEnglish:"🌐 Open GMPay checkout"},
	"payment.refresh_status": {languageChinese:"🔄 我已付款，刷新状态",languageEnglish:"🔄 I've paid — refresh status"},

	"delivery.order": {languageChinese: "订单：", languageEnglish: "Order: "},
	"delivery.keep_secret": {languageChinese: "请妥善保存，不要转发给他人。", languageEnglish: "Keep these details safe and do not share them with anyone."},
	"delivery.invite_intro": {languageChinese: "你的专属入群邀请链接（仅限 1 人使用）：", languageEnglish: "Your personal group invite link (one use only):"},
	"delivery.invite_warning": {languageChinese: "请勿转发，链接过期后无法使用。", languageEnglish: "Do not share this link. It cannot be used after it expires."},
	"delivery.upstream_payload": {languageChinese: "上游商品交付内容：", languageEnglish: "Supplier fulfillment details:"},
	"delivery.card_codes": {languageChinese: "配套卡密：", languageEnglish: "Your item codes:"},

	"support.submit_success": {languageChinese: "售后申请已提交！", languageEnglish: "✅ Your support request has been submitted!"},
	"support.ticket_number": {languageChinese: "工单号：", languageEnglish: "Ticket: "},
	"support.order_number": {languageChinese: "订单号：", languageEnglish: "Order: "},
	"support.status_pending": {languageChinese: "状态：待受理。管理员回复后会通过本机器人通知你。", languageEnglish: "Status: Pending review. You'll be notified here when an administrator replies."},
	"support.attachment_saved": {languageChinese: "文字说明和附件已一起保存。你可以继续补充文字、图片或文件。", languageEnglish: "Your description and attachment were saved. You can add more text, images, or files."},
	"support.added": {languageChinese: "补充内容已添加到工单。", languageEnglish: "Your message has been added to the ticket."},
	"support.recreated": {languageChinese: " 已结单，已为你重新创建工单 #", languageEnglish: " is closed. A new ticket #"},
	"support.continue_uploads": {languageChinese: "。你可以继续发送文字、图片或文件。", languageEnglish: ". You can continue sending text, images, or files."},

	"support.update_title": {languageChinese: "售后工单更新", languageEnglish: "Support ticket update"},
	"support.current_status": {languageChinese: "当前状态：", languageEnglish: "Status: "},
	"support.admin_reply": {languageChinese: "管理员回复：", languageEnglish: "Support team reply:"},

	"owner.menu": {languageChinese: "🛠️ 主人管理", languageEnglish: "🛠️ Owner console"},
	"owner.support_menu": {languageChinese: "🧾 待处理工单", languageEnglish: "🧾 Pending tickets"},
	"owner.back": {languageChinese: "返回", languageEnglish: "Back"},
	"owner.overview.db_unavailable": {languageChinese: "数据库暂不可用。", languageEnglish: "The database is temporarily unavailable."},
	"owner.overview.query_failed": {languageChinese: "经营数据暂时无法读取，请稍后重试。", languageEnglish: "Business metrics are temporarily unavailable. Please try again later."},
	"owner.overview.title": {languageChinese: "🛠️ Bot 管理指挥台", languageEnglish: "🛠️ Bot owner console"},
	"owner.overview.users": {languageChinese: "👥 累计用户：", languageEnglish: "👥 Total users: "},
	"owner.overview.products": {languageChinese: "🛍️ 在售商品：", languageEnglish: "🛍️ Active products: "},
	"owner.overview.pending_orders": {languageChinese: "⏳ 待付款订单：", languageEnglish: "⏳ Pending orders: "},
	"owner.overview.paid_orders": {languageChinese: "✅ 已付款/交付订单：", languageEnglish: "✅ Paid / delivered orders: "},
	"owner.overview.tickets": {languageChinese: "🧾 待处理售后：", languageEnglish: "🧾 Open support tickets: "},
	"owner.overview.instructions": {languageChinese: "请选择下方管理功能，按提示完成操作：", languageEnglish: "Choose an action below and follow the prompts:"},
	"owner.menu.stats": {languageChinese: "📊 经营数据总览", languageEnglish: "📊 Business overview"},
	"owner.menu.users": {languageChinese: "👥 用户查询", languageEnglish: "👥 User lookup"},
	"owner.menu.orders": {languageChinese: "🧾 最新订单", languageEnglish: "🧾 Recent orders"},
	"owner.menu.stock": {languageChinese: "📦 商品库存", languageEnglish: "📦 Product inventory"},
	"owner.menu.add_stock": {languageChinese: "➕ 添加库存", languageEnglish: "➕ Add inventory"},
	"owner.menu.broadcast": {languageChinese: "📣 群发通知", languageEnglish: "📣 Broadcast"},
	"owner.menu.ban": {languageChinese: "🔒 封禁用户", languageEnglish: "🔒 Ban user"},
	"owner.menu.unban": {languageChinese: "🔓 解封用户", languageEnglish: "🔓 Unban user"},
	"owner.menu.close_order": {languageChinese: "⛔ 关闭待付款订单", languageEnglish: "⛔ Close pending order"},
	"owner.menu.back_home": {languageChinese: "⬅️ 返回个人首页", languageEnglish: "⬅️ Back to home"},
	"owner.support.list_error": {languageChinese: "待处理工单暂时无法读取。", languageEnglish: "Pending support tickets are temporarily unavailable."},
	"owner.support.list_empty": {languageChinese: "目前没有待处理或处理中的工单。", languageEnglish: "There are no pending or in-progress tickets."},
	"owner.support.list_title": {languageChinese: "待处理/处理中工单（", languageEnglish: "Pending / in-progress tickets ("},
	"owner.support.count_suffix": {languageChinese: " 条，第 ", languageEnglish: " tickets, page "},
	"owner.support.page_separator": {languageChinese: "/", languageEnglish: "/"},
	"owner.support.page_suffix": {languageChinese: " 页）：", languageEnglish: "):"},
	"owner.support.previous": {languageChinese: "上一页", languageEnglish: "Previous"},
	"owner.support.next": {languageChinese: "下一页", languageEnglish: "Next"},
	"owner.support.back": {languageChinese: "返回", languageEnglish: "Back"},
	"owner.support.not_found": {languageChinese: "工单不存在或不属于当前机器人。", languageEnglish: "Ticket not found or it does not belong to this Bot."},
	"owner.support.history_error": {languageChinese: "工单记录读取失败。", languageEnglish: "Failed to load ticket history."},
	"owner.support.customer": {languageChinese: "用户", languageEnglish: "Customer"},
	"owner.support.agent": {languageChinese: "客服", languageEnglish: "Support"},
	"owner.support.ticket_number": {languageChinese: "工单号：", languageEnglish: "Ticket: "},
	"owner.support.user": {languageChinese: "用户：", languageEnglish: "Customer: "},
	"owner.support.bot": {languageChinese: "Bot：", languageEnglish: "Bot: "},
	"owner.support.order": {languageChinese: "关联订单：", languageEnglish: "Order: "},
	"owner.support.status": {languageChinese: "状态：", languageEnglish: "Status: "},
	"owner.support.attachments": {languageChinese: "[图片 %d 张 / 文件 %d 个，已在上面转发]\n", languageEnglish: "[%d image(s) / %d file(s), forwarded above]\n"},
	"owner.support.history": {languageChinese: "\n沟通记录：\n", languageEnglish: "\nConversation history:\n"},
	"owner.support.start_reply": {languageChinese: "开始回复", languageEnglish: "Reply"},
	"owner.support.resolve": {languageChinese: "结单", languageEnglish: "Resolve"},
	"owner.support.back_list": {languageChinese: "返回列表", languageEnglish: "Back to list"},
	"owner.support.reply_prompt": {languageChinese: "请直接发送要回复用户的内容（文字/图片/文件均可）。当前工单：", languageEnglish: "Send your reply to the customer (text, image, or file). Current ticket: "},
	"owner.support.closed": {languageChinese: "该工单已结单。", languageEnglish: "This ticket is already closed."},

	"owner.support.session_update_failed": {languageChinese: "无法开启回复会话：工单状态更新失败，请稍后重试。", languageEnglish: "Unable to start the reply session because the ticket status could not be updated. Try again later."},
	"owner.support.session_open_failed": {languageChinese: "工单已进入处理中，但回复会话未能开启，请重新点击开始回复。", languageEnglish: "The ticket is now in progress, but the reply session could not be opened. Select Reply again."},
	"owner.support.resolve_failed": {languageChinese: "结单失败：", languageEnglish: "Failed to resolve ticket: "},
	"owner.support.confirm_invalid": {languageChinese: "确认操作无效，请重新打开工单。", languageEnglish: "Invalid confirmation. Reopen the ticket and try again."},
	"owner.support.confirm_expired": {languageChinese: "这条确认操作已过期或当前已切换其他工单，请重新打开工单后操作。", languageEnglish: "This confirmation expired or another ticket is active. Reopen the ticket and try again."},
	"owner.support.ticket_missing": {languageChinese: "工单不存在。", languageEnglish: "Ticket not found."},
	"owner.support.continue_reply": {languageChinese: "继续回复即可。", languageEnglish: "You can continue replying."},
	"owner.support.ticket_closed_or_missing": {languageChinese: "该工单已结单或不存在，请重新选择待处理工单。", languageEnglish: "This ticket is closed or no longer exists. Select another pending ticket."},
	"owner.support.message_check_failed": {languageChinese: "暂时无法确认消息是否已处理，请稍后重试。", languageEnglish: "Unable to verify whether this message was processed. Try again later."},
	"owner.support.message_already_processed": {languageChinese: "这条消息已处理，请继续发送下一条内容。", languageEnglish: "This message has already been processed. Send the next message when ready."},
	"owner.support.chat_invalid": {languageChinese: "用户会话地址无效，工单保持处理中。", languageEnglish: "The customer's chat address is invalid. The ticket remains in progress."},
	"owner.support.reply_prefix": {languageChinese: "客服回复：", languageEnglish: "Support reply:"},
	"owner.support.send_failed": {languageChinese: "发送给用户失败：", languageEnglish: "Failed to send the reply: "},
	"owner.support.send_failed_suffix": {languageChinese: "\n工单保持处理中，可重新发送新的回复。", languageEnglish: "\nThe ticket remains in progress. You can send a new reply."},
	"owner.support.timeline_save_failed": {languageChinese: "消息已发送给用户，但沟通记录保存失败，请检查数据库后再继续操作。", languageEnglish: "The message was delivered, but the conversation history could not be saved. Check the database before continuing."},
	"owner.support.confirm_prompt": {languageChinese: "已发送给用户。是否结单？", languageEnglish: "Reply sent. Resolve this ticket?"},
	"owner.support.confirm_yes": {languageChinese: "是，结单", languageEnglish: "Yes, resolve"},
	"owner.support.confirm_no": {languageChinese: "否，继续沟通", languageEnglish: "No, continue"},

	"owner.support.open_ticket": {languageChinese: "进入工单 ", languageEnglish: "Open ticket "},
	"owner.support.pending_list": {languageChinese: "待处理列表", languageEnglish: "Pending tickets"},
	"owner.support.new_ticket": {languageChinese: "🧾 新售后工单：", languageEnglish: "🧾 New support ticket: "},
	"owner.support.media_forwarded": {languageChinese: "媒体凭证已在上方转发。", languageEnglish: "Attachments were forwarded above."},
	"owner.support.reopened": {languageChinese: "⚠️ 用户表示问题仍未解决，工单已重新开启。", languageEnglish: "⚠️ The customer says the issue is unresolved. The ticket has been reopened."},
	"owner.support.followup": {languageChinese: "🧾 工单补充消息：", languageEnglish: "🧾 Ticket follow-up: "},
	"support.resolved.default_result": {languageChinese: "工单已由客服处理并结单。", languageEnglish: "The support team has resolved and closed this ticket."},
	"support.resolved.title": {languageChinese: "售后工单已结单", languageEnglish: "Support ticket resolved"},
	"support.resolved.result": {languageChinese: "处理结果：", languageEnglish: "Resolution: "},
	"support.resolved.prompt": {languageChinese: "如仍有问题可直接回复本消息继续沟通。", languageEnglish: "If you still need help, reply to this message to continue the conversation."},
	"support.resolved.solved": {languageChinese: "问题已解决", languageEnglish: "Resolved"},
	"support.resolved.reopen": {languageChinese: "仍有问题，继续沟通", languageEnglish: "Still need help"}
	"owner.support.forwarded_user": {languageChinese: "用户补充：\n", languageEnglish: "Customer follow-up:\n"},
	"owner.support.forwarded_ticket": {languageChinese: "🧾 工单补充消息：", languageEnglish: "🧾 Ticket follow-up: "},
	"owner.support.question": {languageChinese: "问题：", languageEnglish: "Issue: "}
	"owner.support.reply_keyboard_back": {languageChinese: "返回", languageEnglish: "Back"},
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


func supportStatusText(language, status string) string {
	if normalizeLanguage(language) != languageEnglish { return status }
	switch status {
	case "open": return "Open"
	case "in_progress": return "In progress"
	case "waiting_user": return "Waiting for your reply"
	case "resolved": return "Resolved"
	case "closed": return "Closed"
	case "rejected": return "Rejected"
	default: return status
	}
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
