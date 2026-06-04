# ThingsPanel PLC4X Connector

English | [中文](#中文说明)

ThingsPanel PLC4X Connector is a ThingsPanel DeviceConnector built on Apache PLC4X / PLC4Go. It connects industrial PLC data into ThingsPanel and turns raw protocol points into standard platform telemetry fields.

This repository is intended for public release under Apache License 2.0.

## What It Does

- Connects ThingsPanel to PLC devices through a unified connector model
- Reads PLC tags and publishes flat ThingsPanel telemetry fields
- Supports controlled downlink writes for tags explicitly marked `writable=true`
- Bridges PLC data into dashboards, alarms, automation, history, and remote operations

## Why It Matters

In PLC projects, the hard part is usually not only "reading registers", but turning site data into reusable platform capability.

This connector helps reduce:

- repeated per-project protocol glue work
- ad hoc gateway scripts
- inconsistent point modeling across projects
- extra effort to connect field data with alarms and automation

With this connector, PLC data can be normalized once and then reused across the rest of ThingsPanel.

## Current Scope

Current standalone repository support:

- `modbus-tcp://...`
- `modbus-rtu://...`
- `modbus-ascii://...`
- `s7://...`

Current design characteristics:

- configuration-driven tag mapping
- flat telemetry publishing to ThingsPanel direct-device MQTT
- guarded write behavior
- optional preconfigured `devices` list for PLC projects without generic discovery

Not in current scope:

- generic PLC auto discovery across arbitrary protocols
- production-grade connection pooling
- full runtime coverage for every PLC4X protocol
- built-in public simulator support in the standalone release

## Architecture Boundary

The intended boundary is:

1. Apache PLC4X / PLC4Go handles PLC protocol communication.
2. This connector converts PLC tags into ThingsPanel-ready fields.
3. ThingsPanel handles storage, visualization, alarms, automation, and command workflows.

This is the recommended long-term split for PLC projects because it keeps protocol complexity in the connector layer and keeps the platform side clean.

## Platform Output Contract

Telemetry is published to `devices/telemetry` as a flat JSON object.

Example:

```json
{
  "temperature": 28.5,
  "counter": 1024,
  "plc_online": true,
  "plc_source": "plc4x-service",
  "plc_observed_at": "2026-06-04T03:29:55Z"
}
```

This allows ThingsPanel to store and display each field separately instead of keeping one large nested JSON blob.

Recommended field style:

- use readable business fields such as `temperature`, `pressure`, `running_status`
- keep protocol addresses in configuration, not in telemetry field names
- use connector trace fields such as `plc_online`, `plc_source`, `plc_observed_at`

## Supported Scenarios

- Equipment runtime monitoring
- Production line data acquisition
- Pump station, HVAC, water treatment, energy, and utility systems
- Remote operations across multiple sites
- OEM or industry solution packaging where PLC access should become a reusable product capability

## Configuration

Service access example:

```json
{
  "connection_string": "modbus-tcp://192.168.1.10:502?unit-identifier=1",
  "poll_interval_seconds": 10,
  "connect_timeout_seconds": 8,
  "tags": [
    {
      "name": "temperature",
      "address": "holding-register:1:REAL"
    },
    {
      "name": "pump_start",
      "address": "coil:1:BOOL",
      "writable": true
    }
  ]
}
```

Preconfigured multi-device example:

```json
{
  "connection_string": "modbus-tcp://192.168.1.10:502?unit-identifier=1",
  "devices": [
    {
      "device_name": "Boiler PLC",
      "device_number": "boiler-plc-01",
      "description": "Boiler line PLC",
      "tags": [
        {
          "name": "temperature",
          "address": "holding-register:1:REAL"
        }
      ]
    }
  ]
}
```

## Downlink

Supported commands:

- query all configured tags
- write a single configured tag when `writable=true`

Query example:

```json
{
  "query": "state"
}
```

Write example:

```json
{
  "write": {
    "tag": "pump_start",
    "value": true
  }
}
```

Write rules:

- the target tag must exist
- the target tag must be configured as writable
- field-level write permission should still follow customer safety and process constraints

## Local Run

Required environment variables usually include:

- `CONNECTOR_SERVICE_IDENTIFIER=plc4x-service`
- `CONNECTOR_INSTANCE_ID=<instance-id>`
- `THINGSPANEL_BACKEND_URL=http://127.0.0.1:9999`
- `TP_MQTT_BROKER=tcp://127.0.0.1:1883`

Run locally:

```bash
go run -tags plc4x .
```

Health and config smoke check:

```bash
curl http://127.0.0.1:9001/health
curl "http://127.0.0.1:9001/api/v1/form/config?form_type=SVCR"
```

## Testing

Unit and manifest validation:

```bash
go test ./...
go test -tags plc4x ./...
tpctl device-connectors validate . --strict
```

Recommended integration test path:

1. Start the connector with `go run -tags plc4x .`
2. Register it in ThingsPanel
3. Create a PLC service access point
4. Bind a device with configured tags
5. Verify telemetry fields appear separately in ThingsPanel
6. Optionally test a safe writable point

Recommended first live test:

- Modbus TCP simulator or test PLC
- a small point set such as one temperature, one counter, one boolean output

Example Modbus-style point mapping:

```json
{
  "connection_string": "modbus-tcp://127.0.0.1:1502?unit-identifier=1",
  "poll_interval_seconds": 10,
  "connect_timeout_seconds": 8,
  "tags": [
    {"name": "temperature", "address": "holding-register:1:REAL"},
    {"name": "counter", "address": "holding-register:10:INT"},
    {"name": "pump_start", "address": "coil:1:BOOL", "writable": true}
  ]
}
```

## Standards and Release Notes

- License: Apache License 2.0
- This connector uses Apache PLC4X / PLC4Go as the PLC communication layer
- The connector should publish flat ThingsPanel telemetry fields instead of nested raw JSON blobs
- Business naming should stay stable across projects even if PLC address expressions differ per site
- Write operations should be explicitly opt-in and controlled per point

## Customer Value Summary

This connector helps ThingsPanel present PLC capability as a platform feature instead of a one-off project script:

- faster project delivery
- lower integration cost
- better reuse across sites and lines
- direct reuse of monitoring, alarms, automation, and historical data
- a cleaner path from control-layer data to business-layer applications

## License

- [LICENSE](./LICENSE)
- [NOTICE](./NOTICE)

---

## 中文说明

ThingsPanel PLC4X Connector 是一个基于 Apache PLC4X / PLC4Go 的 ThingsPanel DeviceConnector，用来把工业 PLC 数据接入 ThingsPanel，并把原始协议点位转换成平台可直接使用的标准字段。

这个仓库面向 Apache 2.0 协议公开发布。

## 这个连接器能做什么

- 以统一的接入模型把 PLC 接入 ThingsPanel
- 读取 PLC 点位并上报为扁平化的 ThingsPanel 遥测字段
- 对显式配置了 `writable=true` 的点位提供受控下行写入
- 把 PLC 数据直接接入监控、告警、自动化、历史数据和远程运维链路

## 它解决什么问题

在 PLC 项目里，难点通常不只是“把寄存器读出来”，而是怎么把现场数据真正变成平台能力。

这个连接器主要解决：

- 每个项目重复做一遍协议胶水代码
- 临时网关脚本多，后期难维护
- 点位命名和平台字段不统一
- PLC 数据和告警、自动化、看板之间缺少直接打通

它的核心价值是：把 PLC 接入从一次性工程工作，变成可复用的平台能力。

## 当前公开仓库支持范围

当前独立公开仓库已覆盖：

- `modbus-tcp://...`
- `modbus-rtu://...`
- `modbus-ascii://...`
- `s7://...`

当前实现特点：

- 通过配置驱动点位映射
- 直接向 ThingsPanel 设备遥测主题发布扁平字段
- 下行写入有显式保护
- 对不支持通用发现的 PLC 场景，可使用预配置 `devices` 列表

当前不在范围内：

- 任意 PLC 协议的通用自动发现
- 生产级连接池
- 覆盖 PLC4X 全部协议的完整运行时支持
- 独立公开仓库内置的模拟 PLC 驱动能力

## 架构边界

推荐边界如下：

1. Apache PLC4X / PLC4Go 负责协议通信
2. 本连接器负责把 PLC 点位翻译成 ThingsPanel 字段
3. ThingsPanel 负责存储、展示、告警、自动化和指令流程

这条边界是比较标准、也更适合长期维护的 PLC 接入方式。

## 平台输出规范

遥测发布到 `devices/telemetry`，消息体应该是扁平 JSON。

例如：

```json
{
  "temperature": 28.5,
  "counter": 1024,
  "plc_online": true,
  "plc_source": "plc4x-service",
  "plc_observed_at": "2026-06-04T03:29:55Z"
}
```

这样 ThingsPanel 会把每个字段拆开存储和展示，而不是把整块 JSON 当成一个大字段。

建议字段风格：

- 业务字段用可读名称，比如 `temperature`、`pressure`、`running_status`
- 协议地址只放在配置里，不放在平台字段名里
- 补充连接器状态字段，比如 `plc_online`、`plc_source`、`plc_observed_at`

## 适用场景

- 设备运行监控
- 产线数据采集
- 泵站、暖通、水处理、能源、公用工程系统
- 多站点远程运维
- OEM 或行业方案沉淀，把 PLC 接入变成可复用产品能力

## 配置示例

服务接入示例：

```json
{
  "connection_string": "modbus-tcp://192.168.1.10:502?unit-identifier=1",
  "poll_interval_seconds": 10,
  "connect_timeout_seconds": 8,
  "tags": [
    {
      "name": "temperature",
      "address": "holding-register:1:REAL"
    },
    {
      "name": "pump_start",
      "address": "coil:1:BOOL",
      "writable": true
    }
  ]
}
```

预配置多设备示例：

```json
{
  "connection_string": "modbus-tcp://192.168.1.10:502?unit-identifier=1",
  "devices": [
    {
      "device_name": "Boiler PLC",
      "device_number": "boiler-plc-01",
      "description": "Boiler line PLC",
      "tags": [
        {
          "name": "temperature",
          "address": "holding-register:1:REAL"
        }
      ]
    }
  ]
}
```

## 下行控制

支持的指令：

- 查询已配置点位
- 对 `writable=true` 的单点位做写入

查询示例：

```json
{
  "query": "state"
}
```

写入示例：

```json
{
  "write": {
    "tag": "pump_start",
    "value": true
  }
}
```

写入规则：

- 点位必须存在
- 点位必须显式标记为可写
- 真正投入现场时，还要结合客户权限和工艺安全约束

## 本地运行

常用环境变量：

- `CONNECTOR_SERVICE_IDENTIFIER=plc4x-service`
- `CONNECTOR_INSTANCE_ID=<instance-id>`
- `THINGSPANEL_BACKEND_URL=http://127.0.0.1:9999`
- `TP_MQTT_BROKER=tcp://127.0.0.1:1883`

启动方式：

```bash
go run -tags plc4x .
```

基础探活：

```bash
curl http://127.0.0.1:9001/health
curl "http://127.0.0.1:9001/api/v1/form/config?form_type=SVCR"
```

## 测试方法

单元测试和清单校验：

```bash
go test ./...
go test -tags plc4x ./...
tpctl device-connectors validate . --strict
```

推荐联调路径：

1. 用 `go run -tags plc4x .` 启动连接器
2. 在 ThingsPanel 里注册连接器
3. 创建 PLC 服务接入点
4. 绑定配置好点位的设备
5. 验证遥测字段在 ThingsPanel 中按字段拆开展示
6. 有条件再测试一个安全的可写点位

推荐第一轮真机或模拟器联调：

- 使用 Modbus TCP 模拟器或测试 PLC
- 先配一个温度点、一个计数点、一个布尔输出点

例如：

```json
{
  "connection_string": "modbus-tcp://127.0.0.1:1502?unit-identifier=1",
  "poll_interval_seconds": 10,
  "connect_timeout_seconds": 8,
  "tags": [
    {"name": "temperature", "address": "holding-register:1:REAL"},
    {"name": "counter", "address": "holding-register:10:INT"},
    {"name": "pump_start", "address": "coil:1:BOOL", "writable": true}
  ]
}
```

## 标准与发布说明

- 许可证：Apache License 2.0
- PLC 通信层使用 Apache PLC4X / PLC4Go
- 连接器应该输出扁平 ThingsPanel 字段，而不是嵌套原始 JSON
- 业务字段命名要尽量稳定，不要把 PLC 地址暴露到平台字段名里
- 写入能力必须是显式开启、按点位控制

## 对客户的价值

这个连接器可以帮助 ThingsPanel 用更像“平台能力”的方式来介绍 PLC 接入，而不是一次性脚本开发：

- 交付更快
- 集成成本更低
- 更容易跨站点和产线复用
- 监控、告警、自动化、历史数据可以直接复用平台能力
- 更容易把控制层数据接到业务层应用上

## 许可证

- [LICENSE](./LICENSE)
- [NOTICE](./NOTICE)
