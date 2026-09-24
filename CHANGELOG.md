# Changelog

本项目遵循 [语义化版本](https://semver.org/lang/zh-CN/)（SemVer）规范。

## v0.4.0 (2026-09-23)

### 新增

- 连接恢复（2026-09-23 计划 · 步骤 3）：子节点可先于主节点启动，主节点上线/重启后自动恢复
  - `Runner.Run(ctx)` 阻塞运行**唯一的连接管理循环**：连接 → 带 token 注册 → 心跳 + 消息接收；任一步失败即清理会话并按退避重试（`runner/runner.go`）
  - 有界退避：初始 1s 指数增长至 30s（可配置）+ 0~25% 随机抖动，成功注册后重置
  - **可取消的拨号与注册**（协议 v0.6.x `ConnectContext` / `RegisterContext`）：拨号超时可配置（默认 5s）、注册响应超时默认 10s；等待或单次尝试期间收到退出信号立即中断，主节点只接 TCP 不回响应时不会永久阻塞
  - **错误分类**（不做字符串匹配）：主节点明确拒绝（token 无效/节点未登记）归类为 `registerRejectedError`，进入低频受控重试（默认 60s），避免每秒刷请求；网络类错误持续退避重试
  - **会话生命周期隔离**：每次尝试使用全新 client/连接；会话退出时先取消心跳与读取协程、关闭连接（`client.Close`）并等待其真正退出，再开始下一轮，防止重复心跳与旧会话向新连接回写
  - 任务处理与产物/查询回执改为显式传入当前 `session`；会话缓存连接引用，`Close` 后写入返回错误而非 panic

### 变更

- 协议库升级至 **Go v0.6.1**（`go.mod`），接入 `ConnectContext` / `RegisterContext` / `Close` 等新 API
  - v0.6.1 的修复集中在主节点侧（已注册消息按连接绑定会话鉴权、超时/断开仅通知被移除的会话），`client` / `codec` 包无变化，executor 正常路径不受影响：注册成功后才发心跳与回执、退出时发送 `DISCONNECT` 供主节点条件移除会话、会话隔离保证被接管的旧连接不再发消息
- `main.go` 改用 `signal.NotifyContext` 驱动 `Run(ctx)`，不再在连接/注册失败时直接退出；配置加载失败仍为明确启动错误
- README 修正环境默认值（实际为 `prod`，原文写 `dev`），补充连接恢复配置与运行行为说明

### 说明

- 断线后**不自动重放**已下发任务，由主节点掉线/超时机制兜底（任务持久化与幂等性另立计划）
- 任务执行中收到退出信号时等待当前 `docker compose` 结束（不中断），退出过程中不再开始新任务

## v0.3.0 (2026-08-25)

### 新增

- 产物标准化 + 子节点认证（2026-08-25 计划 · 步骤 3）：
  - 注册令牌认证（L1）：`runner.token` 配置随 `REGISTER` 上报，主节点校验失败拒绝注册；`client.Register` 检查回执 `success`，被拒时直接报错（`runner/runner.go`、`config/`）
  - 协议分块传输替代 HTTP 下载：`runner/artifact.go` 重写为 `ArtifactTransferManager`——发 `ARTIFACT_REQ` → 收 `ARTIFACT_DATA` 分块重组（校验序号连续）→ 整体 sha256 校验 → 按类型落盘（JAR→app.jar、BINARY→app、DIST→安全解压 tar 到 dist/）→ 回 `ARTIFACT_ACK`
  - 产物类型按服务类型推断：backend→JAR（回退 BINARY）、frontend→DIST、fullstack→后端产物 + DIST；`service_type` 作为元数据不再写盘
  - 传输带超时（5 分钟）与大小上限（1GB），期间到达的非产物消息记录并跳过
- 协议库 Go client 补全 token 支持：`WithToken` option、`Register` 携带 token 并校验回执

### 变更

- 移除基于 HTTP 的产物下载（`ArtifactDownloadController` 已由主节点删除，`artifact_url` 不再使用）

## v0.2.0 (2026-08-11)

### 新增

- 产物下载（远程部署完善 · 步骤 2）：`START` 动作前从主节点 `artifact_url` 拉取整 volumeDir 的 tar 包并解压，再以 config 消息为准覆盖写配置（`runner/artifact.go`）
  - 解压复用 `safeJoin` 拦截路径穿越，跳过符号链接/硬链接等特殊条目，防解压逃逸
- 容器状态 / 日志查询处理（远程部署完善 · 步骤 5）：处理 `CONTAINER_STATUS_REQ`（`docker compose ps --format {{json .}}` 解析，多容器取最差状态）与 `CONTAINER_LOGS_REQ`（`docker compose logs --no-color --tail N`，支持 since/until/timestamps），响应复用请求 `request_id` 供主节点关联（`runner/query.go`）
- 抽取共享执行函数 `runCompose`，任务执行与状态/日志查询复用同一 docker compose 调用链

### 修复

- `startTaskLoop` 接收循环新增查询消息分支，此前仅处理 `TASK_DISPATCH_REQ`

## v0.1.0 (2026-06-29)

### 新增

- 项目骨架搭建：嵌入式配置加载（`config/`），支持 `APP_ENV` 环境变量切换
- 集成 [nexa-protocol/go](https://github.com/leehaohaohao/nexa-protocol) 通信协议库（v0.2.0）
- Runner 客户端模块（`runner/`）
  - 连接 Master 主节点（TCP）
  - 注册上线（REGISTER）
  - 定时心跳保活（HEARTBEAT）
  - 优雅断开连接（DISCONNECT）
  - 信号量驱动的优雅关闭（SIGINT/SIGTERM）
