# FlowOps Executor

FlowOps 分布式部署平台的子节点执行器：连接 Master、注册上线、心跳保活、接收并执行部署任务、上报容器状态与日志、按需拉取部署产物。

**同一个已编译的二进制可以部署到不同机器、连接不同 Master，无需重新编译** —— 通过命令行参数、环境变量或外部配置文件提供实际取值。

## 快速开始

```bash
# 方式一：命令行参数
./flowops-executor --master-addr 10.0.0.10:9090 --runner-id runner-a --token secret

# 方式二：环境变量（部署/容器推荐）
FLOWOPS_RUNNER_ID=runner-b FLOWOPS_MASTER_ADDR=10.0.0.10:9090 FLOWOPS_RUNNER_TOKEN=secret ./flowops-executor

# 方式三：外部配置文件
./flowops-executor --config /etc/flowops/executor.yaml
```

---

# 配置方式使用指南

## 优先级

**每个字段独立解析，取第一个非空值**（不是"整份来源二选一"）：

```
CLI  >  ENV  >  外部 YAML  >  内嵌 YAML
```

所以可以混用：`runner.id` 来自 ENV、`runner.master_addr` 来自 CLI、`runner.version` 来自内嵌默认值，完全合法。
空字符串与纯空白视为「未提供」，会继续向后 fallback。

## 方式一：命令行参数（CLI）

**适用场景**：本地调试、临时联调、故障排查（快速验证某个 Master / token 是否可用）。

```bash
./flowops-executor \
  --env prod \
  --runner-id runner-01 \
  --master-addr 10.0.0.10:9090 \
  --token abcdef \
  --runner-version 1.0.0
```

| 参数 | 说明 |
|------|------|
| `--env <prod\|dev>` | 运行环境，决定内嵌配置（默认 `prod`） |
| `--runner-id <id>` | runner 唯一标识 |
| `--master-addr <host:port>` | Master 地址 |
| `--token <token>` | 注册令牌 |
| `--runner-version <ver>` | runner 版本号 |
| `--config <path>` | 外部配置文件路径（见方式三） |
| `-h, --help` | 显示用法（输出到 stdout） |

> ⚠️ **token 安全**：`--token` 会出现在 shell history、`ps` 输出、CI 日志中。
> 功能上完整支持（便于调试），但**正式部署请改用环境变量或外部配置文件**。

## 方式二：环境变量（ENV）

**适用场景**：Docker / Docker Compose、systemd、CI、Master 通过 SSH 自动部署 —— 任何"配置由外部注入"的场合。

```bash
export APP_ENV=prod
export FLOWOPS_RUNNER_ID=runner-02
export FLOWOPS_MASTER_ADDR=10.0.0.10:9090
export FLOWOPS_RUNNER_TOKEN=xxxxx
export FLOWOPS_RUNNER_VERSION=1.0.0

./flowops-executor
```

| 变量 | 等价 CLI 参数 | 说明 |
|------|--------------|------|
| `APP_ENV` | `--env` | `prod` / `dev`（默认 `prod`） |
| `FLOWOPS_RUNNER_ID` | `--runner-id` | runner 唯一标识 |
| `FLOWOPS_MASTER_ADDR` | `--master-addr` | Master 地址 `host:port` |
| `FLOWOPS_RUNNER_TOKEN` | `--token` | 注册令牌 |
| `FLOWOPS_RUNNER_VERSION` | `--runner-version` | runner 版本号 |
| `FLOWOPS_CONFIG` | `--config` | 外部配置文件路径（见方式三） |

## 方式三：外部配置文件

**适用场景**：字段较多、需要集中管理或在机器上留档；容器中通过挂载提供。

先创建配置文件（可从 `config/config.example.yaml` 复制）：

```yaml
# /etc/flowops/executor.yaml
runner:
  id: runner-03
  master_addr: 10.0.0.10:9090
  version: 1.0.0
```

然后指定它：

```bash
./flowops-executor --config /etc/flowops/executor.yaml
# 或
FLOWOPS_CONFIG=/etc/flowops/executor.yaml ./flowops-executor
```

**只需写要覆盖的字段**，未写字段继续 fallback（例如上面没写 `token`，就从 ENV 或内嵌配置取）。

### 定位顺序与失败策略

定位顺序：

```
--config  >  FLOWOPS_CONFIG  >  <可执行文件目录>/config/config.{APP_ENV}.yaml
```

默认路径基于**可执行文件所在目录**（不是当前工作目录），因此从任意 CWD 启动都能找到。

