#!/usr/bin/env bash
set -euo pipefail

: "${CONTAINER:=komari}"
: "${KOMARI_DB:=/app/data/komari.db}"
: "${RULE_NAME:=高负载混合告警}"
: "${CPU_THRESHOLD:=85}"
: "${RAM_THRESHOLD:=90}"
: "${LOAD_THRESHOLD:=4}"
: "${TRAFFIC_THRESHOLD:=80}"
: "${MATCH_COUNT:=2}"
: "${RATIO:=0.6}"
: "${INTERVAL_MINUTES:=5}"

if [[ -z "${TG_TOKEN:-}" || -z "${TG_CHAT_ID:-}" || -z "${PANEL_URL:-}" || -z "${SITE_NAME:-}" || -z "${CLIENTS_JSON:-}" ]]; then
  cat <<'EOF'
缺少必填环境变量。

至少需要：
  TG_TOKEN
  TG_CHAT_ID
  PANEL_URL
  SITE_NAME
  CLIENTS_JSON

示例：
  CONTAINER=komari \
  TG_TOKEN='123456:ABC' \
  TG_CHAT_ID='-1001234567890' \
  PANEL_URL='http://140.245.121.220:25774' \
  SITE_NAME='我的 Komari' \
  CLIENTS_JSON='["服务器UUID1","服务器UUID2"]' \
  bash scripts/docker-configure-tg-notify.sh
EOF
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "未检测到 docker 命令，请先安装 Docker。" >&2
  exit 1
fi

if ! docker inspect "$CONTAINER" >/dev/null 2>&1; then
  echo "容器不存在：$CONTAINER" >&2
  exit 1
fi

docker exec -u 0 -i \
  -e TG_TOKEN="$TG_TOKEN" \
  -e TG_CHAT_ID="$TG_CHAT_ID" \
  -e PANEL_URL="$PANEL_URL" \
  -e SITE_NAME="$SITE_NAME" \
  -e CLIENTS_JSON="$CLIENTS_JSON" \
  -e KOMARI_DB="$KOMARI_DB" \
  -e RULE_NAME="$RULE_NAME" \
  -e CPU_THRESHOLD="$CPU_THRESHOLD" \
  -e RAM_THRESHOLD="$RAM_THRESHOLD" \
  -e LOAD_THRESHOLD="$LOAD_THRESHOLD" \
  -e TRAFFIC_THRESHOLD="$TRAFFIC_THRESHOLD" \
  -e MATCH_COUNT="$MATCH_COUNT" \
  -e RATIO="$RATIO" \
  -e INTERVAL_MINUTES="$INTERVAL_MINUTES" \
  "$CONTAINER" sh <<'SH'
set -e

if ! command -v python3 >/dev/null 2>&1; then
  apk add --no-cache python3 >/dev/null
fi

python3 - <<'PY'
import json
import os
import sqlite3

db_path = os.environ.get("KOMARI_DB", "/app/data/komari.db")
clients_json = os.environ["CLIENTS_JSON"]

try:
    clients = json.loads(clients_json)
except json.JSONDecodeError as exc:
    raise SystemExit(f"CLIENTS_JSON 不是有效 JSON: {exc}")

if not isinstance(clients, list) or not clients:
    raise SystemExit("CLIENTS_JSON 必须是非空数组，例如：[\"uuid1\",\"uuid2\"]")

