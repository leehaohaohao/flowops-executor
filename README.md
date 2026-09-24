# FlowOps Executor

## 一键启动（部署包）

部署包内含**预编译产物**与启动脚本，目标机器**不需要安装 Go**。

```
flowops-executor/
├── start.bat / start.sh                  # 薄入口（转调 scripts/）
├── scripts/start.ps1 | start.sh          # 启动逻辑
├── bin/windows-amd64/flowops-executor.exe   # Windows native 产物
├── bin/linux-amd64/flowops-executor         # Linux native 产物
└── docker/                               # docker 模式的启动入口（Linux）
```

### Windows（仅 native）

```bat
start.bat                  :: 默认 native，前台运行
start.bat native --check   :: 只检查运行环境，不启动
start.bat docker           :: Windows 暂不支持 docker 模式（exit 3）
```

### Linux（默认 docker）

```bash
./start.sh                       # 默认 docker 模式（调用 docker/start.sh）
./start.sh native                # 宿主机前台运行预编译二进制
./start.sh native --check        # 只检查环境，不启动
./start.sh docker --check        # 只检查 Docker CLI/daemon 与入口
./start.sh docker --foreground   # 前台 attach（透传 Docker 入口退出码）
```

### 模式对照

| 平台 | native | docker | 默认模式 |
|------|--------|--------|---------|
| Windows amd64 | ✅ 支持 | ❌ 当前不支持（exit 3） | `native` |
| Linux amd64 | ✅ 支持 | ✅ 支持 | `docker` |

- **native**：不依赖 Docker，机器未安装 Docker 也能正常运行
- **docker**：executor 运行在容器内，不要求宿主机存在 `bin/`；容器启动与状态确认由 `docker/start.sh` 负责
- 两种模式**互斥**，不会重复启动 executor

### 退出码

| 码 | 含义 |
|----|------|
| 0 | 成功 |
| 1 | 参数 / `APP_ENV` / 部署目录错误 |
| 2 | 平台不支持（仅支持 Windows/Linux amd64） |
| 3 | 运行模式非法（含 Windows 请求 docker） |
| 4 | Docker CLI 不存在 |
| 5 | Docker daemon 不可用或检查超时（20s） |
| 6 | native 二进制缺失或不可执行 |
| 7 | Docker 启动入口缺失 |
| 8 | Docker executor 启动失败 |

native 模式下 executor 自身的退出码**原样透传**。

### 环境变量

| 变量 | 默认 | 说明 |
|------|------|------|
| `APP_ENV` | `prod` | `prod` / `dev`，决定二进制使用的内嵌配置 |
| `FLOWOPS_DOCKER_ENTRY` | 空 | 覆盖 Docker 启动入口（默认 `docker/start.sh`） |

### 常见失败处理

- **Docker daemon 不可用**（exit 5）：先启动 Docker 再重试；本脚本**不安装、不启动** Docker
- **未找到 executor**（exit 6）：确认部署包包含当前平台二进制；Linux 无执行权限时脚本会自动 `chmod +x`（`--check` 模式不修改文件，直接报错退出）
- **未找到 Docker 启动入口**（exit 7）：确认 `docker/start.sh` 存在，或用 `FLOWOPS_DOCKER_ENTRY` 指定路径
- **改配置不生效**：配置通过 `go:embed` 编译进二进制，外部 `config/` 目录不影响运行；请用 `APP_ENV` 切换内嵌配置
- **模式只能作为第一个位置参数**：`./start.sh native --check` ✅；脚本选项需写在 executor 参数之前（`native` 模式下第一个未知参数之后的参数全部透传给 executor；`docker` 模式会报参数错误）

## 从源码构建与运行（开发）

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
│   └── config.*.yaml   # 各环境配置（编译期 embed 进二进制）
├── runner/             # Runner 执行器
│   ├── runner.go       # 连接管理循环（重连/退避/会话生命周期）+ 注册(token)/心跳
│   ├── task.go         # 任务处理 + docker compose 执行
│   ├── artifact.go     # 产物协议传输（ARTIFACT_REQ/CHUNK/ACK + sha256 校验 + 落盘）
│   ├── query.go        # 容器状态/日志查询处理（CONTAINER_STATUS/LOGS）
│   └── metrics.go      # 宿主 CPU/内存指标采集
├── scripts/            # 启动脚本主逻辑（部署包）
│   ├── start.ps1       # Windows（仅 native）
│   └── start.sh        # Linux（native / docker）
├── bin/                # 预编译产物（部署包；仓库内 gitignore）
│   ├── windows-amd64/flowops-executor.exe
│   └── linux-amd64/flowops-executor
├── docker/             # docker 模式启动入口（由 Docker 规范化任务提供）
├── start.bat           # Windows 薄入口
├── start.sh            # Linux 薄入口
├── main.go             # 入口文件
├── build.bat           # 打包脚本（产出 bin/<platform>/ 二进制）
├── .gitattributes      # 换行符策略（*.sh 强制 LF）
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
