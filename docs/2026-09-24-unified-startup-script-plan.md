# FlowOps Executor 启动脚本实现计划

> 状态：**启动脚本已实施**（实现与验证记录见文末 §12）
> 范围：`flowops-executor` **运行包**的启动脚本
> 平台：**Windows amd64 / Linux amd64**
> 撰写时间：2026-09-24
> 版本：**v7：明确平台能力矩阵，Windows 仅 native，Linux 支持 native/docker**

---

## 0. v7 修订说明

本版在 v6 基础上进一步收敛平台能力和运行模式，主要调整如下：

| #  | 修改项                              | v7 处理                                                             |
| -- | -------------------------------- | ----------------------------------------------------------------- |
| 1  | Windows Docker 能力存在但未验证，边界不清晰    | Windows 当前**仅正式支持 native**；执行 `docker` 模式直接提示当前版本不支持              |
| 2  | Linux 示例误写“未指定模式等于 native”       | 修正为 Linux 默认 `docker`，Windows 默认 `native`                         |
| 3  | Docker 验证矩阵仍存在“两平台”描述            | Docker 组全部改为 **Linux**                                            |
| 4  | 完成标志仍引用 Windows Docker 场景        | 删除 Windows Docker 验收，只验收 Linux Docker                             |
| 5  | Docker “running / healthy”语义不够明确 | 有 healthcheck 必须 `healthy`；无 healthcheck 时需保持 `running` 并经过观察窗口   |
| 6  | Docker 容器识别规则未明确                 | 要求 Docker 入口通过 Compose service/project 或固定 label 精确定位 executor 容器 |
| 7  | `--foreground` 退出码容易误解           | 明确透传的是 **Docker 入口退出码**，不承诺等于容器内 executor 原始退出码                   |
| 8  | `APP_ENV` 只规定导出，未要求实际验证容器内结果     | 增加容器内 `APP_ENV` 实际注入验收                                            |
| 9  | 命令行模式/选项解析顺序未明确                  | 模式仅允许作为第一个位置参数；脚本选项先消费，剩余参数 native 才透传                            |
| 10 | 发布包是否必须同时包含 bin/docker 不够清晰      | 明确 native/docker 可按模式分别打包，不要求所有资源同时存在                             |
| 11 | §6 仍写“默认 native”                 | 改为“按平台默认”                                                         |
| 12 | 修订标题版本不一致                        | 全文统一为 v7                                                          |

---

# 1. 背景与目标

`flowops-executor` 以**已经打包完成的运行产物**交付到目标机器。

目标机器：

* 不要求安装 Go；
* 不包含源码也可运行；
* 不执行 `go run`、`go build`、`go mod download`；
* 根据平台与运行模式启动预编译二进制或 Docker 容器。

当前正式支持范围：

| 平台            | native | docker  | 默认模式     |
| ------------- | ------ | ------- | -------- |
| Windows amd64 | ✅ 支持   | ❌ 当前不支持 | `native` |
| Linux amd64   | ✅ 支持   | ✅ 支持    | `docker` |

其中：

### native

直接在宿主机运行已经打包好的：

```text
flowops-executor.exe
```

或：

```text
flowops-executor
```

不依赖 Docker。

### docker

`flowops-executor` 本身运行在 Docker 容器中。

Docker 模式：

* 不启动宿主机 executor 二进制；
* 不要求宿主机存在 `bin/`；
* 由项目既有 Docker 启动入口负责 executor 容器的创建、启动和状态确认。

两种模式互斥。

---

## 1.1 职责边界

| 启动脚本负责                              | 启动脚本不负责                          |
| ----------------------------------- | -------------------------------- |
| 定位部署目录                              | Go SDK 安装                        |
| 检测 Windows/Linux 与 amd64            | Go 版本管理                          |
| 解析 native/docker 模式                 | 源码构建                             |
| native 模式检查并启动预编译二进制                | `go build` / `go mod` / `go run` |
| Linux native 执行权限检查与必要修复            | Docker 安装                        |
| Linux docker 检查 Docker CLI / daemon | 自动启动 Docker daemon               |
| 调用既有 Docker 入口                      | Compose 文件设计                     |
| 校验 `APP_ENV`                        | Docker 镜像构建逻辑设计                  |
| native 透传 executor 参数与退出码           | Docker 网络 / Volume 设计            |
| docker 判断入口成功/失败                    | systemd / Windows Service        |

---

## 1.2 硬性约束

1. 全流程只启动**已经构建完成的运行产物**。
2. 全流程不得调用任何 Go 工具链。
3. Windows 当前仅正式支持 `native`。
4. Linux 支持 `native` 和 `docker`。
5. Windows 默认 `native`。
6. Linux 默认 `docker`。
7. `native` 不依赖 Docker。
8. `docker` 不依赖宿主机 executor 二进制。
9. native/docker 不得重复启动 executor。
10. Docker 不可用时 Docker 模式立即失败。
11. Docker 入口失败时不得误报启动成功。
12. 不自动安装或启动 Docker。
13. native 默认前台运行。
14. Linux docker 默认后台运行。
15. Docker 前台模式通过固定 `--foreground` 开关启用。
16. 不引入自由字符串 Docker 参数拼接。
17. 所有脚本支持无交互调用。
18. `--check` 不启动 executor、不启动容器、不修改文件。

