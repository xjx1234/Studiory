# 拾习社（Shixi Club）

一个面向中小学的学习工具库项目，支持多学科、多种练习形式，目标是将核心能力封装为统一 API，并逐步接入 Web、微信小程序和 App 等多端。

## 顶层结构

```text
.
├── backend/          # Golang 主后端（统一 API 入口，独立服务）
│   ├── cmd/          # 可执行入口（将来拆分多个服务）
│   ├── internal/     # 业务内部实现（学科、工具、题库、用户等）
│   ├── pkg/          # 可复用的公共库（工具函数、中间件等）
│   └── main.go       # 当前的简易主服务入口（后续会迁移到 cmd）
├── apps/
│   ├── node/         # Node 子服务（BFF / 实时能力 / 网关辅助等）
│   └── python/       # Python 子服务（算法、AI、题目生成等）
├── frontend/         # Vue 前端（PC + Mobile 响应式 Web，独立应用）
└── docs/             # 文档（接口设计、需求说明等）
```

## 目录设计说明

- **backend/**（后端服务，独立运行）
  - 作为整个系统的统一 HTTP API 入口，对外暴露 REST/JSON 接口。
  - Node、Python 子服务通过 RPC/HTTP 调用方式挂载在该层后面，对前端透明。
  - 可以单独启动、单独部署，不依赖 `frontend`。
- **apps/node/**
  - 适合承载：
    - 与前端强相关的 BFF（Backend For Frontend）逻辑。
    - WebSocket/实时推送等能力。
    - 对第三方服务的适配层（如即时翻译服务等）。
- **apps/python/**
  - 适合承载：
    - 英语单词工具的智能出题、难度评估。
    - 语文文本分析、错题推荐等算法类功能。
- **frontend/**（前端应用，独立开发）
  - 使用 Vue 构建一个响应式 Web 应用，同时适配 PC 和 Mobile。
  - 只通过 HTTP 调用 `backend` 暴露的 API，不直接依赖后端代码。
  - 可以单独启动（本地开发服务器）、单独构建和部署。
  - 后续可以将组件逻辑抽象出来，迁移/复用到微信小程序或其他端。

## 英文单词工具（第一期目标）

第一期将围绕「英文单词工具」设计和实现一组稳定的 API，包括但不限于：

- 获取练习配置、模式列表（听写、拼写、词义等）。
- 拉取一组待练习的单词。
- 提交作答并返回判分、解析和错题记录。

> 当前阶段优先打牢项目结构和接口设计，具体业务逻辑与题型细节后续逐步补充。

## 数据库与 Redis

- **PostgreSQL**：主库，存用户、学科/工具、单词集与单词、练习会话与答题记录。迁移文件在 `backend/migrations/`，使用 [golang-migrate](https://github.com/golang-migrate/migrate) 执行：
  ```bash
  migrate -path backend/migrations -database "postgres://user:pass@localhost:5432/shixishe?sslmode=disable" up
  ```
- **Redis**：验证码（短信/邮件）、Refresh Token 黑名单、限流与缓存。键规范见 `docs/redis-keys.md`。
- 后端通过环境变量连接：`DATABASE_URL`、`REDIS_URL`（示例见 `backend/internal/config`）。

