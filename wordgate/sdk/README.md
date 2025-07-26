# Wordgate 同步 SDK

Wordgate 同步 SDK 是一个用于与Wordgate API进行交互的工具包，主要用于同步产品、会员等级和应用配置。

## 功能特点

- 从YAML/JSON配置文件加载配置
- 从Markdown文件中提取产品信息（相对于配置文件所在目录）
- 直接在配置文件中定义产品
- 支持会员等级同步
- 支持应用配置同步
- 提供干运行模式，用于预览同步数据而不实际发送请求

## 安装

### 使用预编译的二进制文件

从[GitHub Releases](https://github.com/allnationconnect/mods/wordgate/sdk/releases)页面下载最新版本的预编译二进制文件。

### 在Mac上使用Homebrew安装

```bash
# 添加Homebrew tap
brew tap allnationconnect/wordgate

# 安装WordGate
brew install wordgate
```

### 在Ubuntu/Debian上使用apt-get安装

```bash
# 添加WordGate APT仓库
echo "deb [trusted=yes] https://raw.githubusercontent.com/allnationconnect/sdk/apt-repository/apt-repo/ /" | sudo tee /etc/apt/sources.list.d/wordgate.list

# 更新包列表
sudo apt-get update

# 安装WordGate
sudo apt-get install wordgate
```

### 从源码构建

要从源码构建，您需要安装Go 1.20或更高版本:

```bash
git clone https://github.com/allnationconnect/mods/wordgate/sdk.git
cd sdk
go build -o wordgate ./cmd/wordgate/main.go
```

## 使用示例

### 基本用法

```go
package main

import (
    "fmt"
    "log"
    "path/filepath"
    "github.com/allnationconnect/mods/wordgate/sdk"
)

func main() {
    // 获取配置文件路径和目录
    configPath := "config.yaml"
    absConfigPath, _ := filepath.Abs(configPath)
    configDir := filepath.Dir(absConfigPath)
    
    // 加载配置
    config, err := sdk.LoadConfig(absConfigPath)
    if err != nil {
        log.Fatalf("加载配置失败: %v", err)
    }

    // 创建客户端
    client := sdk.NewClient(config, configDir)

    // 执行同步
    result, err := client.SyncAll()
    if err != nil {
        log.Fatalf("同步失败: %v", err)
    }

    // 处理结果
    fmt.Printf("同步状态: %v\n", result.Success)
    
    // 产品同步结果
    fmt.Printf("产品总数: %d\n", result.Products.Total)
    fmt.Printf("新建产品: %d\n", result.Products.Created)
    fmt.Printf("更新产品: %d\n", result.Products.Updated)
}
```

### 干运行模式

```go
// 执行干运行，不会发送实际请求
dryRunResult, err := client.DryRun()
if err != nil {
    log.Fatalf("干运行失败: %v", err)
}

// 输出将要同步的产品
fmt.Printf("产品数量: %d\n", len(dryRunResult.Products))
for _, product := range dryRunResult.Products {
    fmt.Printf("产品: %s (%s) 价格: %.2f\n", 
        product.Name, 
        product.Code, 
        float64(product.Price)/100)
}
```

### 仅同步产品

```go
// 只同步产品
productResult, err := client.SyncProducts()
if err != nil {
    log.Fatalf("产品同步失败: %v", err)
}

fmt.Printf("同步状态: %v\n", productResult.Success)
fmt.Printf("新建产品: %d\n", productResult.Created)
```

### 订单管理功能

```go
// 创建订单
orderRequest := &sdk.CreateOrderRequest{
    Items: []sdk.OrderItem{
        {
            ItemCode: "COURSE001", // 产品代码
            Quantity: 1,
            ItemType: "product", // 商品类型：商品
        },
    },
    CouponCode: "DISCOUNT10", // 优惠券代码（可选）
    AddressID: 1,
    Customer: sdk.OrderCustomer{
        Provider: "wechat",
        UID: "wx_user_123456",
    },
}

// 创建订单
order, err := client.CreateOrder(orderRequest)
if err != nil {
    log.Fatalf("创建订单失败: %v", err)
}

fmt.Printf("订单创建成功: %s, 金额: %.2f\n", 
    order.OrderNo, float64(order.Amount)/100)

// 查询订单
orderDetail, err := client.GetOrder("ORDER12345")
if err != nil {
    log.Fatalf("查询订单失败: %v", err)
}

fmt.Printf("订单号: %s, 金额: %.2f, 状态: %v\n", 
    orderDetail.OrderNo,
    float64(orderDetail.Amount)/100,
    orderDetail.IsPaid ? "已支付" : "未支付")
```

## 配置文件示例

```yaml
wordgate:
  # 应用基本信息
  app:
    name: "我的应用"
    description: "应用描述"
    currency: "CNY"
  
  # API连接信息
  base_url: "https://api.wordgate.52j.me"  
  appCode: "your-app-code"
  app_secret: "your-app-secret"
  enable_purchase: true
  
  # 产品配置
  products:
    files:
      - "content/courses/*.md"  # 路径相对于配置文件所在目录
    items:
      - code: "COURSE001"
        name: "示例课程"
        price: 9900
  
  # 会员等级配置
  membership:
    tiers:
      - code: "FREE"
        name: "免费会员"
        level: 0
        is_default: true
      - code: "PRO"
        name: "专业会员"
        level: 1
        prices:
          - period_type: "month"
            price: 9900
            original_price: 19900
```

## API 参考

### 配置加载

- `LoadConfig(filePath string) (*WordgateConfig, error)`: 从文件加载配置

### 客户端操作

- `NewClient(config *WordgateConfig, configDir string) *Client`: 创建新的客户端
- `SyncProducts() (*ProductSyncResponse, error)`: 同步产品
- `SyncMembershipTiers() (*MembershipSyncResponse, error)`: 同步会员等级
- `SyncAppConfig() (*AppConfigSyncResponse, error)`: 同步应用配置
- `SyncAll() (*SyncAllResponse, error)`: 同步所有内容
- `DryRun() (*DryRunResult, error)`: 执行干运行

### 内容处理

- `NewContentProcessor(configDir string, config *WordgateConfig) *ContentProcessor`: 创建内容处理器
- `Process() ([]Product, error)`: 处理内容文件，提取产品信息

### 订单操作

- `CreateOrder(request *CreateOrderRequest) (*OrderResponse, error)`: 创建订单
- `GetOrder(orderNo string) (*OrderDetailResponse, error)`: 获取订单详情

### 常量

- `ItemTypeProduct`: 普通商品类型
- `ItemTypeMembership`: 会员商品类型

## 快速开始

### 获取示例配置

如果您使用命令行工具，可以通过以下命令获取一个完整的示例配置：

```bash
wordgate -print-demo > config.yaml
```

这将生成一个包含所有可用配置选项的示例文件，以及一个示例Markdown文件，展示了如何在内容文件中定义产品信息。

### 使用SDK创建客户端

// ... existing code ... 

## 在其他私有项目中使用

### 方法1：使用 Go Modules（推荐）

1. 在您的项目中添加依赖：
```bash
go get github.com/allnationconnect/mods/wordgate/sdk
```

2. 在您的 `go.mod` 文件中添加：
```go
require github.com/allnationconnect/mods/wordgate/sdk v0.0.0
replace github.com/allnationconnect/mods/wordgate/sdk => github.com/allnationconnect/mods/wordgate/sdk v0.0.0
```

3. 确保您的项目有权限访问私有仓库：
```bash
git config --global url."git@github.com:".insteadOf "https://github.com/"
```

### 方法2：使用 Git Submodule

1. 在您的项目中添加子模块：
```bash
git submodule add https://github.com/allnationconnect/wordgate.git
git submodule update --init --recursive
```

2. 在您的代码中引用SDK：
```go
import "github.com/allnationconnect/mods/wordgate/sdk"
```

### 方法3：使用私有 Go Proxy

1. 设置私有代理：
```bash
export GOPROXY=your-private-proxy-url
```

2. 正常使用 go get 命令：
```bash
go get github.com/allnationconnect/mods/wordgate/sdk
```

// ... existing code ... 