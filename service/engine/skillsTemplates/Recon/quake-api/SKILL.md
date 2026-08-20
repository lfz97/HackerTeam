---
name: quake-api
description: 调用 Quake（360 网络空间测绘）API v3 查询公网资产数据。当用户需要做资产排查、攻击面分析、查询 IP/端口/域名/组件指纹、favicon 相似资产、主机信息、资产聚合统计，或提到 Quake、360 测绘、空间引擎、网络空间测绘时触发。即使只是说"查一下某某公司的资产"或"这个IP暴露了什么服务"也应考虑使用。脚本封装了认证、查询语法与全部接口（user/info、search、scroll、aggregation、filterable、similar_icon），凭据自动读取。
---

# Quake API Skill（360 网络空间测绘）

封装 Quake API v3 的完整调用能力：认证、查询语法、全部数据接口、积分管理。

## 何时使用

- 用户想查公网资产：某个 IP 开放了哪些端口/服务、某域名绑定的资产、某公司暴露面
- 用户想找相似资产：通过 favicon/图标/页面特征找同源系统
- 用户想做资产统计：按国家/服务/组件聚合分布
- 用户提到 Quake、360 网络空间测绘、空间引擎查资产

## 凭据

API Key 自动从 `/home/lfz/hackerteam/.HackerTeam/ReconSkills/quake-api/quake-credentials.yaml` 读取（格式：`api_key: <uuid>`）。
脚本会自动向上搜索凭据文件，也支持环境变量 `QUAKE_CREDENTIALS` 指定路径。
不要在命令里硬编码 Key。
可查看余额：`python3 quake_api.py info`

## 快速使用

```bash
cd /home/lfz/hackerteam/.HackerTeam/ReconSkills/quake-api/scripts

# 用户信息 / 余额 / 剩余免费查询次数
python3 quake_api.py info

# 服务数据搜索（默认）
python3 quake_api.py search --query 'service:http' --size 5
python3 quake_api.py search --query 'ip:1.2.3.4'
python3 quake_api.py search --query 'org:"腾讯" AND service:ssh'

# 主机数据搜索
python3 quake_api.py search --query 'ip:1.2.3.4' --kind host

# 深度查询（一次性拉取大批量数据，返回分页 id 后翻页）
python3 quake_api.py scroll --query 'service:http' --size 1000 --save-page /tmp/quake-page.json
python3 quake_api.py scroll --page /tmp/quake-page.json --size 1000

# 聚合统计（按服务/组件/国家等维度）
python3 quake_api.py agg --query 'country:"china"' --fields service,org

# favicon 相似资产查询（favicon 的 MD5）
python3 quake_api.py favicon --hash 827fd6c561d4b1f932f75e0f9a17f766 --similar 0.9
# 注意：该接口需要开通 front.api.query.icon 权限，当前账号无此权限（q3012），
# 页面内查询语法 favicon:"<md5>" 不受影响

# 查看可筛选字段 / 查询语法帮助
python3 quake_api.py fields
python3 quake_api.py help-query
```

## 接口映射

| 场景 | 命令 | 底层接口 |
|------|------|----------|
| 用户信息/余额 | `info` | GET `/api/v3/user/info` |
| 可筛选字段 | `fields [service\|host]` | GET `/api/v3/filterable/field/quake_{service,host}` |
| 服务实时查询 | `search` | POST `/api/v3/search/quake_service` |
| 主机实时查询 | `search --kind host` | POST `/api/v3/search/quake_host` |
| 深度查询 | `scroll` | POST/GET `/api/v3/scroll/quake_{service,host}` |
| 聚合查询 | `agg` | POST `/api/v3/aggregation/quake_{service,host}` |
| favicon 相似 | `favicon` | POST `/api/v3/query/similar_icon/aggregation` |

## 常用查询语法

- 服务：`service:http`、`service:ftp`、`service:ssh`、`service:dns`、`service:tls`
- IP：`ip:"1.2.3.4"`、IP 段 `ip:"1.2.3.0/24"`；多 IP 用 `rule:ip_list` 或 `ip:"a" OR ip:"b"`
- 域名：`domain:"example.com"`、`hostname:"*.example.com"`
- 国家/地区：`country:"china"`、`province:"beijing"`、`city:"shanghai"`
- 组织：`org:"腾讯"`、`org:"China Telecom"`
- 组件/指纹：`app:"nginx"`、`product:"Apache"`、`vendor:"Microsoft"`、`title:"后台管理"`
- favicon：`favicon:"<md5>"`（页面内查询语法）；相似查询用 `favicon` 子命令
- 组合：`AND`/`OR`/`NOT`，如 `service:http AND country:"china" NOT title:"登录"`
- 时间：仅付费用户可指定时间范围，非付费默认近一年；时间参数格式 `YYYY-MM-DD HH:MM:SS`（UTC）

## 积分与配额（重要）

- 每月有免费 API 查询次数（本账号当前 10 次/月），个人中心可查剩余
- 免费次数耗尽后，1 条资产 = 1 积分；本账号当前月度剩余 3000 积分
- `info` 子命令可随时查看 `free_query_api_count` 和 `month_remaining_credit`
- 大规模查询优先用 `scroll`（深度查询），比多次 `search` 分页更高效省积分
- `size` 单次最大 10000，`scroll` 单次最大也是 10000，但可连续翻页

## 返回格式

统一返回 `code=0` 为成功；`data` 为结果数组，`meta` 含分页信息：
- search：`meta.pagination`（count/page_index/page_size/total）
- scroll：`meta.total` + `meta.pagination_id`（用 id 翻页）
- agg：`data` 为按字段聚合的 `{key, doc_count}` 列表

## 脚本能力说明

`quake_api.py` 无需第三方依赖（仅标准库 urllib），输出 JSON。它自动处理：
- 凭据读取与错误提示（Key 缺失/无效时给出明确指引）
- 参数校验（query 必填、size 范围、时间格式）
- 错误码翻译（q5000 等）
- scroll 分页 id 的保存与复用（`--save-page` / `--page`）

## 验证方法

执行 `python3 quake_api.py info` 应返回账号信息；执行一次 `search --query 'service:http' --size 1` 应返回真实资产数据。两者都成功即环境就绪。