---

# 2. 现状核对

| 项              | 当前情况                                       | 对计划的影响          |
| -------------- | ------------------------------------------ | --------------- |
| 启动脚本           | 当前不存在                                      | 本计划新增           |
| Windows 二进制    | `build.bat` 可产出 `flowops-executor.exe`     | native 使用       |
| Linux 二进制      | `build.bat` 可产出 `flowops-executor`         | native 使用       |
| 实际平台           | windows/amd64、linux/amd64                  | 本计划只支持这两个       |
| CLI 参数         | executor 当前没有正式参数解析                        | 只做脚本层透传预留       |
| 配置             | `go:embed config/*` 内嵌                     | 运行包不依赖外部 config |
| 有效配置选择         | `APP_ENV`                                  | 两模式统一使用         |
| Docker 入口      | 约定 Linux `docker/start.sh`，文件待 Docker 任务提供 | Docker 模式前置依赖   |
| Windows Docker | 当前不使用                                      | 本版不声明支持         |
| Docker 成功语义    | 入口需检查 executor 容器状态                        | 写入接口约定          |

---

# 3. 部署包结构

## 3.1 Windows native 包

```text
flowops-executor/
├── start.bat
├── scripts/
│   └── start.ps1
└── bin/
    └── windows-amd64/
        └── flowops-executor.exe
```

Windows 当前不要求携带 Docker 相关文件。

---

## 3.2 Linux native 包

```text
flowops-executor/
├── start.sh
├── scripts/
│   └── start.sh
└── bin/
    └── linux-amd64/
        └── flowops-executor
```

---

## 3.3 Linux docker 包

```text
flowops-executor/
├── start.sh
├── scripts/
│   └── start.sh
└── docker/
    ├── start.sh
    └── <Docker 入口所需的其它文件>
```

Docker 包不要求存在：

```text
bin/
```

因为 executor 直接运行在容器中。

---

# 4. 关键设计决策

## D1. 启动入口

### Windows

```text
start.bat
    ↓
scripts/start.ps1
```

`start.bat` 只做薄转调：

```bat
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\start.ps1" %*
```

---

### Linux

```text
start.sh
    ↓
scripts/start.sh
```

根目录 `start.sh` 只负责：

```bash
exec bash "$(dirname "$0")/scripts/start.sh" "$@"
```

业务逻辑全部放 `scripts/start.*`。

---

## D2. 部署目录定位

部署目录只能根据**稳定运行包入口**判断。

必须：

```text
scripts/ 存在
```

并且当前平台对应根入口存在：

Windows：

```text
start.bat
```

Linux：

```text
start.sh
```

不得使用以下内容作为根目录识别依据：

```text
go.mod
main.go
runner/
config/
docker/
bin/
```

原因是 `bin/` 和 `docker/` 是否存在与运行模式有关。

定位失败：

```text
[ERROR] [1/4] 无法定位 FlowOps Executor 部署目录
```

退出：

```text
1
```

---

## D3. 平台检测

正式支持：

### Windows

要求：

```text
OS = Windows
ARCH = AMD64
```

native 二进制：

```text
bin/windows-amd64/flowops-executor.exe
```

---

### Linux

要求：

```text
uname -s = Linux
uname -m = x86_64
```

native 二进制：

```text
bin/linux-amd64/flowops-executor
```

---

以下当前均不支持：

```text
macOS
darwin
arm64
aarch64
其它 OS / Arch
```

失败示例：

```text
[ERROR] [2/4] 不支持的平台: linux/arm64
当前支持:
  Windows amd64
  Linux amd64
```

退出：

```text
2
```

---

# D4. 运行模式解析

模式只使用**命令行第一个位置参数**作为配置源。

支持：

```bash
./start.sh native
./start.sh docker
```

Windows：

```bat
start.bat native
```

---

## 默认行为

未指定模式时：

```text
Windows → native
Linux   → docker
```

日志必须打印：

```text
[INFO] [3/4] 未指定运行模式，平台默认模式: native
```

或：

```text
[INFO] [3/4] 未指定运行模式，平台默认模式: docker
```

---

## Windows docker

Windows 当前不正式支持 Docker 模式。

执行：

```text
start.bat docker
```

直接返回：

```text
[ERROR] [3/4] 当前版本 Windows 暂不支持 docker 模式
支持模式: native
```

退出：

```text
3
```

---

## 非法模式

例如：

```bash
./start.sh host
```

输出：

