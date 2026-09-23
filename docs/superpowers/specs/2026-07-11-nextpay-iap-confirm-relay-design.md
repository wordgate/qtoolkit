# NextPay SDK — Apple IAP 核销中转方法

**日期**：2026-07-11
**范围**：`github.com/wordgate/qtoolkit/nextpay` 客户端 SDK。
**依赖**：NextPay 后端的 IAP 端点契约（另一份 spec 在 nextpay 仓库：`docs/superpowers/specs/2026-07-11-apple-iap-onetime-payment-design.md`）。**本 spec 排在其后实现**——端点契约稳定后再动。

---

## 1. 背景

接入方后端用本 SDK（`X-Access-Key` 认证）调 NextPay。NextPay 新增 Apple IAP 一次性付款：客户端（iOS）完成 StoreKit 购买拿到 `transactionId`，**一律经接入方后端中转**给 NextPay 核销（客户端绝不直连 NextPay，AccessKey 只在服务端）。

现有 SDK 已有 `CreateOrder` / `CreateSubscription`，但**没有 confirm/核销方法**——因为 Stripe 的 confirm 发生在浏览器支付页，不需要接入方后端参与。IAP 需要接入方后端主动发起核销，故补一个中转方法。

## 2. 目标

让接入方后端**一行**完成 IAP 核销：把 iOS 传上来的 `transactionId` 连同 `orderUUID` 转交 NextPay，NextPay 反查苹果 + 置 paid + 履约。

## 3. 改动

### 3.1 请求类型增补 `AppleProductID`

`OrderRequest`（钱包充值类 IAP 订单）与 `SubscriptionRequest`（订阅购买类 IAP 订单）各加一个可选字段：

```go
type OrderRequest struct {
    // ...现有字段...
    AppleProductID string `json:"appleProductId,omitempty"` // 给出即走 IAP 钱包充值；金额由 NextPay 目录派生（客户端 Amount 被忽略）
}

type SubscriptionRequest struct {
    // ...现有字段...
    AppleProductID string `json:"appleProductId,omitempty"` // 给出即走 IAP 订阅购买
}
```

给出 `AppleProductID` 时，NextPay 按 `AppleIapProduct` 目录派生权威金额；不给出时维持现有非 IAP 行为。SDK 侧只是透传字段，无额外逻辑。

### 3.2 新增核销中转方法 `ConfirmAppleIAP`

```go
// ConfirmResult 对应 NextPay 的 ConfirmPaymentResponse。
// IAP 核销成功时 CheckoutURL 为空、RedirectURL = 订单 SuccessURL。
type ConfirmResult struct {
    CheckoutURL string `json:"checkoutUrl,omitempty"`
    RedirectURL string `json:"redirectUrl,omitempty"`
}

// ConfirmAppleIAP 把 iOS StoreKit 交易提交给 NextPay 核销一次性 IAP 订单。
// 成功返回（err==nil）即表示订单已置 paid 并完成履约（发授权 / 充钱包）。
func ConfirmAppleIAP(ctx context.Context, orderUUID, transactionID string) (*ConfirmResult, error)
```

实现遵循现有 SDK 模式（`do` → `doRequest` → `decodeData`）：

```go
func ConfirmAppleIAP(ctx context.Context, orderUUID, transactionID string) (*ConfirmResult, error) {
    return do(ctx, func(ctx context.Context, c *Client) (*ConfirmResult, error) {
        return c.confirmAppleIAP(ctx, orderUUID, transactionID)
    })
}

func (c *Client) confirmAppleIAP(ctx context.Context, orderUUID, transactionID string) (*ConfirmResult, error) {
    body := map[string]any{
        "paymentMethod":      "apple_iap",
        "appleTransactionId": transactionID,
    }
    resp, err := c.doRequest(ctx, "POST", "/api/checkout/"+orderUUID+"/confirm", body)
    if err != nil {
        return nil, err
    }
    return decodeData[ConfirmResult](resp.Data)
}
```

- 打的是**认证** confirm 端点 `POST /api/checkout/:orderUuid/confirm`（`X-Access-Key` 命中 NextPay 的 `logicConfirmPayment` → `apple_iap` 分支）。
- 错误经现有 envelope 机制（`apiResp.Code != 0` → `*APIError`）返回；调用方据此区分「productId 不符 / 交易查不到 / 已核销」等业务错误与 HTTP/网络错误。

## 4. 接入方用法示意

```go
// 订阅购买（CreateSubscription 返回 OrderID == order.UUID）
sub, _ := nextpay.CreateSubscription(ctx, &nextpay.SubscriptionRequest{
    UserID: uid, Email: email, Code: "pro-yearly", AppleProductID: "com.app.pro.yearly",
})
res, err := nextpay.ConfirmAppleIAP(ctx, sub.OrderID, txID)

// 钱包充值（CreateOrder 返回 OrderID == order.UUID）
ord, _ := nextpay.CreateOrder(ctx, &nextpay.OrderRequest{
    UserID: uid, Email: email, ProductName: "Coins $10", AppleProductID: "com.app.coins.10",
})
res, err := nextpay.ConfirmAppleIAP(ctx, ord.OrderID, txID)
if err != nil { /* 业务错误或苹果核销失败，iOS 交易保持 unfinished 可重试 */ }
```

## 5. 待与 NextPay 契约对齐的点（实现前确认）

1. **confirm 用的标识：已确认统一。** 钱包类 `CreateOrder → OrderResult.OrderID` 与订阅类 `CreateSubscription → SubscriptionResult.OrderID` **都等于 `order.UUID`**（NextPay 两个建单逻辑均返回 `order.UUID`）。故 `ConfirmAppleIAP(ctx, orderUUID, txID)` 对两类统一适用，都走 `/api/checkout/:orderUuid/confirm`。
2. **confirm 响应体**：以 NextPay `ConfirmPaymentResponse` 实际字段为准（当前为 `CheckoutURL` / `RedirectURL`）。若 NextPay 后续加显式 `status` 字段，`ConfirmResult` 同步增补。
3. SDK 不做去重 / 幂等——由 NextPay 端保证（`(app_id, apple_transaction_id)` 唯一 + 订单乐观锁）。SDK 层重复调用是安全的（幂等返回成功）。

## 6. 测试

- `ConfirmAppleIAP` 单测：打桩 HTTP，断言 POST 路径 `/api/checkout/{uuid}/confirm`、body 含 `paymentMethod=apple_iap` 与 `appleTransactionId`、`X-Access-Key` 头。
- 错误路径：envelope `code!=0` → `*APIError`；HTTP ≥400 无 JSON → 透传错误。
- 请求类型序列化：`AppleProductID` 有值 / 空值（omitempty）。

## 7. 明确排除

- 不在 SDK 里做 StoreKit / 收据解析（那是 iOS 客户端职责）。
- 不做 App Store Server Notifications 处理。
- 不引入 IAP 凭证 —— SDK 只中转 `transactionId`，苹果反查在 NextPay 服务端。
