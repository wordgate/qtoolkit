# nextpay

Go SDK for the NextPay payment platform: a REST client for orders,
subscriptions, plans, wallets and recharge contracts, plus an inbound webhook
receiver for the events NextPay pushes back to your app.

## Configuration

Load your viper config once at startup; the client lazy-loads on first use. See
[`nextpay_config.yml`](./nextpay_config.yml) for the full template.

```yaml
nextpay:
  access_key: "YOUR_NEXTPAY_ACCESS_KEY" # sent as X-Access-Key on every request
```

## Client

Every network call takes a `context.Context` as its first argument.

```go
result, err := nextpay.CreateSubscription(ctx, &nextpay.SubscriptionRequest{
    UserID: "user123",
    Email:  "user@example.com",
    Code:   "pro-monthly", // plan referenced by its stable code
})
```

The rest of the surface (`CreateOrder`, `GrantSubscription`, plan CRUD,
`GetSubscriptions`, wallet ops, recharge contracts, ...) is documented inline in
[`nextpay_config.yml`](./nextpay_config.yml).

## Webhooks

NextPay `POST`s event notifications to your app's configured webhook URL. The
SDK provides framework-agnostic verification and typed decoding — it does not
depend on any web framework, so it works the same under gin, echo or net/http.

Two things the receiver must get right, both handled below:

- **Verify before decoding.** The HMAC covers the exact raw request body; verify
  it before any JSON parse or re-encode, or the signature will not match.
- **Dedupe on `event.ID`.** Delivery is at-least-once (retried up to 15 times
  plus a sweeper), so the same event can arrive more than once. Make handlers
  idempotent by remembering the `evt_<uuid>` id.

### gin

```go
r.POST("/webhooks/nextpay", func(c *gin.Context) {
    body, err := io.ReadAll(c.Request.Body)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "read body"})
        return
    }

    evt, err := nextpay.ParseWebhook(body, c.GetHeader("X-NextPay-Signature"), webhookSecret)
    if err != nil {
        // Bad signature or malformed body — 400 so NextPay stops retrying.
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    // Idempotency: if you have already processed evt.ID, ack and return.

    switch evt.Type {
    case nextpay.WebhookOrderPaid:
        d, _ := evt.OrderData()
        _ = d // fulfil the order for d.UserID / d.ObjectID
    case nextpay.WebhookSubscriptionRenewed, nextpay.WebhookSubscriptionExpired:
        d, _ := evt.SubscriptionData()
        _ = d // sync entitlement for d.UserID
    case nextpay.WebhookWalletDeducted:
        d, _ := evt.WalletData()
        _ = d // d.Amount is a signed delta (negative on deduct)
    }

    // 2xx acknowledges receipt. Return 5xx to have NextPay retry later.
    c.JSON(http.StatusOK, gin.H{"received": true})
})
```

### Events

| Type constant | Event | Accessor |
|---|---|---|
| `WebhookOrderPaid` / `WebhookOrderExpired` / `WebhookOrderFailed` | `order.*` | `evt.OrderData()` |
| `WebhookSubscriptionCreated` / `Renewed` / `Cancelled` / `Expired` / `PastDue` / `Paused` / `Resumed` | `subscription.*` | `evt.SubscriptionData()` |
| `WebhookContractActivated` / `Cancelled` | `contract.*` | `evt.ContractData()` |
| `WebhookChargeSucceeded` / `Failed` | `charge.*` | `evt.ChargeData()` |
| `WebhookWalletDeposited` / `Deducted` | `wallet.*` | `evt.WalletData()` |

Each accessor returns an error if called on an event that does not carry that
data type, so a wrong `switch` arm fails loudly instead of yielding a zero value.

### Signature format

`X-NextPay-Signature: t=<unix>,v1=<hex>` where the signature is
`HMAC-SHA256(secret, "<t>.<rawBody>")`. `VerifyWebhookSignature` exposes this
check on its own if you need to verify without decoding.