```text
[ERROR] [3/4] 非法运行模式: host
Linux 可选:
  native
  docker
```

退出：

```text
3
```

---

# D5. native 模式

native 流程：

```text
[native 1/3] 检查平台二进制
        ↓
[native 2/3] 校验 APP_ENV
        ↓
[native 3/3] 前台启动 executor
```

native 模式：

```text
不检查 Docker CLI
不检查 Docker daemon
不检查 docker/
```

即使机器完全没有 Docker，也必须可以正常运行。

---

# D6. docker 模式

docker 当前仅在 Linux 正式支持。

流程：

```text
[docker 1/3] 检查 Docker CLI + daemon
        ↓
[docker 2/3] 校验 APP_ENV + Docker 入口
        ↓
[docker 3/3] 启动 executor 容器
```

Docker 模式：

```text
不检查 bin/
不检查宿主机 executor
不启动宿主机 executor
```

---

# D7. native 二进制检查

## Windows

检查：

```text
bin/windows-amd64/flowops-executor.exe
```

不存在：

```text
[ERROR] [native 1/3] 未找到 executor:
<完整路径>
```

退出：

```text
6
```

---

## Linux

检查：

```text
bin/linux-amd64/flowops-executor
```

首先确认文件存在。

然后检查执行权限。

### 正常启动

无执行权限：

```text
chmod +x
```

成功：

```text
[WARN] [native 1/3] executor 缺少执行权限，已自动 chmod +x
```

继续运行。

失败：

```text
[ERROR] [native 1/3] 无法为 executor 添加执行权限
```

退出：

```text
6
```

---

### `--check`

如果无执行权限：

```text
不 chmod
不修改文件
```

输出：

```text
[ERROR] [native 1/3] executor 缺少执行权限
```

退出：

```text
6
```

`--check` 不能因为只是权限问题而返回 0。

---

# D8. APP_ENV

当前 executor 使用内嵌配置：

```text
APP_ENV=prod
APP_ENV=dev
```

合法值：

```text
prod
dev
```

默认：

```text
prod
```

非法值：

```text
APP_ENV=staging
```

输出：

```text
[ERROR] APP_ENV 非法: staging
支持值: prod / dev
```

退出：

```text
1
```

---

## native 模式

启动 executor 前设置/继承：

```text
APP_ENV
```

executor 根据它选择内嵌配置。

---

## docker 模式

主脚本先：

```bash
export APP_ENV
```

再调用：

```text
docker/start.sh
```

Docker 入口必须负责将：

```text
APP_ENV
```

注入 executor 容器。

例如 Compose：

```yaml
environment:
  APP_ENV: ${APP_ENV}
```

或者：

```bash
docker run -e APP_ENV="$APP_ENV" ...
```

### 验收要求

执行：

```bash
APP_ENV=dev ./start.sh docker
```

最终容器内部必须满足：

```text
APP_ENV=dev
```

不能只验证宿主机变量存在。

---

# D9. Docker CLI / daemon 检查

docker 模式首先执行。

## CLI

```bash
command -v docker
```

不存在：

```text
[ERROR] [docker 1/3] 未检测到 Docker CLI
```

退出：

```text
4
```

---

## daemon

执行：

```text
docker info
```

最大等待：

```text
20 秒
```

### 返回非 0

```text
[ERROR] [docker 1/3] Docker daemon 不可用
```

退出：

```text
5
```

### 超时

```text
[ERROR] [docker 1/3] Docker daemon 检查超时（20s）
```

退出：

```text
5
```

---

Linux 优先：

```bash
timeout 20 docker info
```

如果系统没有 `timeout`，脚本必须使用后台进程 + kill/wait 方式实现真实超时。

不得：

```text
sudo
systemctl start docker
service docker start
自动安装 Docker
```

只能给用户提示。

---

# D10. Docker 入口

Linux 默认入口：

```text
docker/start.sh
```

可使用：

```text
FLOWOPS_DOCKER_ENTRY
```

覆盖。

例如：

```bash
FLOWOPS_DOCKER_ENTRY=/opt/custom/start-executor.sh ./start.sh docker
```

解析顺序：

```text
1. FLOWOPS_DOCKER_ENTRY
2. <ROOT>/docker/start.sh
```

都不存在：

```text
[ERROR] [docker 2/3] 未找到 Docker 启动入口
```

同时打印已尝试路径。

退出：

```text
7
```

---

# D11. Docker 容器识别规则

Docker 入口必须能够**唯一识别 executor 容器**。

禁止使用：

```text
模糊 docker ps | grep executor
```

作为唯一识别方式。

建议采用以下任一种稳定方式：

### Compose service

例如：

```text
service = flowops-executor
```

并结合 Compose project。

### 固定 label

例如：

```text
com.nexa.flowops.role=executor
```

Docker 入口协议必须明确采用哪一种。

---

# D12. Docker 成功语义

Docker 入口返回：

```text
0
```