| 情形 | 行为 |
|------|------|
| 显式指定（`--config` / `FLOWOPS_CONFIG`）的文件缺失、不可读、YAML 非法 | **启动失败**，不 fallback（你已经明确指定了它） |
| 默认路径不存在 | 正常，继续使用内嵌配置 |
| 默认路径存在但读取失败（权限 / 损坏 / YAML 非法） | **启动失败**，不静默回退（避免误连测试 Master） |

## 混合使用（正式部署推荐）

常见做法：**非敏感字段放配置文件，token 由环境注入**（既便于集中管理，又避免令牌落盘）。

`/etc/flowops/executor.yaml`：

```yaml
runner:
  id: runner-01
  master_addr: 10.0.0.10:9090
  version: 1.0.0
```

启动：

```bash
FLOWOPS_CONFIG=/etc/flowops/executor.yaml \
FLOWOPS_RUNNER_TOKEN=xxxxx \
./flowops-executor
```

最终取值：

| 字段 | 取值来源 |
|------|---------|
| `runner.id` | 外部 YAML |
| `runner.master_addr` | 外部 YAML |
| `runner.version` | 外部 YAML |
| `runner.token` | ENV |

再叠加 CLI 覆盖单项（临时指定另一个 Master）：

```bash
FLOWOPS_CONFIG=/etc/flowops/executor.yaml \
FLOWOPS_RUNNER_TOKEN=xxxxx \
./flowops-executor --master-addr 10.0.0.99:9090     # master 以 CLI 为准
```

## 如何确认取值来源

启动时会打印每个字段的**实际来源**（`cli` / `env` / `external` / `embedded` / `default`）：

```
[INFO] APP_ENV: prod (env)
[INFO] Config File: /etc/flowops/executor.yaml
[INFO] Runner ID: runner-01 (external)
[INFO] Master Addr: 10.0.0.99:9090 (cli)
[INFO] Runner Version: 1.0.0 (external)
[INFO] Runner Token: configured (env)
```

- `Config File: （未使用外部配置，使用内嵌配置）` 表示没有加载任何外部配置文件
- **token 只显示 `configured` / `missing`，不输出明文**

## 常见问题排查

| 现象 | 原因与处理 |
|------|-----------|
| 改了配置文件但没生效 | 看启动日志的 `Config File` 是否是**你改的那个文件**；`--config` > `FLOWOPS_CONFIG` > 默认路径，检查是否有更高优先级来源覆盖（日志中括号内即来源） |
| 报 `配置校验失败，以下字段...均为空: runner.token` | 所有来源都没有 token：用 `FLOWOPS_RUNNER_TOKEN`、`--token` 或外部配置提供 |
| 报 `读取外部配置失败 (由 --config/FLOWOPS_CONFIG 显式指定: ...)` | 显式指定的文件不存在或不可读；不会回退到内嵌配置 |
| 报 `runner.master_addr 格式非法` | 需为 `host:port`（如 `10.0.0.10:9090`），端口 1-65535 |
| 报 `内嵌配置不存在: config.prod.yaml` | 该环境的配置未编译进当前二进制；从 `config/config.example.yaml` 创建，或用外部配置 / CLI / ENV 提供取值 |
| 主节点拒绝注册（token 无效 / 节点未登记） | 确认 `runner.id` 与 token 与主节点 `nexa_node` 表登记一致；子节点会每 60s 低频重试，主节点补录后无需重启 |

## Docker 部署

完全通过环境变量：

```bash
docker run -d --name flowops-executor \
  -e APP_ENV=prod \
  -e FLOWOPS_RUNNER_ID=runner-linux-01 \
  -e FLOWOPS_MASTER_ADDR=10.0.0.10:9090 \
  -e FLOWOPS_RUNNER_TOKEN=xxxxx \
  -e FLOWOPS_RUNNER_VERSION=1.0.0 \
  flowops-executor:latest
```

外部配置 + ENV 混合（token 不落盘）：

```yaml
services:
  executor:
    image: flowops-executor:latest
    volumes:
      - ./executor.yaml:/etc/flowops/executor.yaml:ro
    environment:
      FLOWOPS_CONFIG: /etc/flowops/executor.yaml
      FLOWOPS_RUNNER_TOKEN: ${FLOWOPS_RUNNER_TOKEN}
```

systemd 示例：

```ini
[Service]
Environment=APP_ENV=prod
Environment=FLOWOPS_RUNNER_ID=runner-01
Environment=FLOWOPS_MASTER_ADDR=10.0.0.10:9090
EnvironmentFile=/etc/flowops/executor.env      # 其中放 FLOWOPS_RUNNER_TOKEN=...
ExecStart=/opt/flowops/flowops-executor
Restart=always
```

## 内嵌配置

`config/config.{prod,dev}.yaml` 通过 `go:embed`（**通配模式**）编译进二进制，作为**最低优先级默认值**。

