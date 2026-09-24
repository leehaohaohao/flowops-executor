#!/usr/bin/env bash
#
# FlowOps Executor 启动脚本主逻辑（Linux）
#
# 设计依据: docs/2026-09-24-unified-startup-script-plan.md (v7)
# 由根目录薄入口 start.sh 调用；本脚本全程不调用任何 Go 工具链。
#
# 模式:
#   native  前台运行 bin/linux-amd64/flowops-executor（不依赖 Docker）
#   docker  调用 Docker 启动入口启动 executor 容器（默认模式，不依赖宿主机二进制）
#
set -u

# ---------------------------------------------------------------- 常量

DEFAULT_APP_ENV="prod"
DOCKER_ENTRY_DEFAULT="docker/start.sh"
NATIVE_BIN_REL="bin/linux-amd64/flowops-executor"
DOCKER_INFO_TIMEOUT=20

# 退出码（计划 §D19）
EXIT_OK=0
EXIT_USAGE=1
EXIT_PLATFORM=2
EXIT_MODE=3
EXIT_NO_DOCKER_CLI=4
EXIT_DOCKER_DAEMON=5
EXIT_BIN_MISSING=6
EXIT_ENTRY_MISSING=7
EXIT_CONTAINER_FAIL=8

# ---------------------------------------------------------------- 日志

log_info() { printf '[INFO] %s\n' "$*"; }
log_warn() { printf '[WARN] %s\n' "$*"; }
log_error() { printf '[ERROR] %s\n' "$*" >&2; }

usage() {
	cat <<'EOF'
FlowOps Executor 启动脚本（Linux）

用法:
  ./start.sh [native|docker] [选项] [executor 参数...]

模式:
  native   在宿主机前台运行 bin/linux-amd64/flowops-executor（不依赖 Docker）
  docker   通过 Docker 启动入口运行 executor 容器（Linux 默认模式）

选项:
  --check        只检查运行环境，不启动 executor、不启动容器、不修改文件
  --foreground   仅 docker 模式：前台 attach（透传 Docker 入口退出码）
  -h, --help     显示本帮助

环境变量:
  APP_ENV               prod | dev（默认 prod）
  FLOWOPS_DOCKER_ENTRY  覆盖 Docker 启动入口路径（默认 docker/start.sh）

说明:
  模式只能作为第一个位置参数。脚本选项需写在 executor 参数之前；
  遇到第一个未知参数后，native 模式将其余参数原样透传给 executor，
  docker 模式则报参数错误（容器内参数协议尚未定义）。

退出码:
  0 成功                        1 参数/APP_ENV/目录错误
  2 平台不支持                  3 运行模式非法
  4 Docker CLI 不存在           5 Docker daemon 不可用或超时
  6 native 二进制缺失或不可执行  7 Docker 入口缺失
  8 Docker executor 启动失败
EOF
}

# ---------------------------------------------------------------- 全局状态

ROOT=""
MODE=""
MODE_EXPLICIT=0
CHECK=0
FOREGROUND=0
EXECUTOR_ARGS=()

# ---------------------------------------------------------------- 公共阶段

# [1/4] 定位部署目录：只依据稳定运行包入口特征
locate_root() {
	local script_dir
	script_dir=$(cd "$(dirname "$0")" && pwd -P) || {
		log_error "[1/4] 无法定位 FlowOps Executor 部署目录（脚本路径解析失败）"
		exit "$EXIT_USAGE"
	}
	ROOT=$(cd "$script_dir/.." && pwd -P)

	if [ ! -d "$ROOT/scripts" ] || [ ! -f "$ROOT/start.sh" ]; then
		log_error "[1/4] 无法定位 FlowOps Executor 部署目录"
		log_error "期望目录包含: scripts/ 与 start.sh（推导路径: $ROOT）"
		exit "$EXIT_USAGE"
	fi
	log_info "[1/4] 部署目录: $ROOT"
}

# [2/4] 平台检测：仅支持 Linux amd64
detect_platform() {
	local os arch
	os=$(uname -s 2>/dev/null || echo unknown)
	arch=$(uname -m 2>/dev/null || echo unknown)

	if [ "$os" != "Linux" ]; then
		log_error "[2/4] 不支持的平台: $os/$arch"
		log_error "当前支持: Windows amd64（start.bat）/ Linux amd64（start.sh）"
		exit "$EXIT_PLATFORM"
	fi
	case "$arch" in
	x86_64 | amd64) ;;
	*)
		log_error "[2/4] 不支持的平台: $os/$arch"
		log_error "当前支持: Windows amd64 / Linux amd64"
		exit "$EXIT_PLATFORM"
		;;
	esac
	log_info "[2/4] 平台: linux/amd64"
}