必须表示：

> executor 容器已经启动并达到项目定义的可运行状态。

不能仅表示：

```text
docker compose up -d
```

命令执行成功。

---

## 如果容器配置了 healthcheck

必须满足：

```text
State = running
Health = healthy
```

才返回：

```text
0
```

---

## 如果没有 healthcheck

至少要求：

```text
容器处于 running
```

并经过一个稳定观察窗口，例如：

```text
5~10 秒
```

期间没有退出或重启失败。

满足后才返回：

```text
0
```

否则 Docker 入口返回非 0。

---

主启动脚本不重复实现 Docker health check。

职责为：

```text
主脚本
  ↓
调用 Docker 入口
  ↓
入口自己确认 executor 容器状态
  ↓
0 / 非0
```

---

# D13. Docker 前后台模式

## 默认后台

执行：

```bash
./start.sh docker
```

Docker 入口采用后台语义：

```text
docker compose up -d
```

然后：

```text
等待
↓
检查容器
↓
确认 running / healthy
↓
返回 0
```

主启动脚本随后结束。

executor 容器继续后台运行。

---

## 前台模式

执行：

```bash
./start.sh docker --foreground
```

脚本内部设置固定协议：

```text
FLOWOPS_DOCKER_FOREGROUND=1
```

Docker 入口根据该变量进入前台模式。

例如：

```text
docker compose up
```

或：

```text
docker compose logs -f
```

具体行为由 Docker 入口协议规定。

### 退出码说明

`docker --foreground` 最终透传的是：

> **Docker 入口脚本的退出码。**

不承诺：

```text
Docker 入口退出码
=
容器内 flowops-executor 进程原始退出码
```

除非未来 Docker 入口额外实现该映射。

---

# D14. native 参数透传

native 支持：

```bash
./start.sh native --foo bar
```

脚本解析自身选项后，将剩余参数：

```text
--foo
bar
```

原样传给：

```text
flowops-executor
```

即：

```bash
exec "$BIN" --foo bar
```

---

当前 executor 尚未正式实现 CLI 参数解析。

因此该能力的含义仅为：

> 启动脚本不会吞掉 executor 参数。

不代表：

```text
--master
--config
--log-level
```

当前已经真正生效。

---

# D15. 参数解析顺序

模式只能作为第一个位置参数。

支持：

```bash
./start.sh native --check
./start.sh native --foo bar
./start.sh docker
./start.sh docker --check
./start.sh docker --foreground
```

未指定模式：

```bash
./start.sh
```

使用平台默认。

---

不建议支持：

```bash
./start.sh --check docker
```

本计划规定：

> 模式必须在所有模式相关选项之前。

---

脚本自身参数：

```text
--check
--foreground
--help
```

解析完成后：

### native

剩余参数：

```text
全部交给 executor
```

### docker

剩余未知参数：

```text
报参数错误
```

因为 Docker 容器内 executor 参数协议当前未定义。

---

# D16. native 启动

## Linux

使用：

```bash
exec "$BIN" "$@"
```

这样 Bash 进程被 executor 替换。

因此：

```text
SIGINT
SIGTERM
```

直接送达 executor。

executor 的退出码即启动命令退出码。

---

## Windows

PowerShell：

```powershell
& $exe @executorArgs
exit $LASTEXITCODE
```

前台运行。

Ctrl+C 行为以实际验证为准。

---

# D17. 日志设计

## 公共阶段

```text
[INFO] [1/4] 部署目录: /opt/flowops-executor
[INFO] [2/4] 平台: linux/amd64
[INFO] [3/4] 运行模式: docker（Linux 默认）
[INFO] [4/4] 进入 docker 模式
```

---

## native

```text
[INFO] [native 1/3] executor: bin/linux-amd64/flowops-executor
[INFO] [native 2/3] APP_ENV: prod
[INFO] [native 3/3] 启动 executor
```

---

## docker

```text
[INFO] [docker 1/3] Docker CLI: /usr/bin/docker
[INFO] [docker 1/3] Docker daemon: 正常
[INFO] [docker 2/3] APP_ENV: prod
[INFO] [docker 2/3] Docker 入口: docker/start.sh
[INFO] [docker 3/3] 启动 executor 容器（后台）
[INFO] executor 容器已达到可运行状态
```

---

# D18. `--check`

## native

```bash
./start.sh native --check
```

检查：

```text
部署目录
平台
运行模式
APP_ENV
当前平台 executor 二进制
Linux 执行权限
```

不检查：

```text
Docker
```

不执行：

```text
chmod
executor
```

---

## docker

```bash
./start.sh docker --check
```

检查：

```text
部署目录
平台
运行模式
APP_ENV
Docker CLI
Docker daemon
Docker 入口
```

不检查：

```text
宿主机 bin/
宿主机 executor
```

不启动：

```text
容器
executor
```

---

# D19. 退出码

