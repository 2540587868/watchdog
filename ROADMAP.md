# Roadmap: Watchdog

> 最后更新: 2026-05-17 | 版本: v0.1

## 项目现状

- 代码文件: 13 个 `.go` 文件
- 测试覆盖率: 从 0% 提升到核心包已覆盖（88 个测试，含 race 检测）
- 已知问题: 0 个数据竞争 + 多个功能缺失
- 技术债: 6 项
- 项目类型: 服务监控 / 看门狗（HTTP + TCP 探测、状态机驱动告警）

## 🔴 P0 — 紧急（立即处理）

（无待处理项）

## 🟠 P1 — 高优先级（本版本）

| # | 类别 | 描述 | 影响 | 工作量 | 关联文件 |
|---|------|------|------|--------|----------|
| 2 | 🔧 techdebt | Degraded 阈值硬编码为 2s，应提取为 Target 或全局配置 | 不同服务无法设置不同的延迟降级阈值 | <1h | ✅ 已完成 |
| 3 | ✨ feature | TLS 证书到期告警未实现：ProbeResult 已采集 TLSExpiry 但无告警逻辑 | HTTPS 服务证书即将过期无法提前预警 | 2-4h | ✅ 已完成 |
| 4 | 🐛 bug | 优雅关闭未正确关闭数据库连接，依赖 main 中 defer 但信号处理中 return 可能跳过 | 数据库 WAL 文件可能未正确刷盘 | <1h | ✅ 已完成 |

## 🟡 P2 — 中优先级（下个版本）

| # | 类别 | 描述 | 影响 | 工作量 | 关联文件 |
|---|------|------|------|--------|----------|
| 5 | 🔧 techdebt | 零测试覆盖：整个项目无任何 `_test.go` 文件 | 核心逻辑（状态机、调度器、告警）无回归保障 | 2-3d | ✅ 已完成 |
| 6 | 📊 observability | Prometheus 自定义指标未注册：已引入 prometheus 依赖并暴露 /metrics 端点，但无自定义指标 | 无法通过 Prometheus 监控探测成功率、延迟、状态分布 | 2-4h | ✅ 已完成 |
| 7 | ✨ feature | 缺少 PUT /api/v1/targets/:id 更新目标端点 | 无法通过 API 修改已有目标配置，只能删除重建 | 2-4h | ✅ 已完成 |
| 8 | 📊 observability | /health 端点仅返回 `{"status":"ok"}`，未检查 DB 和 Scheduler 状态 | 数据库故障或调度器异常时健康检查仍通过 | 2-4h | ✅ 已完成 |
| 9 | 🐛 bug | 优雅关闭未刷写待发送的聚合告警 | 关闭期间缓冲的告警消息丢失 | 2-4h | ✅ 已完成 |
| 10 | ✨ feature | SIGHUP 配置热更新仅更新内存配置，未同步更新 Scheduler 中已运行的目标 | 修改配置后需重启才能生效 | 半天 | ✅ 已完成 |
| 11 | 🔧 techdebt | RingBuffer 的 AllSuccess/AllFail 使用 `any(v) != true` 类型断言，脆弱且仅对 bool 有效 | 泛型约束不明确，易误用 | <1h | ✅ 已完成 |
| 12 | 🔧 techdebt | store.go 中 scanTarget 和 scanTargetFromRows 代码重复（~30行） | 字段变更需同步修改两处 | <1h | ✅ 已完成 |
| 13 | ⚡ perf | HTTPProber 每次探测分配 1MB 固定缓冲区读取响应体 | 即使不需要检查响应体也分配大内存 | <1h | ✅ 已完成 |
| 14 | 🔧 techdebt | 错误信息中英文混用：API 返回英文，告警内容使用中文 | 用户体验不一致 | <1h | ✅ 已完成 |

## 🔵 P3 — 低优先级（排期待定）

