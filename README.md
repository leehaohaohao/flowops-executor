# FlowOps Executor

## 快速开始

### 1. 配置文件

```bash
# 复制配置示例
cp config/config.example.yaml config/config.dev.yaml

# 编辑配置文件，修改数据库连接等信息
```

### 2. 运行

```bash
# 默认读取 config.prod.yaml（APP_ENV 未设置时默认 prod）
go run main.go

# 指定环境
APP_ENV=dev go run main.go
```

### 3. 打包

```bash
# Windows
build.bat

# 或手动打包
go build -o flowops-executor.exe .
```

## 项目结构

```
flowops-executor/
├── config/             # 配置文件
│   ├── config.go       # 配置结构体
│   └── config.*.yaml   # 各环境配置
├── runner/             # Runner 执行器
│   ├── runner.go       # 连接管理循环（重连/退避/会话生命周期）+ 注册(token)/心跳
│   ├── task.go         # 任务处理 + docker compose 执行
│   ├── artifact.go     # 产物协议传输（ARTIFACT_REQ/CHUNK/ACK + sha256 校验 + 落盘）
│   ├── query.go        # 容器状态/日志查询处理（CONTAINER_STATUS/LOGS）
│   └── metrics.go      # 宿主 CPU/内存指标采集
├── main.go             # 入口文件
├── build.bat           # Windows 打包脚本
└── go.mod              # Go 模块定义
```

## 配置说明

配置文件按环境区分：`config.{env}.yaml`

- `config.dev.yaml` - 开发环境
- `config.prod.yaml` - 生产环境

通过环境变量 `APP_ENV` 切换，**默认为 `prod`**。

`runner.token`：注册令牌（L1 认证），需与主节点 `nexa_node` 表录入的令牌一致（主节点存 sha256，这里配明文；不配置则注册会被主节点拒绝）。

连接恢复相关配置（用于主节点未启动或重启的场景）：

| 配置项 | 默认 | 说明 |
|--------|------|------|
| `runner.connect_timeout` | 5（秒） | 单次拨号超时（可被退出信号立即中断） |
| `runner.reconnect_initial_interval` | 1（秒） | 重连初始退避 |
| `runner.reconnect_max_interval` | 30（秒） | 重连最大退避（指数增长到此上限） |
| `runner.register_timeout` | 10（秒） | 注册响应超时，避免主节点只接 TCP 不回响应时永久阻塞 |
| `runner.auth_retry_interval` | 60（秒） | token 无效 / 节点未登记时的低频重试间隔 |

## 运行行为

- **先启动子节点后启动主节点**：子节点保持运行并按退避重试（1s → 2s → … → 30s，含随机抖动），主节点上线后自动注册、开始心跳与任务接收
- **主节点运行中重启**：子节点检测到断线后清理旧会话（停止心跳与读取、关闭连接），自动重新注册
- **认证失败**（token 无效 / 节点未登记）：打印主节点拒绝原因，进入低频受控重试（`auth_retry_interval`），不每秒刷请求；主节点侧补录后无需重启子节点
- **退出信号**（SIGINT/SIGTERM）：等待重试、拨号、已注册三个阶段都能快速结束；已连接时尽力发送 `DISCONNECT` 再关闭连接
- **任务执行中收到退出信号**：等待当前 Docker 命令执行结束后再退出（不中断 `docker compose`，避免留下半成品状态），且退出过程中不再开始新任务
- **任务语义**：断线后不自动重放已下发的任务，由主节点的掉线/超时机制兜底，避免重复执行部署动作
- **配置加载失败**：属于明确启动错误，直接退出并打印配置路径（与暂时连不上主节点区分）