| 退出码 | 含义                   |
| --- | -------------------- |
| `0` | 成功                   |
| `1` | 参数/用法/APP_ENV/目录错误   |
| `2` | 平台不支持                |
| `3` | 运行模式非法或平台不支持该模式      |
| `4` | Docker CLI 不存在       |
| `5` | Docker daemon 不可用或超时 |
| `6` | native 二进制缺失/不可执行    |
| `7` | Docker 入口缺失          |
| `8` | Docker executor 启动失败 |

native 模式 executor 自身退出码：

```text
原样透传
```

即使恰好等于：

```text
3
6
8
```

也不改写。

日志用于区分：

```text
[INFO] executor 已退出（exit=8）
```

---

# 5. 完整启动流程

```text
start.bat / start.sh
        │
        ▼
[1/4] 定位部署目录
        │
        ▼
[2/4] 检测平台
        │
        ├── 非 Windows/Linux amd64
        │       └── exit 2
        ▼
[3/4] 解析运行模式
        │
        ├── Windows 默认 native
        ├── Linux 默认 docker
        └── 非法 / Windows docker
                └── exit 3
        ▼
[4/4] 按模式分派
```

---

## native

```text
[native 1/3]
检查当前平台二进制
        │
        ├── 缺失 → exit 6
        │
        └── Linux 无执行权限
                ├── --check → exit 6
                └── 正常启动 → chmod +x
        ▼
[native 2/3]
检查 APP_ENV
        │
        └── 非法 → exit 1
        ▼
[native 3/3]
前台启动 executor
        │
        ▼
透传 executor 退出码
```

---

## docker

```text
[docker 1/3]
检查 Docker CLI
        │
        ├── 不存在 → exit 4
        ▼
检查 Docker daemon
        │
        ├── 失败/超时 → exit 5
        ▼
[docker 2/3]
校验 APP_ENV
        │
        ├── 非法 → exit 1
        ▼
解析 docker/start.sh
        │
        ├── 不存在 → exit 7
        ▼
[docker 3/3]
调用 Docker 入口
        │
        ├── 默认后台
        │      └── 等待并验证 executor 容器
        │
        └── --foreground
               └── 前台 attach
        │
        ├── 非0 → exit 8
        ▼
Docker 模式启动成功
```

---

# 6. 分阶段实施

| 阶段 | 内容                           | 验收                                  |
| -- | ---------------------------- | ----------------------------------- |
| 1  | `start.bat` / `start.sh` 薄入口 | 可正确调用 scripts 主逻辑                   |
| 2  | 部署目录定位                       | 任意 CWD 都能定位                         |
| 3  | Windows/Linux amd64 平台检测     | 其它平台 exit 2                         |
| 4  | 模式解析                         | Windows 默认 native / Linux 默认 docker |
| 5  | Windows docker 拒绝逻辑          | exit 3                              |
| 6  | native 二进制检查                 | 缺失/权限行为正确                           |
| 7  | native 前台启动                  | 退出码、Linux 信号正确                      |
| 8  | Linux Docker CLI/daemon 检查   | 未安装/未运行/超时正确区分                      |
| 9  | Docker 入口解析                  | 默认与覆盖路径正确                           |
| 10 | Docker 成功状态协议                | running/healthy 判定正确                |
| 11 | APP_ENV 两模式传递                | native/docker 实际一致                  |
| 12 | `--check`                    | 两模式均零启动副作用                          |
| 13 | 日志/退出码                       | 能定位失败阶段                             |
| 14 | 完整回归                         | §8 全部正式场景通过                         |

---

# 7. 接口约定

## 7.1 Windows

```bat
start.bat
start.bat native
start.bat native --check
```

Windows 当前：

```text
docker → 不支持
```

---

## 7.2 Linux

```bash
./start.sh
./start.sh native
./start.sh native --check
./start.sh docker
./start.sh docker --check
./start.sh docker --foreground
```

其中：

```text
./start.sh
```

等同：

```text
./start.sh docker
```

---

## 7.3 环境变量

| 变量                          | 默认值    | 说明                          |
| --------------------------- | ------ | --------------------------- |
| `APP_ENV`                   | `prod` | `prod` / `dev`              |
| `FLOWOPS_DOCKER_ENTRY`      | 空      | Linux Docker 入口覆盖           |
| `FLOWOPS_DOCKER_FOREGROUND` | 空      | 内部协议，`--foreground` 时设为 `1` |

不再使用：

```text
GO_VERSION
GO_DOWNLOAD_BASE
FLOWOPS_NO_DOWNLOAD
FLOWOPS_RUN_MODE
FLOWOPS_EXECUTOR_MODE
FLOWOPS_DOCKER_ARGS
```

---

# 8. 验证方案

## 8.1 Windows native