script = r"""/* ═══════════════════════════════
   Komari · TG Javascript 中文完整版模板
   适配：普通通知 / 流量提醒 / 混合告警 / 每日流量汇总
   ═══════════════════════════════ */

const TG_TOKEN = "__TG_TOKEN__";
const TG_CHAT_ID = "__TG_CHAT_ID__";
const PANEL_URL = "__PANEL_URL__";
const SITE_NAME = "__SITE_NAME__";

function escapeHtml(value) {
  return String(value ?? "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

function toUTC8(input) {
  const raw = String(input || "");
  if (!raw || raw.startsWith("0001")) return nowUTC8();
  const date = new Date(raw.replace(/\.\d+Z$/, "Z"));
  if (Number.isNaN(date.getTime())) return nowUTC8();
  const utc8 = new Date(date.getTime() + 8 * 3600 * 1000);
  const pad = (n) => String(n).padStart(2, "0");
  return `${utc8.getUTCFullYear()}-${pad(utc8.getUTCMonth() + 1)}-${pad(utc8.getUTCDate())} ${pad(utc8.getUTCHours())}:${pad(utc8.getUTCMinutes())}:${pad(utc8.getUTCSeconds())}`;
}

function nowUTC8() {
  const utc8 = new Date(Date.now() + 8 * 3600 * 1000);
  const pad = (n) => String(n).padStart(2, "0");
  return `${utc8.getUTCFullYear()}-${pad(utc8.getUTCMonth() + 1)}-${pad(utc8.getUTCDate())} ${pad(utc8.getUTCHours())}:${pad(utc8.getUTCMinutes())}:${pad(utc8.getUTCSeconds())}`;
}

function panelHost() {
  return PANEL_URL.replace(/^https?:\/\//, "").replace(/\/+$/, "");
}

function hasHtml(text) {
  return /<\/?(b|i|code|pre|blockquote|u|s)>/.test(String(text || ""));
}

function clientName(client) {
  if (!client) return "未知节点";
  return client.name || client.Name || client.uuid || client.UUID || "未知节点";
}

function clientId(client) {
  return client?.uuid || client?.UUID || null;
}

function clientRegion(client) {
  return client?.region || client?.Region || "";
}

function joinClientNames(clients) {
  if (!Array.isArray(clients) || clients.length === 0) return "未提供节点信息";
  return clients.map(clientName).join("、");
}

function buildKeyboard(instanceId = null) {
  const row = [{ text: "控制台", url: PANEL_URL }];
  if (instanceId) row.push({ text: "节点详情", url: `${PANEL_URL}/instance/${instanceId}` });
  return { inline_keyboard: [row] };
}

async function telegram(method, payload) {
  const resp = await fetch(`https://api.telegram.org/bot${TG_TOKEN}/${method}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  return resp.ok;
}

function wrapStandardMessage(title, body) {
  const footer = `<b>${escapeHtml(SITE_NAME)}</b>  ·  <i>${escapeHtml(panelHost())}</i>`;
  return [`<b>${escapeHtml(title)}</b>`, "", body, "", footer].join("\n");
}

function buildDailySummaryMessage(message) {
  const body = hasHtml(message)
    ? String(message).trim()
    : `<blockquote>${escapeHtml(message).replace(/\n/g, "\n")}</blockquote>`;
  const footer = `<b>${escapeHtml(SITE_NAME)}</b>  ·  <i>${escapeHtml(panelHost())}</i>`;
  return [body, "", footer].join("\n");
}

function buildMixedAlertMessage(event) {
  const first = Array.isArray(event.clients) && event.clients.length > 0 ? event.clients[0] : null;
  const region = clientRegion(first);
  const nodeLine = first
    ? `<b>◈ 节点：</b> ${escapeHtml(clientName(first))}${region ? `  <code>[ ${escapeHtml(region)} ]</code>` : ""}`
    : `<b>◈ 节点：</b> ${escapeHtml(joinClientNames(event.clients))}`;
  const detail = escapeHtml(String(event.message || "检测到多指标混合异常告警。"));
  return [
    "🚨 <b>多指标混合告警</b>",
    "",
    nodeLine,
    `<b>◈ 时间：</b> <code>${escapeHtml(toUTC8(event.time))}</code> <i>UTC+8</i>`,
    "",
    `<blockquote>${detail}</blockquote>`,
  ].join("\n");
}

function buildTrafficAlertMessage(event) {
  const first = Array.isArray(event.clients) && event.clients.length > 0 ? event.clients[0] : null;
  const region = clientRegion(first);
  return [
    "📈 <b>流量使用提醒</b>",
    "",
    `<b>◈ 节点：</b> ${escapeHtml(clientName(first))}${region ? `  <code>[ ${escapeHtml(region)} ]</code>` : ""}`,
    `<b>◈ 时间：</b> <code>${escapeHtml(toUTC8(event.time))}</code> <i>UTC+8</i>`,
    "",
    `<blockquote>${escapeHtml(String(event.message || "已达到流量提醒阈值。"))}</blockquote>`,
  ].join("\n");
}

function buildGenericEventMessage(event) {
  const titleMap = {
    Offline: "🔴 节点离线",
    Online: "🟢 节点恢复",
    Expire: "⏰ 到期提醒",
    Renew: "✨ 续费成功",
    Login: "🔐 登录提醒",
    Alert: "⚠️ 节点告警",
    Traffic: "📈 流量提醒",
  };
  const lines = [`<b>${escapeHtml(titleMap[event.event] || `事件通知：${event.event || "未知事件"}`)}</b>`, ""];
  if (Array.isArray(event.clients) && event.clients.length > 0) {
    for (const client of event.clients) {
      const region = clientRegion(client);
      lines.push(`<b>◈ ${escapeHtml(clientName(client))}</b>${region ? `  <code>[ ${escapeHtml(region)} ]</code>` : ""}`);
    }
    lines.push("");
  }
  lines.push(`<b>◈ 时间：</b> <code>${escapeHtml(toUTC8(event.time))}</code> <i>UTC+8</i>`);
  if (event.message && String(event.message).trim()) {
    lines.push("", `<blockquote>${escapeHtml(String(event.message).trim())}</blockquote>`);
  }
  return lines.join("\n");
}

function formatOutgoingMessage(message, title) {
  if (title === "每日流量汇总") return buildDailySummaryMessage(message);
  if (hasHtml(message)) return wrapStandardMessage(title || "通知消息", String(message).trim());
  return wrapStandardMessage(title || "通知消息", `<blockquote>${escapeHtml(String(message || "无消息内容"))}</blockquote>`);
}

async function sendMessage(message, title, instanceId = null) {
  if (!TG_TOKEN || !TG_CHAT_ID) return false;
  return await telegram("sendMessage", {
    chat_id: TG_CHAT_ID,
    text: formatOutgoingMessage(message, title),
    parse_mode: "HTML",
    disable_web_page_preview: true,
    reply_markup: buildKeyboard(instanceId),
  });
}

async function sendEvent(event) {
  try {
    const clients = Array.isArray(event.clients) ? event.clients : [];
    const firstId = clients.length === 1 ? clientId(clients[0]) : null;
    const rawMessage = String(event.message || "");
    let text = "";
    if (event.event === "Alert" && rawMessage.includes("混合告警规则：")) {
      text = buildMixedAlertMessage(event);
    } else if (event.event === "Traffic") {
      text = buildTrafficAlertMessage(event);
    } else {
      text = buildGenericEventMessage(event);
    }
    return await sendMessage(text, event.event || "通知", firstId);
  } catch (error) {
    return await sendMessage(`TG Javascript 模板运行异常：${escapeHtml(error?.message || String(error))}`, "通知异常");
  }
}
"""

