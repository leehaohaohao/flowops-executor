# Changelog

本项目遵循 [语义化版本](https://semver.org/lang/zh-CN/)（SemVer）规范。

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
