# session-kit

[![Go Reference](https://pkg.go.dev/badge/github.com/soulteary/session-kit/v2.svg)](https://pkg.go.dev/github.com/soulteary/session-kit/v2)
[![Go Report Card](.github/goreportcard.svg)](.github/goreportcard-report.md)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![codecov](https://codecov.io/gh/soulteary/session-kit/graph/badge.svg)](https://codecov.io/gh/soulteary/session-kit)

[English](README.md)

Go 语言会话管理库，支持内存和 Redis 存储后端，兼容 Fiber v3 会话中间件。

## 功能特性

- **多存储后端**: 内存（开发用）和 Redis（生产用）
- **Fiber v3 兼容**：将存储后端适配为 Fiber v3 支持上下文的 `fiber.Storage` 接口
- **工厂模式**: 通过配置轻松创建存储
- **会话管理**: 使用 SessionData 结构体进行高级会话操作
- **链式配置**: 使用构建器模式进行简单配置
- **自动过期**: 基于 TTL 的会话过期
- **线程安全**: 支持并发访问

## 环境要求

- **Go 1.27+**（`go.mod` 声明 `go 1.27.0`）
- Fiber 辅助函数需要 `github.com/gofiber/fiber/v3` v3.4.0+
- Redis 存储需要 `github.com/redis/go-redis/v9`

v2 模块线面向 Fiber v3。仍在 Fiber v2 上的应用请继续使用
`github.com/soulteary/session-kit` v1。

## 安装

```bash
go get github.com/soulteary/session-kit/v2
```

Fiber 集成要求 Fiber v3.4.0 或更高版本。仍使用 Fiber v2 的应用应继续使用 `github.com/soulteary/session-kit` v1。

## 快速开始

### 内存存储（开发环境）

```go
package main

import (
    "time"
    session "github.com/soulteary/session-kit/v2"
)

func main() {
    // 创建内存存储
    storage := session.NewMemoryStorage("session:", 10*time.Minute)
    defer storage.Close()

    // 使用存储
    storage.Set("session-123", []byte("data"), time.Hour)
    data, _ := storage.Get("session-123")
}
```

### Redis 存储（生产环境）

```go
package main

import (
    session "github.com/soulteary/session-kit/v2"
)

func main() {
    // 创建 Redis 存储
    storage, err := session.NewRedisStorageFromConfig(
        "localhost:6379",  // 地址
        "",                // 密码
        0,                 // 数据库
        "myapp:session:",  // 键前缀
    )
    if err != nil {
        panic(err)
    }
    defer storage.Close()
}
```

### 使用工厂模式

```go
package main

import (
    session "github.com/soulteary/session-kit/v2"
)

func main() {
    // 从配置创建存储
    cfg := session.DefaultStorageConfig().
        WithType(session.StorageTypeRedis).
        WithRedisAddr("localhost:6379").
        WithKeyPrefix("myapp:session:")

    storage, err := session.NewStorage(cfg)
    if err != nil {
        panic(err)
    }
    defer storage.Close()
}
```

### 与 Fiber 配合使用

```go
package main

import (
    "time"

    "github.com/gofiber/fiber/v3"
    fibersession "github.com/gofiber/fiber/v3/middleware/session"
    session "github.com/soulteary/session-kit/v2"
)

func main() {
    app := fiber.New()

    // 创建会话管理器
    storage := session.NewMemoryStorage("session:", 0)
    config := session.DefaultConfig().
        WithCookieName("my_session").
        WithExpiration(24 * time.Hour)

    manager := session.NewManager(storage, config)

    // 创建 Fiber 会话存储
    store := fibersession.NewStore(manager.FiberSessionConfig())

    app.Get("/", func(c fiber.Ctx) error {
        sess, _ := store.Get(c)

        if session.IsAuthenticated(sess) {
            return c.SendString("你好, " + session.GetUserID(sess))
        }

        return c.SendString("请登录")
    })

    app.Post("/login", func(c fiber.Ctx) error {
        sess, _ := store.Get(c)

        session.SetUserID(sess, "user-123")
        session.SetEmail(sess, "user@example.com")
        session.AddAMR(sess, "pwd")
        session.Authenticate(sess)

        return c.SendString("登录成功")
    })

    app.Listen(":3000")
}
```

## 配置

### 会话配置

```go
cfg := session.DefaultConfig().
    WithExpiration(24 * time.Hour).   // 会话持续时间
    WithCookieName("my_session").     // Cookie 名称
    WithCookieDomain(".example.com"). // Cookie 域
    WithCookiePath("/").              // Cookie 路径
    WithSecure(true).                 // 仅 HTTPS
    WithHTTPOnly(true).               // 禁止 JS 访问
    WithSameSite("Lax").              // SameSite 策略
    WithKeyPrefix("myapp:session:")   // 存储键前缀
```

### 安全建议

- **请从 `DefaultConfig()` 开始**，而不是 `session.Config{}`。零值不会通过校验：它会
  产出一个无名称的 Cookie，且 `Secure=false`、`HTTPOnly=false`；一个会批准"可用配置中
  最不安全的那个"的校验器只会带来虚假的安心感。
- **配置校验**：使用前调用 `cfg.Validate()`，避免无效或不安全的组合（例如
  `SameSite=None` 但未开启 `Secure=true`）。
- **SameSite 行为**：支持 `Strict`、`Lax`、`None`、`Disabled`。只有在必须跨站请求时使用 `None`，且务必启用 `Secure=true`。
- **登录加固**：认证成功后应轮换（重新生成）会话 ID，以防止会话固定攻击。
- **Redis 加固**：将 Redis 视为可信后端，使用网络隔离与访问控制，并在客户端设置超时避免资源耗尽。

### 存储配置

```go
cfg := session.DefaultStorageConfig().
    WithType(session.StorageTypeRedis).      // memory 或 redis
    WithKeyPrefix("session:").               // 键前缀
    WithRedisAddr("localhost:6379").         // Redis 地址
    WithRedisPassword("secret").             // Redis 密码
    WithRedisDB(0).                          // Redis 数据库
    WithMemoryGCInterval(10 * time.Minute)   // 内存 GC 间隔
```

## 会话数据

`SessionData` 结构体提供了丰富的会话数据模型：

```go
session := session.NewSessionData("session-123", time.Hour)

// 用户信息
session.UserID = "user-456"
session.Email = "user@example.com"
session.Phone = "+1234567890"
session.Authenticated = true

// 认证方法 (AMR)
session.AddAMR("pwd")     // 密码
session.AddAMR("otp")     // OTP
session.HasAMR("pwd")     // 检查是否有该方法

// 授权范围
session.AddScope("read")
session.AddScope("write")
session.HasScope("read")  // 检查是否有该范围

// 自定义数据
session.SetValue("custom", "value")
val, ok := session.GetValue("custom")

// 状态检查
session.IsExpired()
session.IsAuthenticated()
session.Touch()  // 更新最后访问时间
```

## 会话生命周期

### 保存不会续期

`SaveSession` 会**拒绝一个已经过期的会话**。此前写入一个过期会话会给它再来一整个生命
周期，于是一个持续被写入的会话永远不会过期。

`TouchSession` 才是有意延长会话的方式：

```go
if err := manager.TouchSession(ctx, sessionID); err != nil {
    // 会话已不存在或已过期，请重新认证
}
```

### 登出

`Unauthenticate` 会清除身份相关的键、**把清除后的状态保存下来**，然后才销毁会话。
两种失败都会上报：

```go
if err := session.Unauthenticate(sess); err != nil {
    // 即便 Destroy 失败，该会话也已不再处于已认证状态 ——
    // 清除后的状态是先被持久化的。
    log.Printf("登出: %v", err)
}
```

这个清除此前只存在于内存中，于是 `Destroy` 失败时，存储里的会话毫发无损、仍然处于完全
认证状态——而这恰恰是那道保险要覆盖的情形。

### Scope 与 AMR 能在往返后存活

Fiber v3 用 msgpack 序列化会话数据，而它把数组解码回 `[]interface{}` 而不是
`[]string`。`GetScopes`、`GetAMR`、`HasScope` 和 `HasAMR` 两种表示都接受，因此在某个
请求里设置的 scope 在下一个请求里仍然可见：

```go
session.SetScopes(sess, []string{"read", "write"})
// 在后续请求中：
session.HasScope(sess, "read") // true
```

## Fiber 会话辅助函数

用于操作 Fiber 会话的辅助函数：

```go
// 认证
session.Authenticate(sess)      // 标记为已认证
session.Unauthenticate(sess)    // 销毁会话
session.IsAuthenticated(sess)   // 检查是否已认证

// 用户信息
session.SetUserID(sess, "user-123")
session.GetUserID(sess)

session.SetEmail(sess, "user@example.com")
session.GetEmail(sess)

session.SetPhone(sess, "+1234567890")
session.GetPhone(sess)

// AMR（认证方法引用）
session.SetAMR(sess, []string{"pwd", "otp"})
session.GetAMR(sess)
session.AddAMR(sess, "pwd")
session.HasAMR(sess, "pwd")

// 范围
session.SetScopes(sess, []string{"read", "write"})
session.GetScopes(sess)
session.HasScope(sess, "read")

// 时间戳
session.UpdateLastAccess(sess)
session.GetLastAccess(sess)
session.GetCreatedAt(sess)
```

### 会话共享（跨域）

若需将会话 ID 共享给其他域（如子域或合作方应用），可使用 `session.CreateCookie(config, sessionID)` 生成 Cookie 并写入响应。当 `SameSite` 为 `None` 时，库会强制将 Cookie 设为 `Secure` 以满足浏览器要求。

## 服务端 KV 会话（Store）

需要通用键值会话存储的服务（如服务端 challenge/会话记录）可使用 `Store` 接口与 `KVManager`：

- **Store**：提供带 TTL 的 `Create`、`Get`、`Set`、`Delete`、`Exists`。
- **KVManager**：封装 `Store`，提供默认 TTL 及 `Refresh`。
- **RedisStore**：Redis 实现，使用 `NewRedisStore(client, keyPrefix)`。

```go
// redisClient 为 *redis.Client；需引入 "context" 与 "time"。
ctx := context.Background()
store := session.NewRedisStore(redisClient, "myapp:session:")
mgr := session.NewKVManager(store, 10*time.Minute)

id, _ := mgr.Create(ctx, map[string]interface{}{"user_id": "u1"}, 0)
rec, _ := mgr.Get(ctx, id)
_ = mgr.Set(ctx, id, map[string]interface{}{"user_id": "u1", "step": 2}, 0)
_ = mgr.Refresh(ctx, id, 0)
_ = mgr.Delete(ctx, id)
```

## 工厂方法

- **NewStorageFromEnv(redisEnabled, redisAddr, redisPassword, redisDB, keyPrefix)** — 按“是否启用 Redis + 连接参数”创建 Storage（`redisEnabled` 为 false 时使用内存）。
- **MustNewStorage(cfg)** — 与 `NewStorage(cfg)` 相同，但出错时 panic，适用于 `main()` 初始化。

## 升级说明（v2.3.0）

仅升级依赖。没有删除任何 API，调用方无需改代码。

- 测试用 Redis 为 `miniredis` v2.39.0（此前 v2.36.1）。
- 仍依赖 `redis-kit` v1.6.0。该模块没有更新的已发布版本。

## 升级说明（v2.2.0）

没有新增或删除任何 API。有四处行为变化，以及一种此前能通过校验的配置现在不能了。

- **`session.Config{}` 不再通过校验。** 零值此前被当作"还没配置"而被接受，于是它能通过
  `Validate()`，并产出一个无名称的 Cookie、`Secure=false`、`HTTPOnly=false`。
  **请从 `DefaultConfig()` 开始**——如果你是用字面量构造 `Config` 并依赖 `Validate()`
  返回 nil，现在它会报告 Cookie 名称为空。
- **Scope 与 AMR 能在存储往返后存活。** `GetScopes` 和 `GetAMR` 此前断言
  `val.([]string)`，而这只在写入该值的那个请求内成立：msgpack 把数组解码回
  `[]interface{}`，于是**在之后的每个请求里它们都静默返回空，`HasScope`/`HasAMR`
  永远报告 false**。如果你为此做过绕行——每个请求重新推导 scope，或者把 `HasScope` 当成
  不可靠的——现在可以不用了。
- **`SaveSession` 拒绝已过期的会话。** 写入过期会话此前会给它再来一整个生命周期，于是
  持续被写入的会话永不过期。请用 `TouchSession` 来有意延长。**此前会成功的保存现在会
  返回错误**，而这才是正确答案。
- **登出会先持久化再销毁。** `Unauthenticate` 此前"为防 Destroy 失败"清除身份键却从不
  保存，于是 `Destroy` 失败时存储中的会话仍然处于完全认证状态。现在清除后的状态会先被
  保存，而且 `Save` 失败会和 `Destroy` 失败一起上报，不再被丢弃。
- **`MemoryStorage.Get` 返回副本。** 它此前写入时拷贝、读取时却返回内部切片，于是调用方
  修改读到的内容会破坏存储中的会话（对之后所有读者可见），并与并发写入竞争。
- **`MemoryStorage.Set` 传空值会删除该键。** 它此前忽略这次写入，把旧负载留在原处——
  于是用空数据覆盖一个会话，旧的、仍然已认证的字节依然可读。
- **`MemoryStorage.Close` 可重复调用。** 第二次调用此前会 panic。

## 测试

```bash
go test ./...
go test -coverprofile=coverage.out -covermode=atomic ./...
go tool cover -html=coverage.out
```

## 许可证

Apache License 2.0 —— 详见 [LICENSE](LICENSE)。
