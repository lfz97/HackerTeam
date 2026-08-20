#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Quake (360 网络空间测绘) API v3 命令行客户端
无需第三方依赖，凭据自动从 ~/.hyperbot 或 .hyperbot 读取。

用法示例:
  python3 quake_api.py info
  python3 quake_api.py search --query 'service:http' --size 5
  python3 quake_api.py search --query 'ip:1.2.3.4' --kind host
  python3 quake_api.py scroll --query 'service:http' --size 1000 --save-page /tmp/p.json
  python3 quake_api.py scroll --page /tmp/p.json --size 1000
  python3 quake_api.py agg --query 'country:"china"' --fields service,org
  python3 quake_api.py favicon --hash <md5> --similar 0.9
  python3 quake_api.py fields [service|host]
  python3 quake_api.py help-query
"""
import argparse
import json
import os
import re
import sys
import urllib.request
import urllib.error

API_BASE = "https://quake.360.net/api/v3"


def _default_cred_paths():
    """基于脚本自身位置向上查找凭据，避免与具体部署路径耦合。
    查找顺序: QUAKE_CREDENTIALS 环境变量 > 逐级向上在每级目录查找
    <root>/quake-credentials.yaml、<root>/.HackerTeam/、<root>/.hyperbot/
    > 家目录中的 .HackerTeam/.hyperbot。
    """
    paths = []
    env = os.environ.get("QUAKE_CREDENTIALS")
    if env:
        paths.append(env)
    here = os.path.dirname(os.path.abspath(__file__))
    # 从脚本目录逐级向上（覆盖 scripts/ 到项目根）
    root = here
    for _ in range(6):
        paths.append(os.path.join(root, "quake-credentials.yaml"))
        paths.append(os.path.join(root, ".HackerTeam", "quake-credentials.yaml"))
        paths.append(os.path.join(root, ".hyperbot", "quake-credentials.yaml"))
        parent = os.path.dirname(root)
        if parent == root:
            break
        root = parent
    paths.append(os.path.expanduser("~/.HackerTeam/quake-credentials.yaml"))
    paths.append(os.path.expanduser("~/.hyperbot/quake-credentials.yaml"))
    seen, uniq = set(), []
    for p in paths:
        if p not in seen:
            seen.add(p)
            uniq.append(p)
    return uniq


def load_api_key():
    """从 yaml 凭据文件读取 api_key（自动查找，支持 QUAKE_CREDENTIALS 环境变量覆盖）。"""
    for path in _default_cred_paths():
        if os.path.isfile(path):
            with open(path, "r", encoding="utf-8") as f:
                m = re.search(r"^\s*api_key\s*:\s*([^\s#]+)", f.read(), re.M)
            if m:
                return m.group(1).strip().strip('"').strip("'")
            break
    sys.exit(
        "[!] 未找到 Quake API Key。请放置 quake-credentials.yaml（格式: api_key: <uuid>）"
        "到项目配置目录(.HackerTeam/.hyperbot)或设置环境变量 QUAKE_CREDENTIALS。"
    )


def http_request(method, path, api_key, body=None, params=None):
    url = API_BASE + path
    if params:
        qs = urllib.parse.urlencode(params)
        url += ("&" if "?" in url else "?") + qs
    headers = {"X-QuakeToken": api_key}
    data = None
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            return json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as e:
        try:
            err = json.loads(e.read().decode("utf-8"))
        except Exception:
            err = {"code": str(e.code), "message": e.reason}
        return err
    except Exception as e:
        return {"code": "net", "message": str(e)}


def check(result):
    if result.get("code") != 0:
        msg = result.get("message", "未知错误")
        hint = ""
        if result.get("code") in ("q5000", 500):
            hint = "（可能是查询语法/参数问题，检查 query 或用 help-query 查看语法）"
        if result.get("code") == "q3012":
            hint = "（当前账号缺少该接口权限，如 favicon 查询需开通相应 API 权限）"
        sys.exit(f"[!] Quake API 错误 code={result.get('code')}: {msg} {hint}")
    return result


def cmd_info(api_key, args):
    r = check(http_request("GET", "/user/info", api_key))
    d = r["data"]
    print(json.dumps({
        "username": d.get("user", {}).get("username"),
        "fullname": d.get("user", {}).get("fullname"),
        "mobile_phone": d.get("mobile_phone"),
        "ban_status": d.get("ban_status"),
        "month_remaining_credit": d.get("month_remaining_credit"),
        "constant_credit": d.get("constant_credit"),
        "free_query_api_count": d.get("free_query_api_count"),
        "source": d.get("source"),
        "roles": [x.get("fullname") for x in d.get("role", [])],
    }, ensure_ascii=False, indent=2))


def cmd_fields(api_key, args):
    kind = args.kind if args.kind in ("host",) else "service"
    r = check(http_request("GET", f"/filterable/field/quake_{kind}", api_key))
    print(json.dumps({"kind": kind, "fields": r["data"]}, ensure_ascii=False, indent=2))


def cmd_search(api_key, args):
    kind = args.kind if args.kind in ("host",) else "service"
    body = {
        "query": args.query,
        "start": args.start,
        "size": args.size,
        "ignore_cache": args.ignore_cache,
        "latest": True,
    }
    if args.start_time:
        body["start_time"] = args.start_time
    if args.end_time:
        body["end_time"] = args.end_time
    if args.include:
        body["include"] = args.include.split(",")
    if args.exclude:
        body["exclude"] = args.exclude.split(",")
    if args.rule:
        body["rule"] = args.rule
    r = check(http_request("POST", f"/search/quake_{kind}", api_key, body=body))
    out = {"code": r["code"], "meta": r.get("meta"), "data": r["data"]}
    print(json.dumps(out, ensure_ascii=False, indent=2))


def cmd_scroll(api_key, args):
    kind = args.kind if args.kind in ("host",) else "service"
    if args.page:
        with open(args.page, "r", encoding="utf-8") as f:
            page = json.load(f)
        body = {"pagination_id": page["pagination_id"], "size": args.size, "ignore_cache": args.ignore_cache}
        if args.query:
            body["query"] = args.query
        # 官方示例翻页用 POST（curl 示例为 GET，但 POST 更稳定，兼容各种中间层）
        r = check(http_request("POST", f"/scroll/quake_{kind}", api_key, body=body))
    else:
        body = {"query": args.query, "size": args.size, "ignore_cache": args.ignore_cache, "latest": True}
        if args.start_time:
            body["start_time"] = args.start_time
        if args.end_time:
            body["end_time"] = args.end_time
        r = check(http_request("POST", f"/scroll/quake_{kind}", api_key, body=body))
    out = {"code": r["code"], "meta": r.get("meta"), "data_count": len(r["data"])}
    if args.save_page and r.get("meta", {}).get("pagination_id"):
        with open(args.save_page, "w", encoding="utf-8") as f:
            json.dump({"pagination_id": r["meta"]["pagination_id"], "query": args.query}, f)
        out["page_saved_to"] = args.save_page
    print(json.dumps(out, ensure_ascii=False, indent=2))


def cmd_agg(api_key, args):
    kind = args.kind if args.kind in ("host",) else "service"
    body = {
        "query": args.query,
        "size": args.size,
        "ignore_cache": args.ignore_cache,
        "aggregation_list": args.fields.split(","),
        "latest": True,
    }
    if args.start_time:
        body["start_time"] = args.start_time
    if args.end_time:
        body["end_time"] = args.end_time
    r = check(http_request("POST", f"/aggregation/quake_{kind}", api_key, body=body))
    print(json.dumps({"code": r["code"], "data": r["data"]}, ensure_ascii=False, indent=2))


def cmd_favicon(api_key, args):
    body = {
        "favicon_hash": args.hash,
        "similar": args.similar,
        "size": args.size,
        "ignore_cache": args.ignore_cache,
    }
    if args.start_time:
        body["start_time"] = args.start_time
    if args.end_time:
        body["end_time"] = args.end_time
    r = check(http_request("POST", "/query/similar_icon/aggregation", api_key, body=body))
    print(json.dumps({"code": r["code"], "data": r["data"]}, ensure_ascii=False, indent=2))


def cmd_help_query():
    help_text = """Quake 常用查询语法:
  服务:  service:http  service:ssh  service:ftp  service:dns  service:tls  service:smb
  IP:    ip:"1.2.3.4"    ip:"1.2.3.0/24"    多IP用 OR 或 rule:ip_list
  域名:  domain:"example.com"   hostname:"*.example.com"
  地区:  country:"china"  province:"beijing"  city:"shanghai"
  组织:  org:"腾讯"  org:"China Telecom"
  组件:  app:"nginx"  product:"Apache"  vendor:"Microsoft"  title:"后台管理"
  favicon: favicon:"<md5>"
  逻辑:  AND / OR / NOT，如 service:http AND country:"china" NOT title:"登录"
  时间:  start_time/end_time 格式 "YYYY-MM-DD HH:MM:SS"(UTC)，仅付费用户可指定，免费用户默认近一年