script = (
    script.replace("__TG_TOKEN__", os.environ["TG_TOKEN"])
    .replace("__TG_CHAT_ID__", os.environ["TG_CHAT_ID"])
    .replace("__PANEL_URL__", os.environ["PANEL_URL"])
    .replace("__SITE_NAME__", os.environ["SITE_NAME"])
)

conn = sqlite3.connect(db_path)
cur = conn.cursor()

exists = cur.execute(
    "SELECT name FROM sqlite_master WHERE type='table' AND name='mixed_notifications'"
).fetchone()
if not exists:
    raise SystemExit("当前数据库还没有 mixed_notifications 表。请先升级到包含混合告警功能的新镜像后再执行。")

def upsert_config(key, value):
    cur.execute(
        """
        INSERT INTO configs(key, value)
        VALUES(?, ?)
        ON CONFLICT(key) DO UPDATE SET value = excluded.value
        """,
        (key, json.dumps(value, ensure_ascii=False)),
    )

upsert_config("notification_enabled", True)
upsert_config("notification_method", "Javascript")
upsert_config("daily_traffic_summary_enabled", True)

cur.execute(
    """
    INSERT INTO message_sender_providers(name, addition)
    VALUES(?, ?)
    ON CONFLICT(name) DO UPDATE SET addition = excluded.addition
    """,
    ("Javascript", json.dumps({"script": script}, ensure_ascii=False)),
)

cur.execute("DELETE FROM mixed_notifications WHERE name = ?", (os.environ["RULE_NAME"],))
cur.execute(
    """
    INSERT INTO mixed_notifications(
      name, clients, cpu_threshold, ram_threshold, load_threshold,
      traffic_threshold, match_count, ratio, interval, last_notified
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
    """,
    (
        os.environ["RULE_NAME"],
        json.dumps(clients, ensure_ascii=False),
        float(os.environ["CPU_THRESHOLD"]),
        float(os.environ["RAM_THRESHOLD"]),
        float(os.environ["LOAD_THRESHOLD"]),
        float(os.environ["TRAFFIC_THRESHOLD"]),
        int(os.environ["MATCH_COUNT"]),
        float(os.environ["RATIO"]),
        int(os.environ["INTERVAL_MINUTES"]),
    ),
)

with open("/tmp/komari-tg-full-cn.js", "w", encoding="utf-8") as fp:
    fp.write(script)

conn.commit()
conn.close()
print("已写入 Javascript 模板、开启每日流量汇总，并创建混合告警规则。")
print("模板已导出到容器内：/tmp/komari-tg-full-cn.js")
PY
SH

docker restart "$CONTAINER" >/dev/null
echo "容器已重启，配置已生效：$CONTAINER"
