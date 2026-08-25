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
# 开发环境 (默认读取 config.dev.yaml)
go run main.go

# 指定环境
APP_ENV=prod go run main.go
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
│   ├── runner.go       # 连接/注册/心跳/优雅退出
│   ├── task.go         # 任务接收循环 + docker compose 执行
│   ├── artifact.go     # 产物下载（START 前拉取 volumeDir tar 并安全解压）
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

通过环境变量 `APP_ENV` 切换，默认为 `dev`
