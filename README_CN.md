# session-kit

[![Go Reference](https://pkg.go.dev/badge/github.com/soulteary/session-kit/v3.svg)](https://pkg.go.dev/github.com/soulteary/session-kit/v3)
[![Go Report Card](.github/goreportcard.svg)](.github/goreportcard-report.md)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![codecov](https://codecov.io/gh/soulteary/session-kit/graph/badge.svg)](https://codecov.io/gh/soulteary/session-kit)

[English](README.md)

Go 语言会话管理库：存储后端接口与内存实现、服务端会话记录的管理器、通用
键值会话存储，以及一组不绑定任何 Web 框架的 Cookie 与会话辅助函数。

## 包结构

**根包只依赖标准库。** 任何需要第三方模块的后端或适配层都放在子包里，
因此导入根包不会链接进用不到的客户端或框架：

| 包 | 引入的依赖 | 提供的能力 |
|---|---|---|
| `github.com/soulteary/session-kit/v3` | *（仅标准库）* | `Storage`、`MemoryStorage`、`Manager`、`SessionData`、`Store`、`KVManager`、`Config`、`CreateCookie` 及会话辅助函数 |
| `.../v3/fiberadapter` | Fiber v3、fasthttp | `SessionConfig`、`Storage`、`Cookie`、`SameSite` |
| `.../v3/redisstore` | go-redis、redis-kit | `Storage`、`New`、`NewFromConfig`、`Store`、`NewStore`、`Client` |

使用内存会话的 net/http 服务，不会为这两个子包的存在付出任何代价。以只导入
根包的程序实测，相比 v2.3.0：链接的包少了 97 个、模块少了 20 个、二进制
体积小了 24.6%，使用方自己的 `go.mod` 里**一条** `// indirect` 依赖都不剩。
完整数据见 [CHANGELOG.md](CHANGELOG.md)。

## 功能特性

- **与框架无关的内核**：会话辅助函数接收小接口，因此任何带有
  `Get`/`Set`/`Delete`/`Save`/`Destroy` 的会话类型都能用——Fiber v3 的会话
  无需任何改动即可直接传入
- **多存储后端**：内存内置，Redis 在 `redisstore`，还可用 `RegisterStorage`
  注册自定义后端
- **Fiber v3 支持**：`fiberadapter` 负责接线中间件并适配 `fiber.Storage`
- **会话管理**：基于 `SessionData` 记录的高级操作
- **链式配置**：全程构建器模式
- **自动过期**：基于 TTL，写入与读取时都会校验
- **线程安全**：支持并发访问

## 环境要求

- **Go 1.27+**（`go.mod` 声明 `go 1.27.0`）
- `github.com/gofiber/fiber/v3` v3.5.0+ —— 仅 `fiberadapter` 需要
- `github.com/redis/go-redis/v9` —— 仅 `redisstore` 需要

v3 模块线面向 Fiber v3。仍在 Fiber v2 上的应用请继续使用
`github.com/soulteary/session-kit` v1。

## 安装

```bash
go get github.com/soulteary/session-kit/v3
```

从 v2 升级？见[升级说明（v3.0.0）](#升级说明v300)——导入路径对所有人都会变，
但绝大多数调用点不用改。

## 快速开始

### 内存存储（开发环境）

```go
package main

import (
    "time"

    session "github.com/soulteary/session-kit/v3"
)

func main() {
    storage := session.NewMemoryStorage("session:", 10*time.Minute)
    defer storage.Close()

    manager := session.NewManager(storage, session.DefaultConfig())

    record := manager.CreateSession("session-123")
    record.UserID = "user-456"
    record.Authenticated = true
    _ = manager.SaveSession(record)

    loaded, _ := manager.LoadSession("session-123")
    _ = loaded
}
```

### Redis 存储（生产环境）

Redis 位于 `redisstore`，这一行导入正是把 go-redis 链接进来的原因：

```go
package main

import (
    "github.com/redis/go-redis/v9"

    session "github.com/soulteary/session-kit/v3"
    "github.com/soulteary/session-kit/v3/redisstore"
)

func main() {
    client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})

    storage := redisstore.New(client, "myapp:session:")
    defer storage.Close()

    manager := session.NewManager(storage, session.DefaultConfig())
    _ = manager
}
```

`redisstore.New` 接收 `redisstore.Client`，也就是该存储实际用到的那几条命令，
因此 `*redis.Client`、`*redis.ClusterClient`、`*redis.Ring` 和
`redis.UniversalClient` 都能直接传入。Cluster 或 Sentinel 部署不再需要单独
写一份存储实现。

如果希望由存储自己建立连接（并顺带做一次连通性检查）：

```go
storage, err := redisstore.NewFromConfig("localhost:6379", "", 0, "myapp:session:")
```

### 由配置决定后端

`NewStorage` 自己就能创建内存存储。其他后端必须先注册，对 Redis 来说就是
以副作用方式导入子包：

```go
import (
    session "github.com/soulteary/session-kit/v3"
    _ "github.com/soulteary/session-kit/v3/redisstore"
)

cfg := session.DefaultStorageConfig().
    WithType(session.StorageTypeRedis).
    WithRedisAddr("localhost:6379").
    WithKeyPrefix("myapp:session:")

storage, err := session.NewStorage(cfg)
```

漏掉这行导入时，`NewStorage` 会直接点名告诉你，而不是报一个“未知类型”。
`RegisterStorage` 是导出的，Memcached、DynamoDB 或自研后端都能接到同一个
工厂上。

### 与 Fiber 配合使用

```go
package main

import (
    "time"

    "github.com/gofiber/fiber/v3"
    fibersession "github.com/gofiber/fiber/v3/middleware/session"

    session "github.com/soulteary/session-kit/v3"
    "github.com/soulteary/session-kit/v3/fiberadapter"
)

func main() {
    app := fiber.New()

    storage := session.NewMemoryStorage("session:", 0)
    config := session.DefaultConfig().
        WithCookieName("my_session").
        WithExpiration(24 * time.Hour)

    manager := session.NewManager(storage, config)

    store := fibersession.NewStore(fiberadapter.SessionConfig(manager))

    app.Get("/", func(c fiber.Ctx) error {
        sess, _ := store.Get(c)

        if session.IsAuthenticated(sess) {
            return c.SendString("Hello, " + session.GetUserID(sess))
        }

        return c.SendString("Please login")
    })

    app.Post("/login", func(c fiber.Ctx) error {
        sess, _ := store.Get(c)

        session.SetUserID(sess, "user-123")
        session.SetEmail(sess, "user@example.com")
        session.AddAMR(sess, "pwd")

        // 先轮换会话 ID，再标记为已认证。
        return session.Authenticate(sess)
    })

    app.Listen(":3000")
}
```

注意这些辅助函数来自**根包**，而不是 `fiberadapter`。这正是下面这组接口的意义。

## 会话辅助函数及其接口

每个辅助函数只接收它真正需要的最小接口，而不是某个具体的会话类型：

| 接口 | 方法 | 使用者 |
|---|---|---|
| `Reader` | `Get` | 全部 getter、`IsAuthenticated`、`HasAMR`、`HasScope` |
| `Writer` | `Set` | 全部 setter、`UpdateLastAccess` |
| `ReadWriter` | `Get`、`Set` | `AddAMR` |
| `Saver` | `Set`、`Save` | `Authenticate` |
| `Session` | `Get`、`Set`、`Delete`、`Save`、`Destroy` | `Unauthenticate` |
| `Regenerator` | `Regenerate` | `Authenticate`（会话支持时） |

`*github.com/gofiber/fiber/v3/middleware/session.Session` 原样就满足这六个
接口，所以 Fiber 代码直接把手上的会话传进去即可——这也正是根包不必导入
Fiber 的原因。任何具备同样五个方法的类型同样可用：

```go
type mySession struct{ values map[any]any }

func (s *mySession) Get(key any) any  { return s.values[key] }
func (s *mySession) Set(key, val any) { s.values[key] = val }
func (s *mySession) Delete(key any)   { delete(s.values, key) }
func (s *mySession) Save() error      { return nil }
func (s *mySession) Destroy() error   { clear(s.values); return nil }
```

`Regenerator` 是其中唯一可选的接口。只要会话带有 `Regenerate() error` 方法，
`Authenticate` 就会通过它轮换会话 ID——这正是让"登录前被植入的会话 ID 无法在登录后
继续有效"（会话固定攻击）的关键。之所以把它做成一项可选能力、而不是写进 `Saver`，
一是 `Saver` 属于已发布的 v3 API，二是像上面 `mySession` 这样的会话并不向客户端
交付自己的 ID：既没有可轮换的对象，攻击者也无从提前植入。**如果你的会话类型确实
会把 ID 交给客户端，请实现 `Regenerate`**——否则 `Authenticate` 无从轮换，调用方
带来的那个 ID 会原样跨过登录继续有效。

```go
// 认证
session.Authenticate(sess)      // 轮换 ID（支持时）、标记已认证并保存
session.Unauthenticate(sess)    // 清除身份信息并销毁
session.IsAuthenticated(sess)   // 检查是否已认证

// 用户信息
session.SetUserID(sess, "user-123")
session.GetUserID(sess)

session.SetEmail(sess, "user@example.com")
session.GetEmail(sess)

session.SetPhone(sess, "+1234567890")
session.GetPhone(sess)

// AMR（认证方式引用）
session.SetAMR(sess, []string{"pwd", "otp"})
session.GetAMR(sess)
session.AddAMR(sess, "pwd")
session.HasAMR(sess, "pwd")

// 权限范围
session.SetScopes(sess, []string{"read", "write"})
session.GetScopes(sess)
session.HasScope(sess, "read")

// 时间戳
session.UpdateLastAccess(sess)
session.GetLastAccess(sess)
session.GetCreatedAt(sess)
```

## 配置

### 会话配置

```go
cfg := session.DefaultConfig().
    WithExpiration(24 * time.Hour).   // 会话时长
    WithCookieName("my_session").     // Cookie 名称
    WithCookieDomain(".example.com"). // Cookie 域名
    WithCookiePath("/").              // Cookie 路径
    WithSecure(true).                 // 仅 HTTPS
    WithHTTPOnly(true).               // 禁止 JS 访问
    WithSameSite("Lax").              // SameSite 策略
    WithKeyPrefix("myapp:session:")   // 存储键前缀
```

`cfg.SameSiteMode()` 以 `http.SameSite` 返回 SameSite 属性，`cfg.CookieSecure()`
则报告这个 Cookie 是否必须带 `Secure`。各适配层读这两个方法，而不是自己解释
`Config.SameSite`：一条在不同框架之间互相打架的 SameSite 规则是 CSRF 漏洞，
不是无伤大雅的细节差异。

### 安全建议

- **从 `DefaultConfig()` 开始**，不要用 `session.Config{}`。零值通不过校验：
  它会产出一个没有名字、`Secure=false` 且 `HTTPOnly=false` 的 Cookie；
  一个会给最不安全的配置放行的校验器，只会带来虚假的安全感。
- **校验配置**：使用前调用 `cfg.Validate()`，以捕获不安全或非法的组合
  （例如 `SameSite=None` 却没有 `Secure=true`）。
- **SameSite 行为**：支持 `Strict`、`Lax`、`None`、`Disabled`。只有确实需要
  跨站请求时才用 `None`，并且务必配合 `Secure=true`——无论如何
  `CookieSecure()` 都会强制打开它，因为没有 `Secure` 的 `SameSite=None`
  Cookie 会被浏览器直接丢弃，而不是放宽处理。
- **登录加固**：`Authenticate()` 会自行轮换会话 ID，因此攻击者在登录前植入受害者
  浏览器的会话 ID 无法在登录后继续有效（会话固定攻击）。轮换发生在写入认证标记之前，
  且轮换失败会作为错误返回——无法完成轮换的会话会被刻意保持为未认证状态，请检查该
  错误。不暴露 ID 的会话则不受影响；若你的会话类型会暴露 ID，参见上文 `Regenerator`。
- **Redis 加固**：把 Redis 当作受信后端——做好网络隔离与凭据管理，并在客户端
  层面设置超时，防止资源耗尽。

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

这里没有 `WithRedisClient`：配置描述的是一条该由工厂去建立的连接。手上已经
有客户端的代码，直接调用 `redisstore.New(client, prefix)` 即可，它返回的
`session.Storage` 可以原样交给 `NewManager`。

## 会话数据

`SessionData` 结构体提供了完整的会话数据模型：

```go
record := session.NewSessionData("session-123", time.Hour)

// 用户信息
record.UserID = "user-456"
record.Email = "user@example.com"
record.Phone = "+1234567890"
record.Authenticated = true

// 认证方式（AMR）
record.AddAMR("pwd")     // 密码
record.AddAMR("otp")     // 一次性验证码
record.HasAMR("pwd")     // 检查是否包含该方式

// 授权范围
record.AddScope("read")
record.AddScope("write")
record.HasScope("read")  // 检查是否包含该范围

// 自定义数据
record.SetValue("custom", "value")
val, ok := record.GetValue("custom")

// 状态检查
record.IsExpired()
record.IsAuthenticated()
record.Touch()  // 更新最后访问时间
```

## 会话生命周期

### 保存不会续期

`SaveSession` **拒绝写入已过期的会话**。以前写入会再给它一个完整的生命周期，
于是一个被反复写入的会话永远不会过期。

`TouchSession` 才是有意延长会话的方式。它接收的是记录本身，不是 ID：

```go
if err := manager.TouchSession(record); err != nil {
    // 会话已不存在或已过期，需要重新认证
}
```

### 登出

`Unauthenticate` 先清除身份相关的键、销毁会话，只有在销毁失败时才把清除后的
状态落盘——这样即便 `Destroy` 失败，留下的也是一个已退出登录的会话，而不是
一个仍然完整认证的会话：

```go
if err := session.Unauthenticate(sess); err != nil {
    // 即使 Destroy 失败，该会话也已不再是已认证状态——
    // 清除后的状态已作为兜底被持久化。
    log.Printf("logout: %v", err)
}
```

以前清除只发生在内存里，因此 `Destroy` 失败时存储中的会话原封不动、仍然完整
认证——正是这道保险本该覆盖的情况。

`nil` 会话会被正确处理，包括**带类型的** nil，比如一个未赋值的
`*fibersession.Session` 字段。参数是接口类型，普通的 `== nil` 判断会漏掉它，
并在下一次写入时 panic。

### Scope 与 AMR 能在往返后存活

Fiber v3 用 msgpack 序列化会话数据，数组会被解码成 `[]interface{}` 而不是
`[]string`。`GetScopes`、`GetAMR`、`HasScope`、`HasAMR` 两种表示都接受，因此
在某次请求里设置的 scope，在下一次请求中依然可见：

```go
session.SetScopes(sess, []string{"read", "write"})
// 在后续请求中：
session.HasScope(sess, "read") // true
```

### 会话共享（跨域）

要把会话 ID 共享给另一个域名（例如子域名或合作方应用），用
`session.CreateCookie(config, sessionID)` 构造 Cookie 并写入响应。它返回
`*net/http.Cookie`，可以直接交给 `http.SetCookie`：

```go
http.SetCookie(w, session.CreateCookie(config, sessionID))
```

Fiber 用户需要的是 `*fiber.Cookie`：

```go
c.Cookie(fiberadapter.Cookie(config, sessionID))
```

两者由同一套规则构造，其中包括 `SameSite=None` 强制 `Secure`。

## 服务端 KV 会话（Store）

如果服务需要一个通用的键值会话存储（例如服务端的挑战/会话记录），使用
`Store` 接口与 `KVManager`：

- **Store**：带 TTL 的 `Create`、`Get`、`Set`、`Delete`、`Exists`。
- **KVManager**：包装 `Store`，提供默认 TTL 以及 `Refresh`。
- **redisstore.Store**：`Store` 的 Redis 实现，使用 `redisstore.NewStore(client, keyPrefix)`。

```go
// redisClient 是 redisstore.Client；需要导入 "context" 和 "time"。
ctx := context.Background()
store := redisstore.NewStore(redisClient, "myapp:session:")
mgr := session.NewKVManager(store, 10*time.Minute)

id, _ := mgr.Create(ctx, map[string]interface{}{"user_id": "u1"}, 0)
rec, _ := mgr.Get(ctx, id)
_ = mgr.Set(ctx, id, map[string]interface{}{"user_id": "u1", "step": 2}, 0)
_ = mgr.Refresh(ctx, id, 0)
_ = mgr.Delete(ctx, id)
```

## 工厂方法

- **NewStorageFromEnv(redisEnabled, redisAddr, redisPassword, redisDB, keyPrefix)** —— 按环境变量式的开关创建 Storage（`redisEnabled` 为 false 时用内存）。Redis 需要导入 `redisstore`。
- **MustNewStorage(cfg)** —— 与 `NewStorage(cfg)` 相同，但出错时 panic（适合 `main()`）。
- **RegisterStorage(type, builder)** —— 让 `NewStorage` 认识本模块并未内置的后端。

## 升级说明（v3.0.0）

导入路径对所有人都会变，但绝大多数调用点不用改。

**未改变：** 全部会话辅助函数（`Authenticate`、`IsAuthenticated`、
`SetUserID`、`HasScope` 等）。它们现在接收接口，而
`*fibersession.Session` 原样即满足，因此调用点与之前完全一致地通过编译。
`Manager`、`SessionData`、`Storage`、`MemoryStorage`、`Store`、`KVManager`
和 `Config` 同样未变。

**已改变：**

| 之前 | 之后 |
|---|---|
| `github.com/soulteary/session-kit/v2` | `github.com/soulteary/session-kit/v3` |
| `manager.FiberSessionConfig()` | `fiberadapter.SessionConfig(manager)` |
| `session.NewRedisStorage(client, prefix)` | `redisstore.New(client, prefix)` |
| `session.NewRedisStorageFromConfig(...)` | `redisstore.NewFromConfig(...)` |
| `session.NewRedisStore(client, prefix)` | `redisstore.NewStore(client, prefix)` |
| `session.RedisStorage` / `session.RedisStore` | `redisstore.Storage` / `redisstore.Store` |
| `storage.GetClient()` / `GetKeyPrefix()` / `GetTTL(k)` | `storage.Client()` / `KeyPrefix()` / `TTL(k)` |
| `cfg.WithRedisClient(client)` | `redisstore.New(client, prefix)` |
| `CreateCookie` 返回 `*fiber.Cookie` | 返回 `*http.Cookie`；`fiberadapter.Cookie` 返回 `*fiber.Cookie` |
| `session.NewStorage(cfg)` 且 `Type: redis` | 不变，另加 `import _ ".../v3/redisstore"` |

这里刻意没有提供兼容 shim：根包里的 shim 必须导入它所转发的那个包，这会把
Fiber 或 go-redis 重新链接回来，拆分的收益也就全部还回去了。

## 升级说明（v2.3.0）

仅依赖刷新。没有移除任何 API，也不需要改写任何调用。

- 测试用 Redis 为 `miniredis` v2.39.0（原 v2.36.1）。
- 仍需要 `redis-kit` v1.6.0，该模块没有更新的发布版本。

## 升级说明（v2.2.0）

没有新增或移除 API。有四处行为变化，以及一种此前能通过校验的配置不再通过。

- **`session.Config{}` 不再通过校验。** 零值曾被当作“尚未配置”接受，因此能通过
  `Validate()`，并产出一个没有名字、`Secure=false` 且 `HTTPOnly=false` 的
  Cookie。**请从 `DefaultConfig()` 开始**——如果你用字面量构造 `Config` 并依赖
  `Validate()` 返回 nil，它现在会报告 Cookie 名称为空。
- **Scope 与 AMR 能在存储往返后存活。** `GetScopes` 与 `GetAMR` 断言
  `val.([]string)`，而这只在写入值的那次请求内成立：msgpack 会把数组解码回
  `[]interface{}`，于是**在之后的每次请求里它们都静默返回空，
  `HasScope`/`HasAMR` 永远为 false**。如果你为此做过绕行——每次请求重新推导
  scope，或把 `HasScope` 当作不可靠——现在可以停了。
- **`SaveSession` 拒绝已过期的会话。** 写入它会再给一个完整生命周期，于是被
  反复写入的会话永远不会过期。请用 `TouchSession` 有意地延长。**以前会成功的
  保存现在返回错误**，这才是正确答案。
- **登出会先销毁再兜底落盘。** `Unauthenticate` 曾“以防 Destroy 失败”清除身份
  键却从不保存，于是 `Destroy` 失败时存储中的会话仍然完整认证。现在清除后的
  状态会作为兜底被持久化，且 `Save` 失败会与 `Destroy` 失败一并报告，而不是
  被丢弃。
- **`MemoryStorage.Get` 返回副本。** 它写入时复制，读取时却返回内部切片，于是
  一个调用方修改读到的数据就会破坏之后所有读者看到的会话，并与并发写入竞争。
- **`MemoryStorage.Set` 写入空值等于删除该键。** 它曾忽略这次写入，把原有负载
  留在原地——于是用空数据覆盖会话后，旧的、仍处于已认证状态的字节依然可读。
- **`MemoryStorage.Close` 可重复调用。** 第二次调用曾会 panic。

## 测试

```bash
go test ./...
go test -coverprofile=coverage.out -covermode=atomic ./...
go tool cover -html=coverage.out
```

## 许可证

Apache License 2.0 —— 详见 [LICENSE](LICENSE)。