# [3/4] 运行模式解析：模式仅作为第一个位置参数；脚本选项先消费
parse_mode_and_options() {
	# 第一步：模式（仅识别第一个位置参数）
	# - native/docker → 显式模式
	# - 脚本选项（--check/--foreground/-h/--help）→ 模式用平台默认
	# - 其它以 - 开头 → 视为未知选项，后续按模式透传或报错
	# - 其它不带 - 的值 → 非法模式（计划 §D4）
	if [ $# -gt 0 ]; then
		case "$1" in
		native | docker)
			MODE="$1"
			MODE_EXPLICIT=1
			shift
			;;
		--check | --foreground | -h | --help) ;;
		-*) ;;
		*)
			log_error "[3/4] 非法运行模式: $1"
			log_error "Linux 可选: native / docker"
			exit "$EXIT_MODE"
			;;
		esac
	fi

	# 未指定模式：Linux 平台默认 docker
	if [ "$MODE_EXPLICIT" -eq 0 ]; then
		MODE="docker"
		log_info "[3/4] 未指定运行模式，平台默认模式: docker"
	else
		log_info "[3/4] 运行模式: $MODE（显式）"
	fi

	# 第二步：脚本选项；遇未知参数按模式处理
	while [ $# -gt 0 ]; do
		case "$1" in
		--check)
			CHECK=1
			shift
			;;
		--foreground)
			FOREGROUND=1
			shift
			;;
		-h | --help)
			usage
			exit "$EXIT_OK"
			;;
		*)
			if [ "$MODE" = "native" ]; then
				# native：其余参数原样透传给 executor
				EXECUTOR_ARGS=("$@")
				break
			fi
			log_error "[3/4] 参数错误: docker 模式不支持参数 '$1'"
			log_error "容器内 executor 参数协议尚未定义；如需向容器传参请通过 Docker 入口协议"
			exit "$EXIT_USAGE"
			;;
		esac
	done

	# --foreground 仅 docker 模式有效
	if [ "$FOREGROUND" -eq 1 ] && [ "$MODE" != "docker" ]; then
		log_error "[3/4] 参数错误: --foreground 仅适用于 docker 模式"
		exit "$EXIT_USAGE"
	fi

	log_info "[4/4] 进入 $MODE 模式"
}

# ---------------------------------------------------------------- APP_ENV

# 校验 APP_ENV（默认 prod），非法值 exit 1
validate_app_env() {
	local stage="$1"
	if [ -z "${APP_ENV:-}" ]; then
		APP_ENV="$DEFAULT_APP_ENV"
	fi
	case "$APP_ENV" in
	prod | dev) ;;
	*)
		log_error "[$stage] APP_ENV 非法: $APP_ENV"
		log_error "支持值: prod / dev"
		exit "$EXIT_USAGE"
		;;
	esac
}

# ---------------------------------------------------------------- native 模式

