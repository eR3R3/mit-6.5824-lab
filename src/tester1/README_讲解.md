# Tester框架代码讲解

## 一、整体架构

这个tester框架是MIT 6.5840 (原6.824)分布式系统课程的测试框架，用于测试Raft、KV服务器等分布式系统实现。

### 核心组件关系图：

```
Config (配置中心)
  ├── Groups (服务器组管理)
  │     └── ServerGrp (单个服务器组)
  │           └── Server[] (服务器实例)
  │
  ├── Clnts (客户端管理)
  │     └── Clnt (单个客户端)
  │
  ├── Network (labrpc网络层)
  │
  └── Annotation (可视化注解系统)
```

---

## 二、各模块详细讲解

### 1. **config.go** - 测试配置核心

**主要功能：**
- `Config` 结构体是整个测试的配置中心
- 管理所有服务器组、客户端和网络环境

**关键方法：**

```go
MakeConfig(t, n, reliable, mks) *Config
```
- 创建测试配置
- `n`: 服务器数量
- `reliable`: 网络是否可靠
- `mks`: 服务器启动函数（用户提供）

```go
Begin(description) / End()
```
- 标记测试开始/结束
- 统计RPC数量、操作数量、执行时间

```go
Fatalf(format, args...)
```
- 打印错误并终止测试
- 会打印调用栈（仅显示test.go文件）

**设计亮点：**
- 使用 `sync.Once` 确保随机种子只初始化一次
- 设置 `GOMAXPROCS(4)` 强制并发测试
- 120秒超时限制

---

### 2. **group.go** - 服务器组管理

**核心概念：**
- `Tgid`: 组ID（Group ID），用于区分不同的服务器组
- `ServerGrp`: 一个服务器组，包含多个服务器实例
- `Groups`: 管理多个服务器组的容器

**关键数据结构：**

```go
type ServerGrp struct {
    net         *labrpc.Network
    srvs        []*Server        // 服务器列表
    servernames []string         // 服务器名称
    gid         Tgid             // 组ID
    connected   []bool           // 每个服务器的连接状态
    mks         FstartServer     // 服务器启动函数
}
```

**重要方法：**

```go
StartServers()
```
- 启动组内所有服务器
- 调用用户的 `mks` 函数创建服务实例
- 将所有服务器连接到网络

```go
ShutdownServer(i) / Shutdown()
```
- 关闭单个/所有服务器
- 断开网络连接
- 调用服务的 `Kill()` 方法

```go
Partition(p1, p2)
```
- 将服务器分成两个分区
- `p1` 和 `p2` 内的服务器可以互相通信
- 但两个分区之间不能通信

```go
connect(i, to) / disconnect(i, from)
```
- 底层连接/断开机制
- 通过 `labrpc.Network.Enable()` 控制连接

**设计亮点：**
- 使用 `connected[]bool` 跟踪连接状态
- 支持动态添加/删除服务器组
- 支持网络分区测试

---

### 3. **srv.go** - 服务器实例

**核心结构：**

```go
type Server struct {
    mu       sync.Mutex
    net      *labrpc.Network
    saved    *Persister        // 持久化存储
    svcs     []IService        // 服务列表（如Raft、KV）
    endNames []string          // 到其他服务器的连接名称
    clntEnds []*labrpc.ClientEnd // 到其他服务器的客户端端点
}
```

**关键方法：**

```go
makeServer(net, gid, nsrv) *Server
```
- 创建服务器实例
- 为每个服务器创建到其他服务器的 `ClientEnd`
- 通过 `ServerName(gid, i)` 生成唯一名称

```go
startServer(gid) *Server
```
- 启动服务器（重新启动时会调用）
- **重要**：创建新的 `Persister`，但会复制旧的状态
- 这确保重启后能恢复之前的状态

```go
shutdownServer()
```
- 关闭服务器
- 调用所有服务的 `Kill()` 方法
- 保存持久化状态的副本

**设计亮点：**
- 每次重启创建新的 `Persister`，避免旧实例覆盖新实例
- 通过 `endNames` 和 `clntEnds` 管理服务器间连接
- `mu` 保护并发访问

---

### 4. **clnts.go** - 客户端管理

**核心结构：**

```go
type Clnt struct {
    mu   sync.Mutex
    net  *labrpc.Network
    ends map[string]end        // 服务器名称 -> 连接端点
    srvs []string              // 允许连接的服务器列表（nil=全部）
}
```

**关键方法：**

```go
Call(server, method, args, reply) bool
```
- 发送RPC调用
- 懒加载：第一次调用某服务器时才创建连接
- 返回成功/失败

```go
makeEnd(server) end
```
- 创建到指定服务器的连接
- 检查 `srvs` 列表决定是否允许连接
- 使用随机字符串作为连接名称

```go
ConnectTo(srvs) / DisconnectAll()
```
- 控制客户端能连接哪些服务器
- 用于模拟网络分区或客户端受限场景