- 这两个文件是**环境私有配置，刻意不入库**（见 `.gitignore`）——各环境自行维护；公司环境通常由配置中心（Nacos 等）或外部配置提供取值
- 仓库只提供 `config.example.yaml` 作为模板；embed 采用通配模式，因此 clone 后（仅有 example）**仍可正常构建**
- 若某环境的内嵌配置**未编译进当前二进制**（例如 clone 后尚未创建 `config.prod.yaml`），该环境启动时会明确报错并提示：从 `config.example.yaml` 创建，或改用外部配置 / CLI / ENV

> **提示**：内嵌配置会被打包进二进制（包含其中的 token）。如果该二进制需要对外分发，建议不要把敏感 token 写在内嵌配置里，改用 `FLOWOPS_RUNNER_TOKEN` 或外部配置文件。

## 从源码构建

```bash
# 打包（Windows 脚本）
build.bat

# 或手动构建
go build -o flowops-executor .

# 交叉编译 Linux
GOOS=linux GOARCH=amd64 go build -o flowops-executor .
```

## 项目结构

```
flowops-executor/
├── config/                 # 配置加载（多来源逐字段解析）
│   ├── config.go           # Config / RunnerConfig / DatabaseConfig / LogConfig
│   ├── embedded.go         # go:embed 内嵌配置读取（通配模式）
│   ├── env.go              # 环境变量名与读取、APP_ENV 取值
│   ├── cli.go              # 命令行参数解析与用法
│   ├── loader.go           # 外部配置文件定位与读取（含失败策略）
│   ├── resolver.go         # 逐字段解析 + 来源记录
│   ├── validate.go         # 最终配置校验
│   ├── load.go             # Load() 入口 + 安全日志摘要
│   └── config.{prod,dev,example}.yaml
├── runner/                 # Runner 执行器
│   ├── runner.go           # 连接管理循环（重连/退避/会话生命周期）+ 注册(token)/心跳
│   ├── task.go             # 任务处理 + docker compose 执行
│   ├── artifact.go         # 产物协议传输（ARTIFACT_REQ/CHUNK/ACK + sha256 校验 + 落盘）
│   ├── query.go            # 容器状态/日志查询处理（CONTAINER_STATUS/LOGS）
│   └── metrics.go          # 宿主 CPU/内存指标采集
├── main.go                 # 入口：加载配置 → 打印来源摘要 → 启动 Runner
├── build.bat               # Windows 打包脚本
└── go.mod
```

## 运行行为

- **先启动子节点后启动主节点**：子节点保持运行并按退避重试（1s → 2s → … → 30s，含随机抖动），主节点上线后自动注册、开始心跳与任务接收
- **主节点运行中重启**：子节点检测到断线后清理旧会话（停止心跳与读取、关闭连接），自动重新注册
- **认证失败**（token 无效 / 节点未登记）：打印主节点拒绝原因，进入低频受控重试（`auth_retry_interval`，默认 60s），不每秒刷请求；主节点侧补录后无需重启子节点
- **退出信号**（SIGINT/SIGTERM）：等待重试、拨号、已注册三个阶段都能快速结束；已连接时尽力发送 `DISCONNECT` 再关闭连接
- **任务执行中收到退出信号**：等待当前 Docker 命令执行结束后再退出（不中断 `docker compose`，避免留下半成品状态），且退出过程中不再开始新任务
- **任务语义**：断线后不自动重放已下发的任务，由主节点的掉线/超时机制兜底，避免重复执行部署动作
- **配置错误**：属于明确启动错误（缺少 token、master 地址非法、外部配置读取失败等），直接退出并打印原因，与"暂时连不上主节点"区分

### 连接恢复参数（可在内嵌或外部 YAML 中覆盖）

| 配置项 | 默认 | 说明 |
|--------|------|------|
| `runner.connect_timeout` | 5（秒） | 单次拨号超时（可被退出信号立即中断） |
| `runner.reconnect_initial_interval` | 1（秒） | 重连初始退避 |
| `runner.reconnect_max_interval` | 30（秒） | 重连最大退避（指数增长到此上限） |
| `runner.register_timeout` | 10（秒） | 注册响应超时，避免主节点只接 TCP 不回响应时永久阻塞 |
| `runner.auth_retry_interval` | 60（秒） | token 无效 / 节点未登记时的低频重试间隔 |
| `runner.heartbeat_interval` | 10（秒） | 心跳上报间隔 |

> 当前 CLI/ENV 只覆盖核心字段（`id` / `master_addr` / `token` / `version`）；上述参数与 `database`、`log` 支持「外部 YAML > 内嵌 YAML」覆盖，后续可按需追加 CLI/ENV。