| # | 类别 | 描述 | 影响 | 工作量 | 关联文件 |
|---|------|------|------|--------|----------|
| 15 | ✨ feature | 告警升级机制未实现：连续长时间不健康应升级告警级别 | 持续故障可能被忽视 | 半天 | [alerter.go](file:///d:/ccswitch/watchdog/internal/alerter/alerter.go) |
| 16 | 🔧 techdebt | 数据库迁移无版本管理，使用 CREATE TABLE IF NOT EXISTS | 后续 schema 变更无法追踪和自动迁移 | 半天 | [store.go](file:///d:/ccswitch/watchdog/internal/store/store.go#L24-L79) |
| 17 | 📝 docs | 缺少 OpenAPI/Swagger 规范文档 | API 使用者无法自动生成客户端 | 2-4h | [api/](file:///d:/ccswitch/watchdog/internal/api/) |
| 18 | ⚡ perf | 列表端点无分页：ListTargets、ListProbeHistory、ListEvents 返回全量数据 | 目标数量多时响应体积大、性能差 | 2-4h | [router.go](file:///d:/ccswitch/watchdog/internal/api/router.go), [store.go](file:///d:/ccswitch/watchdog/internal/store/store.go) |
| 19 | ✨ feature | 缺少 /api/public/status/:id 单服务状态查询端点 | 无法单独查询某个服务的公开状态 | <1h | [router.go](file:///d:/ccswitch/watchdog/internal/api/router.go) |
| 20 | ✨ feature | 缺少 /api/public/uptime 历史可用性端点 | 无法查询 30 天 uptime 趋势 | 2-4h | [router.go](file:///d:/ccswitch/watchdog/internal/api/router.go), [store.go](file:///d:/ccswitch/watchdog/internal/store/store.go) |
| 21 | 🔧 techdebt | api/router.go 中 generateID/generateToken/hashToken 未使用 | 死代码 | <1h | [router.go](file:///d:/ccswitch/watchdog/internal/api/router.go#L456-L471) |
| 22 | 🔧 techdebt | prober.go 自实现 contains/findSubstring，应使用 strings.Contains | 不必要的重复造轮子 | <1h | [prober.go](file:///d:/ccswitch/watchdog/internal/prober/prober.go#L124-L136) |
| 23 | ✨ feature | 缺少 /api/v1/version 版本信息端点 | Dockerfile 中 ldflags 注入了版本信息但未暴露 | <1h | [main.go](file:///d:/ccswitch/watchdog/cmd/watchdog/main.go) |
| 24 | 🔒 security | Target URL 无格式校验，API 接受任意字符串 | 可能导致 SSRF 或无效探测 | <1h | [router.go](file:///d:/ccswitch/watchdog/internal/api/router.go#L100-L113) |

## ⚪ P4 — 可选（有空再做）

| # | 类别 | 描述 | 影响 | 工作量 | 关联文件 |
|---|------|------|------|--------|----------|
| 25 | ✨ feature | Admin SPA 管理界面和公开状态页前端 | 当前仅 API 可用，无可视化操作界面 | 1周+ | 设计文档规划 |

## 版本规划

| 版本 | 目标 | 包含项目 | 状态 |
|------|------|----------|------|
| v0.2 | 稳定性与安全修复 | #1, #2, #3, #4 | ✅ 已完成 |
| v0.3 | 可观测性与测试补全 | #5, #6, #8, #9 | ✅ 已完成 |
| v0.4 | 功能完善 | #7, #10, #11, #12, #13, #14 | ✅ 已完成 |
| v0.5 | 体验优化 | #15-#24 | ⬜ 计划中 |
| v1.0 | 生产就绪 | #25 + 全面验证 | ⬜ 计划中 |

## 变更记录

| 日期 | 变更 |
|------|------|
| 2026-05-17 | ✅ 完成 #14 告警/API 消息统一为英文 |
| 2026-05-17 | ✅ 完成 #13 HTTPProber 缓冲区 1MB→64KB |
| 2026-05-17 | ✅ 完成 #12 store.go scanTarget 代码重复消除（scanner 接口统一） |
| 2026-05-17 | ✅ 完成 #11 RingBuffer AllSuccess/AllFail 类型断言修复 |
| 2026-05-17 | ✅ 完成 #10 SIGHUP 热更新同步 Scheduler：添加 SyncTargets 方法 |
| 2026-05-17 | ✅ 完成 #9 优雅关闭刷写聚合告警：Alerter.Shutdown() + Aggregator.Flush() |
| 2026-05-17 | ✅ 完成 #8 /health 端点增强：检查 DB（Ping）+ Scheduler 状态，返回 503 当不可用 |
| 2026-05-17 | ✅ 完成 #7 PUT /api/v1/targets/:id 更新端点 + store.UpdateTarget |
| 2026-05-17 | ✅ 完成 #6 Prometheus 自定义指标：6 个指标（probes_total/success/failed, duration, target_state, alerts_sent） |
| 2026-05-17 | ✅ 完成 #5 零测试覆盖→88 个测试（target/ringbuffer/statemachine/alerter/prober），含 race 检测 |
| 2026-05-17 | ✅ 完成 #4 优雅关闭：显式关闭数据库 + 修复循环内 defer cancel 泄漏 |
| 2026-05-17 | ✅ 完成 #3 TLS 证书到期告警：添加 OnProbeResult 检查 + tls_expiry_warning_days 配置 |
| 2026-05-17 | ✅ 完成 #2 Degraded 阈值提取为 Target.DegradedThresholdMs 配置字段 |
| 2026-05-17 | ✅ 完成 #1 HTTPProber 数据竞争修复：拆分为 secureClient/insecureClient 双客户端 |
| 2026-05-17 | 初始创建：5维分析识别 25 项改进，P0 紧急 1 项（数据竞争） |
