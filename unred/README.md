# Unred - 防红短链接 SDK

Unred 是 qtoolkit 的一个独立模块，用于调用 Unred 防红短链接服务 API。

## 功能特性

- ✅ **创建短链接**：支持自定义路径和过期时间
- ✅ **删除短链接**：管理已创建的短链接
- ✅ **配置管理**：支持环境变量和配置文件两种方式
- ✅ **单例模式**：全局单例客户端，减少资源消耗
- ✅ **自定义客户端**：支持创建独立客户端实例

## 安装

```bash
go get github.com/wordgate/qtoolkit/unred
```

## 配置方式

### 使用 Viper 配置（推荐）

Unred 模块使用 Viper 进行配置管理。在应用启动时，使用 viper 加载配置：

```go
import "github.com/spf13/viper"

// 配置文件示例 (config.yml)
viper.SetConfigName("config")
viper.SetConfigType("yaml")
viper.AddConfigPath(".")
viper.ReadInConfig()
```

配置文件内容：

```yaml
unred:
  api_endpoint: "api.x.all7.cc"
  secret_key: "your-secret-key"
```

### 使用环境变量覆盖

也可以通过 viper 的环境变量自动替换功能：

```go
viper.AutomaticEnv()
viper.SetEnvPrefix("MYAPP")
```

这样环境变量 `MYAPP_UNRED_API_ENDPOINT` 会覆盖配置文件中的值。

## 使用示例

### 1. 使用全局单例客户端

```go
package main

import (
    "fmt"
    "time"
    "github.com/wordgate/qtoolkit/unred"
)

func main() {
    // 创建短链接
    expireAt := time.Now().Add(30 * 24 * time.Hour).Unix() // 30天后过期

    resp, err := unred.CreateLink("/s/test", "https://example.com", expireAt)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Short URL: %s\n", resp.URL)
    // 输出: Short URL: https://api.x.all7.cc/s/test

    // 删除短链接
    deleteResp, err := unred.DeleteLink("/s/test")
    if err != nil {
        panic(err)
    }

    fmt.Printf("Deleted: %v\n", deleteResp.Success)
}
```

### 2. 使用自定义客户端

```go
package main

import (
    "fmt"
    "time"
    "github.com/wordgate/qtoolkit/unred"
)

func main() {
    // 创建自定义客户端
    client := unred.NewClient("api.x.all7.cc", "your-secret-key")

    // 创建短链接
    expireAt := time.Now().Add(7 * 24 * time.Hour).Unix() // 7天后过期

    resp, err := client.CreateLink("/promo/sale", "https://shop.example.com/sale", expireAt)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Created: %s\n", resp.URL)

    // 删除短链接
    deleteResp, err := client.DeleteLink("/promo/sale")
    if err != nil {
        panic(err)
    }

    fmt.Printf("Deleted: %v\n", deleteResp.Success)
}
```

### 3. 批量创建短链接

```go
package main

import (
    "fmt"
    "time"
    "github.com/wordgate/qtoolkit/unred"
)

func main() {
    client := unred.NewClient("api.x.all7.cc", "your-secret-key")

    links := map[string]string{
        "/product/item1": "https://shop.example.com/item1",
        "/product/item2": "https://shop.example.com/item2",
        "/product/item3": "https://shop.example.com/item3",
    }

    expireAt := time.Now().Add(30 * 24 * time.Hour).Unix()

    for path, targetURL := range links {
        resp, err := client.CreateLink(path, targetURL, expireAt)
        if err != nil {
            fmt.Printf("Failed to create %s: %v\n", path, err)
            continue
        }
        fmt.Printf("✓ Created: %s -> %s\n", path, resp.URL)
    }
}
```

### 4. 不设置过期时间

```go
// expireAt 设置为 0 表示不设置过期时间
resp, err := unred.CreateLink("/permanent", "https://example.com", 0)
```

## API 参考

### CreateLink

创建短链接。

```go
func CreateLink(path string, targetURL string, expireAt int64) (*CreateLinkResponse, error)
```

**参数：**
- `path`：短链接路径，如 `/s/test` 或 `s/test`（会自动添加前缀 `/`）
- `targetURL`：目标 URL
- `expireAt`：过期时间戳（Unix timestamp），0 表示不设置过期时间

**返回：**
- `CreateLinkResponse`：包含短链接信息
  - `Success`：是否成功
  - `Subdomain`：子域名
  - `Path`：路径
  - `URL`：完整的短链接 URL
  - `Message`：错误信息（如果失败）

### DeleteLink

删除短链接。

```go
func DeleteLink(path string) (*DeleteLinkResponse, error)
```

**参数：**
- `path`：短链接路径，如 `/s/test` 或 `s/test`

**返回：**
- `DeleteLinkResponse`：删除结果
  - `Success`：是否成功
  - `Message`：消息

### NewClient

创建自定义客户端。

```go
func NewClient(apiEndpoint, secretKey string) *Client
```

**参数：**
- `apiEndpoint`：API 端点，如 `api.x.all7.cc`
- `secretKey`：管理接口密钥

**返回：**
- `*Client`：客户端实例

## 测试

### 运行单元测试

```bash
cd unred
go test -v
```

### 运行集成测试

需要设置真实的 API 凭证：

```bash
export UNRED_TEST_API_ENDPOINT="api.x.all7.cc"
export UNRED_TEST_SECRET_KEY="your-secret-key"
go test -v
```

## 错误处理

```go
resp, err := unred.CreateLink("/test", "https://example.com", 0)
if err != nil {
    // 处理错误
    fmt.Printf("Error: %v\n", err)
    return
}

if !resp.Success {
    // API 返回失败
    fmt.Printf("API Error: %s\n", resp.Message)
    return
}

// 成功
fmt.Printf("Short URL: %s\n", resp.URL)
```

## 注意事项

1. **API 密钥安全**：不要将 `secret_key` 硬编码在代码中，使用环境变量或配置文件
2. **路径格式**：路径会自动添加 `/` 前缀，`test` 和 `/test` 都可以
3. **过期时间**：使用 Unix 时间戳（秒），设置为 0 表示不过期
4. **HTTP 超时**：默认 30 秒超时，适用于大多数场景
5. **单例客户端**：全局函数 `CreateLink` 和 `DeleteLink` 使用单例客户端，配置只会加载一次

## 许可证

MIT License

## 贡献

欢迎提交 Issue 和 Pull Request！