**设计亮点：**
- **懒加载连接**：只有在需要时才创建连接
- 通过 `srvs` 列表支持细粒度连接控制
- 使用 `map[string]end` 缓存连接，避免重复创建

---

### 5. **persister.go** - 持久化存储

**功能：**
- 模拟服务器的持久化存储（如磁盘）
- 存储Raft状态和快照

**关键方法：**

```go
Save(raftstate, snapshot)
```
- 原子保存Raft状态和快照
- 确保两者同步

```go
Copy() *Persister
```
- 深拷贝Persister
- **重要**：服务器重启时，会创建新的Persister并复制旧状态

**设计亮点：**
- 使用 `clone()` 函数深拷贝字节数组
- 所有操作都有 `mu` 保护
- 提供 `RaftStateSize()` 和 `SnapshotSize()` 用于统计

---

### 6. **annotation.go** - 可视化注解系统

**功能：**
- 记录测试过程中的事件（RPC、分区、崩溃等）
- 生成可视化HTML文件（使用porcupine库）

**关键概念：**

```go
type Annotation struct {
    annotations []porcupine.Annotation  // 点事件和区间事件
    continuous  map[string]Continuous   // 持续事件（如网络分区）
}
```

**事件类型：**
- **Point**：瞬时事件（如RPC调用）
- **Interval**：有开始和结束的事件（如操作执行）
- **Continuous**：持续状态（如网络分区）

**重要函数：**

```go
Annotate(tag, desp, details)          // 用户注解
AnnotateConnection(connection []bool) // 框架：连接状态
AnnotateShutdown(servers []int)       // 框架：服务器崩溃
```

**设计亮点：**
- 使用全局变量 `annotation`，方便在任意位置添加注解
- 通过环境变量 `VIS_ENABLE` 和 `VIS_FILE` 控制输出
- 捕获 `SIGINT` 信号，中断时也能生成可视化

---

### 7. **tester_test.go** - 使用示例

展示了如何使用tester框架：

```go
// 1. 定义服务器结构，实现 IService 接口
type Server struct {
    n      rpc.Tversion
    killed bool
}
func (s *Server) Kill() { s.killed = true }

// 2. 实现 startServer 函数
func (ts *Test) startServer(...) []tester.IService {
    ts.s = newSrv()
    return []tester.IService{ts.s}
}

// 3. 创建测试配置
cfg := tester.MakeConfig(t, nsrv, true, ts.startServer)

// 4. 创建客户端
clnt := cfg.MakeClient()

// 5. 调用RPC
clnt.Call(servername, "Server.Get", &args, &reply)
```

---

## 三、关键设计模式

### 1. **工厂模式**
- `MakeConfig()` 创建配置
- `MakeClient()` 创建客户端
- `MakePersister()` 创建持久化器

### 2. **回调模式**
- `FstartServer` 函数类型，用户提供服务器启动逻辑
- 框架调用用户的函数创建服务实例

### 3. **资源管理**
- `Cleanup()` 确保测试结束时清理所有资源
- 使用 `defer` 确保清理

### 4. **并发安全**
- 大量使用 `sync.Mutex` 保护共享状态
- 使用 `atomic` 操作统计计数

---

## 四、工作流程

### 典型测试流程：

1. **初始化** (`MakeConfig`)
   - 创建Network
   - 创建Groups和Clnts
   - 调用 `MakeGroupStart` 启动服务器

2. **启动服务器** (`StartServers`)
   - 为每个服务器创建Server实例
   - 调用用户的 `mks` 函数创建服务
   - 注册到Network

3. **测试执行**
   - 客户端通过 `Call()` 发送RPC
   - 可以调用 `Partition()` 模拟网络分区
   - 可以调用 `ShutdownServer()` 模拟崩溃

4. **清理** (`Cleanup`)
   - 关闭所有客户端连接
   - 关闭所有服务器（调用Kill()）
   - 清理Network
   - 生成可视化文件（如果启用）

---

## 五、使用建议

1. **理解接口**：实现 `IService` 接口（至少需要 `Kill()` 方法）
2. **正确使用Persister**：服务器重启时会传递旧的Persister副本
3. **网络控制**：使用 `SetReliable()`、`SetLongDelays()` 等测试各种网络条件
4. **调试技巧**：启用 `VIS_ENABLE=always` 查看可视化
5. **并发测试**：框架强制使用多核，注意线程安全

---

## 六、常见问题

**Q: 为什么每次重启都创建新的Persister？**
A: 防止旧实例的异步操作覆盖新实例的状态。新Persister复制旧状态，保证一致性。

**Q: ServerName的格式是什么？**
A: `"server-{gid}-{i}"`，例如 `"server-0-0"` 表示组0的第0个服务器。

**Q: 如何控制客户端只能连接特定服务器？**
A: 使用 `MakeClientTo(srvs []string)` 指定服务器列表。

**Q: Partition是如何工作的？**
A: 调用 `disconnect` 断开分区间的连接，调用 `connect` 建立分区内连接。

