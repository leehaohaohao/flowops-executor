#!/usr/bin/env bash
#
# FlowOps Executor 薄入口（Linux）
# 只负责转调 scripts/start.sh，业务逻辑全部在主逻辑脚本中。
#
exec bash "$(dirname "$0")/scripts/start.sh" "$@"
