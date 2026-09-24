@echo off
rem FlowOps Executor 薄入口（Windows）
rem 只负责转调 scripts\start.ps1，业务逻辑全部在主逻辑脚本中。
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\start.ps1" %*
exit /b %ERRORLEVEL%