| #   | 场景               | 预期                        |
| --- | ---------------- | ------------------------- |
| W1  | 正常启动             | `.exe` 启动成功               |
| W2  | 未指定模式            | 自动使用 native               |
| W3  | 二进制缺失            | exit 6                    |
| W4  | APP_ENV 非法       | exit 1                    |
| W5  | `native --check` | 只检查，不启动                   |
| W6  | 机器没有 Docker      | 不影响 native                |
| W7  | executor 返回非 0   | 原样透传                      |
| W8  | 任意 CWD 执行        | 正确定位目录                    |
| W9  | native 参数透传      | 脚本不吞未知参数                  |
| W10 | Ctrl+C           | 实测并记录实际行为                 |
| W11 | `docker`         | 明确提示当前 Windows 不支持，exit 3 |

---

## 8.2 Linux native

| #   | 场景                | 预期              |
| --- | ----------------- | --------------- |
| N1  | 正常启动              | `exec` 成功       |
| N2  | 无执行权限             | 自动 chmod + WARN |
| N3  | 无执行权限 + `--check` | 不 chmod，exit 6  |
| N4  | binary 缺失         | exit 6          |
| N5  | Docker 未安装        | native 仍正常      |
| N6  | APP_ENV 非法        | exit 1          |
| N7  | executor 非零退出     | 原样透传            |
| N8  | SIGTERM           | 直达 executor     |
| N9  | SIGINT            | 直达 executor     |
| N10 | 任意 CWD            | 正常启动            |
| N11 | 参数透传              | 脚本不吞参数          |

---

## 8.3 Linux docker

| #   | 场景                    | 预期                                |
| --- | --------------------- | --------------------------------- |
| D1  | 未指定模式                 | Linux 自动使用 docker                 |
| D2  | Docker 正常             | executor 容器后台启动成功                 |
| D3  | Docker CLI 缺失         | exit 4                            |
| D4  | daemon 未启动            | exit 5                            |
| D5  | daemon 超时             | exit 5，日志显示超时                     |
| D6  | Docker 入口缺失           | exit 7                            |
| D7  | Docker 入口返回非 0        | exit 8                            |
| D8  | 宿主机无 `bin/`           | 仍能启动                              |
| D9  | Docker 启动后            | 宿主机没有本地 executor 进程               |
| D10 | `APP_ENV=dev`         | 容器内部实际为 `dev`                     |
| D11 | 有 healthcheck         | 必须达到 `healthy` 才成功                |
| D12 | 无 healthcheck         | running 且通过稳定观察窗口才成功              |
| D13 | 容器启动后立即退出             | Docker 入口返回非 0                    |
| D14 | `docker --check`      | 不启动容器                             |
| D15 | `docker --foreground` | 前台 attach                         |
| D16 | foreground 退出         | 透传 Docker 入口退出码                   |
| D17 | 宿主机 binary 缺失         | 不影响 docker                        |
| D18 | 任意 CWD                | 正确定位                              |
| D19 | 容器识别                  | 通过约定 service/project 或 label 精确识别 |

---

# 9. Docker 入口协议要求

Docker 规范化任务提供：

```text
docker/start.sh
```

该脚本必须满足：

1. 接收宿主机 `APP_ENV`。
2. 将 `APP_ENV` 注入 executor 容器。
3. 默认以后台方式启动 executor 容器。
4. `FLOWOPS_DOCKER_FOREGROUND=1` 时进入前台模式。
5. 能唯一识别 executor 容器。
6. 如果配置 healthcheck，等待到 `healthy`。
7. 如果没有 healthcheck，至少确认持续 `running` 一段观察窗口。
8. 容器退出、重启失败、unhealthy 时返回非 0。
9. 只有 executor 容器达到项目定义的成功状态才返回 0。
10. 不要求主启动脚本了解 Compose 服务、网络、Volume 等内部细节。

---

# 10. 风险与待决项

| #  | 风险                          | 处理                                              |
| -- | --------------------------- | ----------------------------------------------- |
| R1 | `docker/start.sh` 尚未真正落地    | Docker 规范化任务提供；此前可用 `FLOWOPS_DOCKER_ENTRY` 临时联调 |
| R2 | Docker 容器标识尚需固定             | Docker 入口协议确定 Compose service/project 或固定 label |
| R3 | executor 未实现 CLI 参数解析       | native 只做透传预留；README 说明当前主要依赖 APP_ENV           |
| R4 | 容器内 executor 参数协议未定义        | 后续 Docker 入口协议单独设计                              |
| R5 | 无 healthcheck 时只能判断 running | 增加 5–10 秒稳定观察窗口                                 |
| R6 | Windows Ctrl+C 行为存在平台差异     | 实机验证，不作 POSIX 等价承诺                              |
| R7 | 打包路径必须与脚本约定一致               | 固定 `windows-amd64` / `linux-amd64`              |
| R8 | 配置被编译进二进制                   | README 明确外部 config 当前不生效                        |

---

# 11. 完成标志

