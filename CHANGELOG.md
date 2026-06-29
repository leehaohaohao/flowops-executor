# Changelog

本项目遵循 [语义化版本](https://semver.org/lang/zh-CN/)（SemVer）规范。

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
