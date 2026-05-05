/* ═══════════════════════════════
   Komari · TG Javascript 中文完整版模板
   适配：普通通知 / 流量提醒 / 混合告警 / 每日流量汇总
   说明：
   1. 直接粘贴到 面板 -> 设置 -> 消息发送 -> Javascript
   2. sendMessage(message, title) 负责纯文本或日报消息
   3. sendEvent(event) 负责节点事件消息
   ═══════════════════════════════ */

const TG_TOKEN = "请替换为你的 Bot Token";
const TG_CHAT_ID = "请替换为你的 Chat ID";
const PANEL_URL = "http://127.0.0.1:25774";
const SITE_NAME = "Komari 监控";

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
  if (instanceId) {
    row.push({ text: "节点详情", url: `${PANEL_URL}/instance/${instanceId}` });
  }
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

  const lines = [
    `<b>${escapeHtml(titleMap[event.event] || `事件通知：${event.event || "未知事件"}`)}</b>`,
    "",
  ];

  if (Array.isArray(event.clients) && event.clients.length > 0) {
    for (const client of event.clients) {
      const region = clientRegion(client);
      lines.push(
        `<b>◈ ${escapeHtml(clientName(client))}</b>${region ? `  <code>[ ${escapeHtml(region)} ]</code>` : ""}`
      );
    }
    lines.push("");
  }

  lines.push(`<b>◈ 时间：</b> <code>${escapeHtml(toUTC8(event.time))}</code> <i>UTC+8</i>`);

  if (event.message && String(event.message).trim()) {
    lines.push("");
    lines.push(`<blockquote>${escapeHtml(String(event.message).trim())}</blockquote>`);
  }

  return lines.join("\n");
}

function formatOutgoingMessage(message, title) {
  if (title === "每日流量汇总") {
    return buildDailySummaryMessage(message);
  }

  if (hasHtml(message)) {
    return wrapStandardMessage(title || "通知消息", String(message).trim());
  }

  const body = `<blockquote>${escapeHtml(String(message || "无消息内容"))}</blockquote>`;
  return wrapStandardMessage(title || "通知消息", body);
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
    const fallback = `TG Javascript 模板运行异常：${escapeHtml(error?.message || String(error))}`;
    return await sendMessage(fallback, "通知异常");
  }
}