* [ ] 仅支持 Windows/Linux amd64，无 macOS/ARM64 残留。
* [ ] Windows 默认 `native`。
* [ ] Linux 默认 `docker`。
* [ ] Windows 当前只正式支持 native。
* [ ] Windows 请求 docker 时明确拒绝并 exit 3。
* [ ] Linux native/docker 均可显式选择。
* [ ] native 模式完全不依赖 Docker。
* [ ] docker 模式完全不依赖宿主机 executor 二进制。
* [ ] 两种模式互斥，不重复启动 executor。
* [ ] native 启动正确平台的预编译二进制。
* [ ] Linux native 缺执行权限时正常启动自动修复。
* [ ] native `--check` 缺权限时不修改且失败。
* [ ] Linux SIGINT/SIGTERM 正确传给 native executor。
* [ ] native executor 退出码原样透传。
* [ ] Linux Docker CLI/daemon 检查和超时处理完成。
* [ ] Docker 默认入口为 `docker/start.sh`。
* [ ] Docker 入口能唯一定位 executor 容器。
* [ ] 配置 healthcheck 时必须达到 `healthy` 才报告成功。
* [ ] 无 healthcheck 时需通过 running 稳定观察窗口。
* [ ] Docker 入口非 0 时主脚本返回 exit 8。
* [ ] docker 模式不启动任何宿主机 executor。
* [ ] docker `--foreground` 透传 Docker 入口退出码。
* [ ] `APP_ENV` 在 native 与 docker 模式下均实际生效。
* [ ] docker 模式下已验证容器内部 `APP_ENV`。
* [ ] `--check` 在两种模式下均不启动、不修改。
* [ ] 全流程不存在任何 Go 工具链调用。
* [ ] 任意 CWD 下均能正确找到部署目录。
* [ ] README 包含 Windows native、Linux native/docker 的使用方式、退出码和常见错误说明。

---

# 12. 实施与验证记录（2026-09-24）

## 12.1 交付物

| 文件 | 说明 |
| ---- | ---- |
| `start.sh` | Linux 薄入口（`exec bash scripts/start.sh "$@"`），LF 换行 |
| `scripts/start.sh` | Linux 主逻辑（native / docker），LF 换行 |
| `start.bat` | Windows 薄入口（转调 PowerShell），CRLF 换行 |
| `scripts/start.ps1` | Windows 主逻辑（仅 native）。**必须 UTF-8 with BOM** |
| `.gitattributes` | `*.sh → LF`、`*.bat`/`*.ps1 → CRLF` |
| `.gitignore` | 新增 `/bin/`（部署包产物不入库） |
| `README.md` | 新增「一键启动（部署包）」章节：模式对照、退出码表、环境变量、常见失败处理 |
| `bin/<platform>/` | 交叉编译产物（本地验证用，仓库内忽略） |

## 12.2 实现中的取舍

1. **Linux native 使用 `exec` ⇒ 无法打印「executor 已退出（exit=N）」**：`exec` 会用 executor 替换脚本进程，脚本不复存在，故 §D19 中该条区分日志在 native 前台模式下不可实现。取舍上保留 `exec`（信号直达、退出码即脚本退出码），该信息改由 executor 自身输出。
2. **`.ps1` 必须带 UTF-8 BOM**：首次验证即暴露问题 —— 无 BOM 时 Windows PowerShell 5.1 按 ANSI(GBK) 解析含中文的脚本，报 `Array index expression is missing` 等语法错误。`.sh` 相反**不能**有 BOM。
3. **非法模式识别**：首个位置参数若既非 `native/docker`、也非脚本选项且不以 `-` 开头（如 `host`），判定为非法模式 exit 3（§D4）；以 `-` 开头的未知参数在 native 下透传给 executor（便于 `./start.sh --master 127.0.0.1:9090` 之类用法）。
4. **`--help` 提前处理**：在任何阶段日志之前输出并 exit 0，避免 usage 与日志混排。
5. **权限检查的两种语义**：正常启动缺执行权限时自动 `chmod +x` + `[WARN]`；`--check` 下只报告并以 exit 6 失败，**不修改文件**。

## 12.3 验证环境

* Windows：`windows/amd64`，Windows PowerShell 5.1（`start.bat` 调用链）
* Linux：WSL2（`Linux` / `x86_64`）；执行权限行为另在 `/tmp`（ext4）副本上验证（`/mnt/d` 为 drvfs 全权限，`chmod` 不生效）
* 产物：`go build`（windows/amd64）与 `GOOS=linux GOARCH=amd64 go build`（linux/amd64），Go 1.26.3

## 12.4 验证结果

### Windows native