"""
    print(help_text)


def main():
    p = argparse.ArgumentParser(description="Quake API v3 客户端")
    sub = p.add_subparsers(dest="cmd", required=True)

    sub.add_parser("info", help="用户信息/余额")
    sub.add_parser("help-query", help="查询语法帮助")

    pf = sub.add_parser("fields", help="可筛选字段")
    pf.add_argument("kind", nargs="?", default="service", choices=["service", "host"])

    ps = sub.add_parser("search", help="服务/主机实时查询")
    ps.add_argument("--query", required=True, help="查询语句")
    ps.add_argument("--kind", default="service", choices=["service", "host"])
    ps.add_argument("--start", type=int, default=0)
    ps.add_argument("--size", type=int, default=10)
    ps.add_argument("--ignore-cache", action="store_true")
    ps.add_argument("--start-time", help="UTC, YYYY-MM-DD HH:MM:SS")
    ps.add_argument("--end-time", help="UTC, YYYY-MM-DD HH:MM:SS")
    ps.add_argument("--include", help="逗号分隔的可筛选字段")
    ps.add_argument("--exclude", help="逗号分隔的可筛选字段")
    ps.add_argument("--rule", help="如 ip_list")

    psc = sub.add_parser("scroll", help="深度查询(10W级翻页)")
    psc.add_argument("--query", help="查询语句")
    psc.add_argument("--kind", default="service", choices=["service", "host"])
    psc.add_argument("--size", type=int, default=1000)
    psc.add_argument("--ignore-cache", action="store_true")
    psc.add_argument("--start-time", help="UTC, YYYY-MM-DD HH:MM:SS")
    psc.add_argument("--end-time", help="UTC, YYYY-MM-DD HH:MM:SS")
    psc.add_argument("--page", help="上次保存的分页 JSON 文件，续翻页")
    psc.add_argument("--save-page", help="保存分页 id 到文件")

    pa = sub.add_parser("agg", help="聚合统计")
    pa.add_argument("--query", required=True)
    pa.add_argument("--kind", default="service", choices=["service", "host"])
    pa.add_argument("--fields", required=True, help="逗号分隔的聚合字段，如 service,org")
    pa.add_argument("--size", type=int, default=5)
    pa.add_argument("--ignore-cache", action="store_true")
    pa.add_argument("--start-time")
    pa.add_argument("--end-time")

    pfv = sub.add_parser("favicon", help="favicon 相似资产查询")
    pfv.add_argument("--hash", required=True, dest="hash", help="favicon 的 MD5")
    pfv.add_argument("--similar", type=float, default=0.9)
    pfv.add_argument("--size", type=int, default=10)
    pfv.add_argument("--ignore-cache", action="store_true")
    pfv.add_argument("--start-time")
    pfv.add_argument("--end-time")

    args = p.parse_args()
    if args.cmd == "help-query":
        cmd_help_query()
        return
    api_key = load_api_key()
    {
        "info": cmd_info,
        "fields": cmd_fields,
        "search": cmd_search,
        "scroll": cmd_scroll,
        "agg": cmd_agg,
        "favicon": cmd_favicon,
    }[args.cmd](api_key, args)


if __name__ == "__main__":
    main()