run_native() {
	local bin="$ROOT/$NATIVE_BIN_REL"

	# [native 1/3] 二进制存在性与执行权限
	if [ ! -f "$bin" ]; then
		log_error "[native 1/3] 未找到 executor: $bin"
		log_error "请确认部署包包含当前平台二进制（bin/linux-amd64/flowops-executor）"
		exit "$EXIT_BIN_MISSING"
	fi
	if [ ! -x "$bin" ]; then
		if [ "$CHECK" -eq 1 ]; then
			# --check 不修改文件：缺权限即视为环境不满足
			log_error "[native 1/3] executor 缺少执行权限: $bin"
			log_error "--check 不修改文件；请去掉 --check 由脚本自动 chmod +x，或手工执行: chmod +x '$bin'"
			exit "$EXIT_BIN_MISSING"
		fi
		if chmod +x "$bin" 2>/dev/null && [ -x "$bin" ]; then
			log_warn "[native 1/3] executor 缺少执行权限，已自动 chmod +x"
		else
			log_error "[native 1/3] 无法为 executor 添加执行权限: $bin"
			exit "$EXIT_BIN_MISSING"
		fi
	fi
	log_info "[native 1/3] executor: $bin"

	# [native 2/3] APP_ENV
	validate_app_env "native 2/3"
	log_info "[native 2/3] APP_ENV: $APP_ENV"

	if [ "$CHECK" -eq 1 ]; then
		log_info "[native 3/3] --check 完成：环境满足，未启动 executor"
		exit "$EXIT_OK"
	fi

	# [native 3/3] 前台启动：exec 替换当前进程，SIGINT/SIGTERM 直接送达 executor，
	# executor 退出码即本脚本退出码（因此不再打印"已退出"日志）
	log_info "[native 3/3] 启动 executor（前台）"
	if [ ${#EXECUTOR_ARGS[@]} -gt 0 ]; then
		exec "$bin" "${EXECUTOR_ARGS[@]}"
	else
		exec "$bin"
	fi
}

# ---------------------------------------------------------------- docker 模式

# docker info 超时执行；返回 0=成功 124=超时 其它=docker info 退出码
docker_info_with_timeout() {
	if command -v timeout >/dev/null 2>&1; then
		timeout "$DOCKER_INFO_TIMEOUT" docker info >/dev/null 2>&1
		return $?
	fi
	# 无 timeout 命令：后台执行 + 轮询 + kill，实现真实超时
	docker info >/dev/null 2>&1 &
	local pid=$!
	local waited=0
	while kill -0 "$pid" 2>/dev/null; do
		if [ "$waited" -ge "$DOCKER_INFO_TIMEOUT" ]; then
			kill -9 "$pid" 2>/dev/null
			wait "$pid" 2>/dev/null
			return 124
		fi
		sleep 1
		waited=$((waited + 1))
	done
	wait "$pid"
	return $?
}

# [docker 1/3] Docker CLI + daemon
check_docker() {
	local docker_bin
	docker_bin=$(command -v docker 2>/dev/null || true)
	if [ -z "$docker_bin" ]; then
		log_error "[docker 1/3] 未检测到 Docker CLI"
		log_error "请先安装 Docker 后重试（本脚本不安装、不启动 Docker）"
		exit "$EXIT_NO_DOCKER_CLI"
	fi
	log_info "[docker 1/3] Docker CLI: $docker_bin"

	local rc
	docker_info_with_timeout
	rc=$?
	if [ "$rc" -eq 124 ]; then
		log_error "[docker 1/3] Docker daemon 检查超时（${DOCKER_INFO_TIMEOUT}s）"
		log_error "请确认 Docker 已启动后重试（本脚本不自动启动 Docker）"
		exit "$EXIT_DOCKER_DAEMON"
	fi
	if [ "$rc" -ne 0 ]; then
		log_error "[docker 1/3] Docker daemon 不可用（docker info exit=$rc）"
		log_error "请启动 Docker daemon 后重试（本脚本不自动启动 Docker）"
		exit "$EXIT_DOCKER_DAEMON"
	fi
	log_info "[docker 1/3] Docker daemon: 正常"
}

# [docker 2/3] Docker 入口解析
resolve_docker_entry() {
	local tried_default="$ROOT/$DOCKER_ENTRY_DEFAULT"
	if [ -n "${FLOWOPS_DOCKER_ENTRY:-}" ]; then
		case "$FLOWOPS_DOCKER_ENTRY" in
		/*) DOCKER_ENTRY="$FLOWOPS_DOCKER_ENTRY" ;;
		*) DOCKER_ENTRY="$ROOT/$FLOWOPS_DOCKER_ENTRY" ;;
		esac
	else
		DOCKER_ENTRY="$tried_default"
	fi
	if [ ! -f "$DOCKER_ENTRY" ]; then
		log_error "[docker 2/3] 未找到 Docker 启动入口: $DOCKER_ENTRY"
		log_error "已尝试路径:"
		log_error "  1) FLOWOPS_DOCKER_ENTRY=${FLOWOPS_DOCKER_ENTRY:-（未设置）}"
		log_error "  2) $tried_default"
		log_error "可用 FLOWOPS_DOCKER_ENTRY 指定入口，例如: FLOWOPS_DOCKER_ENTRY=./ci/docker-entry.sh ./start.sh docker"
		exit "$EXIT_ENTRY_MISSING"
	fi
	log_info "[docker 2/3] Docker 入口: $DOCKER_ENTRY"
}

# [docker 3/3] 调用 Docker 入口
run_docker_entry() {
	local rc
	if [ "$FOREGROUND" -eq 1 ]; then
		log_info "[docker 3/3] 启动 executor 容器（前台 attach）"
		APP_ENV="$APP_ENV" FLOWOPS_DOCKER_FOREGROUND=1 bash "$DOCKER_ENTRY"
	else
		log_info "[docker 3/3] 启动 executor 容器（后台）"
		APP_ENV="$APP_ENV" bash "$DOCKER_ENTRY"
	fi
	rc=$?
	if [ "$rc" -ne 0 ]; then
		log_error "[docker 3/3] Docker 入口执行失败（exit=$rc）"
		log_error "容器未达到可运行状态，请查看 Docker 入口输出"
		exit "$EXIT_CONTAINER_FAIL"
	fi
	log_info "executor 容器已达到可运行状态"
}

run_docker() {
	check_docker

	validate_app_env "docker 2/3"
	log_info "[docker 2/3] APP_ENV: $APP_ENV"

	resolve_docker_entry

	if [ "$CHECK" -eq 1 ]; then
		log_info "[docker 3/3] --check 完成：环境满足，未启动容器"
		exit "$EXIT_OK"
	fi

	run_docker_entry
}

# ---------------------------------------------------------------- 入口

# 早期 help 预检：脚本选项区内出现 -h/--help 即显示用法（不依赖部署目录与平台）
# 规则与正式解析一致：遇到第一个未知参数即停止扫描（其后参数归 executor）
probe_help() {
	[ $# -gt 0 ] || return 0
	case "$1" in
	native | docker) shift ;;
	esac
	while [ $# -gt 0 ]; do
		case "$1" in
		--check | --foreground) shift ;;
		-h | --help)
			usage
			exit "$EXIT_OK"
			;;
		*) return 0 ;;
		esac
	done
}

main() {
	probe_help "$@"
	locate_root
	detect_platform
	parse_mode_and_options "$@"

	case "$MODE" in
	native) run_native ;;
	docker) run_docker ;;
	*)
		log_error "[3/4] 非法运行模式: $MODE"
		log_error "Linux 可选: native / docker"
		exit "$EXIT_MODE"
		;;
	esac
}

main "$@"