| # | 场景 | 结果 |
| -- | ---- | ---- |
| W1 | `start.bat native` 正常启动 | ✅ executor 进程启动（PID 确认），日志完整，终止后无残留进程 |
| W2 | 未指定模式 | ✅ `平台默认模式: native`，exit 0 |
| W3 | 二进制缺失（重命名 exe） | ✅ exit 6，打印期望路径 |
| W4 | `APP_ENV=staging` | ✅ exit 1 |
| W5 | `native --check` | ✅ 只检查不启动，exit 0 |
| W6 | 机器无 Docker | ✅ native 不受影响（不检查 Docker） |
| W7 | executor 非零退出码透传 | ✅ 探针（固定 exit 7）→ 脚本返回 7 |
| W8 | 任意 CWD 执行 | ✅ 正确定位部署目录 |
| W9 | 参数透传 | ✅ 探针收到 `["--foo" "bar baz" "--master" "127.0.0.1:9090"]`（含空格参数完整） |
| W11 | `start.bat docker` | ✅ exit 3，提示 Windows 暂不支持 docker 模式 |
| — | 非法模式 `host` | ✅ exit 3 |
| — | `APP_ENV=dev` 透传 | ✅ 子进程收到 `APP_ENV=dev` |
| — | 认证拒绝 + 低频重试（对真实 Master 集成） | ✅ `主节点拒绝注册: 节点未登记，请先录入注册令牌` → `1m0s 后重试` |

### Linux（WSL2）

| # | 场景 | 结果 |
| -- | ---- | ---- |
| N1 | `timeout 6 ./start.sh native` | ✅ 前台启动并输出重连退避日志 |
| N2 | 无执行权限 + 正常启动 | ✅ `[WARN] 已自动 chmod +x`，权限位转为 `-rwxr-xr-x` 且启动成功 |
| N3 | 无执行权限 + `--check` | ✅ exit 6，且权限位**保持 `-rw-r--r--`（未修改）** |
| N4 | 二进制缺失 | ✅ exit 6 |
| N5 | Docker 未安装 | ✅ native 仍正常（exit 0） |
| N6 | `APP_ENV=staging` | ✅ exit 1 |
| N8/N9 | SIGTERM 信号 | ✅ executor 打印 `FlowOps Executor 已停止`（`exec` 使信号直达） |
| N10 | 任意 CWD | ✅ 正确定位 |
| — | 平台检测 | ✅ `[2/4] 平台: linux/amd64` |
| — | `--help` | ✅ 用法输出，exit 0 |

### Linux docker（假 docker CLI + 假入口）

WSL 无 Docker，故用可控假 `docker`（`command -v` 可发现、`docker info` 行为可控）与假入口覆盖 docker 分支：

| # | 场景 | 结果 |
| -- | ---- | ---- |
| D1/L3 | 未指定模式 → Linux 默认 docker | ✅ 进入 docker 分支 |
| D2 | Docker 正常 + 入口返回 0 | ✅ 调用入口，`APP_ENV=prod`，exit 0 |
| D3/L2 | Docker CLI 缺失 | ✅ exit 4 |
| D4 | daemon 返回非 0 | ✅ exit 5 |
| D5 | daemon 无响应 | ✅ exit 5，耗时 22s（20s 超时真实生效） |
| D6 | 入口缺失 | ✅ exit 7，列出尝试路径与覆盖方式 |
| D7 | 入口返回非 0 | ✅ exit 8 |
| D8 | 宿主机无 `bin/` | ✅ docker 模式仍成功（不检查二进制） |
| D10 | `APP_ENV=dev` | ✅ 入口收到 `app_env=dev` |
| D14 | `docker --check` | ✅ 检查 CLI/daemon/入口后 exit 0，未启动容器 |
| D15/D16 | `--foreground` | ✅ 入口收到 `FLOWOPS_DOCKER_FOREGROUND=1` |
| — | docker 模式未知参数 | ✅ exit 1 |

### 验证中发现并已修复的问题

1. `.ps1` 缺 UTF-8 BOM 导致中文脚本解析失败（§12.2-2）
2. 非法模式（`host`）被误当作 executor 参数透传并真的启动了进程 → 已按 §D4 修正
3. `--help` 与阶段日志混排 → 已提前至目录定位之前

## 12.5 尚未验证 / 依赖外部条件

| 项 | 原因 | 归属 |
| -- | ---- | ---- |
| W10 Windows Ctrl+C 行为 | 需交互式终端，自动化无法真实模拟 Ctrl+C | 待人工在终端实测（§D16 已允许"以实际验证为准"） |
| D11/D12 容器 healthcheck 与 running 观察窗口 | 依赖真实 Docker 入口实现（`docker/start.sh` 尚未落地） | Docker 规范化任务 + 后续联调 |
| D19 容器唯一识别 | 同上（属 Docker 入口协议） | 同上 |
| 真实 Docker 端到端（Linux 服务器） | 本机 Windows 无 Docker，WSL 内无 daemon | 在装 Docker 的 Linux 服务器按 §8.3 复测 |
| 打包一致性 | `bin/<platform>/` 目前由手工 `go build` 产出 | 建议将 `build.bat` 改为直接输出到 `bin/<platform>/` |
