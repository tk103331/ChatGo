// 插件能力实验室：一个包演示 ChatGo 插件系统的各类钩子。
// 清单声明贡献，这里只实现行为。宿主 API 是同步的，async 函数同样可用。

const DEFAULT_NOTES = [
  { id: "n1", title: "发布清单", body: "1. 跑测试\n2. 更新 CHANGELOG\n3. 构建三端产物" },
  { id: "n2", title: "排查记录", body: "MCP 连接超时通常是 npx 首次下载依赖，重试即可。" },
  { id: "n3", title: "设计备忘", body: "插件渠道只监听 127.0.0.1，外部回调需要带 token。" },
];

function maskingOn(ctx) {
  return String(ctx.settings.masking ?? "true") !== "false";
}

export const tools = {
  echo(args, ctx) {
    const text = String(args.text ?? "").trim();
    const tone = ctx.settings.tone || "friendly";
    const greeting = ctx.settings.greeting || "你好";
    const body = tone === "terse" ? text : `${greeting}，${text}`;
    ctx.log.info("echo", body);
    return { text: body, tone, plugin: ctx.plugin.id };
  },

  render(args) {
    const type = String(args.view || "table");
    const base = { v: 1, view: { type, title: "Lab · " + type } };
    switch (type) {
      case "chart":
        base.view.labels = ["一月", "二月", "三月", "四月"];
        base.view.series = [
          { name: "请求数", values: [120, 180, 90, 240] },
          { name: "错误数", values: [4, 9, 2, 7] },
        ];
        break;
      case "keyvalue":
        base.view.items = [
          { label: "插件", value: "lab" },
          { label: "运行时", value: "moejs" },
          { label: "能力", value: "15 项" },
        ];
        break;
      case "list":
        base.view.items = [
          { title: "工具", detail: "echo / render / fail", badge: "3" },
          { title: "命令", detail: "brief / draft", badge: "2" },
          { title: "触发器", detail: "interval", badge: "1" },
        ];
        break;
      case "code":
        base.view.language = "js";
        base.view.code = "export const tools = { echo(args) { return args.text; } };";
        break;
      case "diff":
        base.view.path = "main.js";
        base.view.before = "const port = 8080;";
        base.view.after = "const port = Number(process.env.PORT || 8080);";
        break;
      case "links":
        base.view.items = [
          { title: "ChatGo", url: "https://github.com/tk103331/ChatGo" },
          { title: "moejs", url: "https://github.com/Calcium-Ion/moejs" },
        ];
        break;
      case "markdown":
        base.view.text = "**Markdown 视图**\n\n- 支持列表\n- 支持 `代码`\n- 支持 [链接](https://example.com)";
        break;
      case "image":
        base.view.src = "https://picsum.photos/320/180";
        base.view.alt = "示例图片";
        break;
      default:
        base.view.type = "table";
        base.view.title = "Lab · table";
        base.view.columns = [
          { key: "name", label: "名称" },
          { key: "value", label: "值" },
        ];
        base.view.rows = [["tools", "echo / render / fail"], ["events", "4"], ["theme", "lab-neon"]];
    }
    return { $chatgo: base };
  },

  fail() {
    throw new Error("这是 lab 工具故意抛出的错误");
  },
};

export const middleware = {
  lab_marker: {
    beforeRequest(req, ctx) {
      return { systemAppend: "[lab] 本条请求经过插件中间件（provider=" + req.provider + "）。" };
    },
    onChunk(text, ctx) {
      return text.replace(/LAB_OK/g, "✅");
    },
    afterResponse(resp, ctx) {
      return { text: String(resp.text || "").trim() };
    },
    onError(err, ctx) {
      return { message: "（Lab 中间件）" + err.message, retry: false };
    },
  },
};

export const events = {
  onEvent(name, payload, ctx) {
    const state = ctx.store.get("events") || {};
    state[name] = (state[name] || 0) + 1;
    state.lastName = name;
    state.lastAt = new Date().toISOString();
    if (name === "tool.afterCall") {
      state.lastTool = payload.name;
    }
    ctx.store.set("events", state);
  },
};

export function onToolResult(payload, ctx) {
  if (!maskingOn(ctx)) {
    return payload.result;
  }
  return String(payload.result)
    .replace(/sk-[A-Za-z0-9_-]{6,}/g, "sk-***")
    .replace(/secret/gi, "***");
}

export const commands = {
  draft(args, ctx) {
    const topic = String(ctx.draft || "").trim() || "当前项目";
    return { draft: `请为「${topic}」写一段发布说明，包含变化、风险和回滚方式。` };
  },
};

export const contextProviders = {
  notes: {
    search(query, ctx) {
      const notes = ctx.store.get("notes") || DEFAULT_NOTES;
      const q = String(query || "").toLowerCase();
      return notes
        .filter((n) => !q || n.title.toLowerCase().includes(q) || n.body.toLowerCase().includes(q))
        .slice(0, 10)
        .map((n) => ({ id: n.id, label: n.title, detail: n.body.slice(0, 60) }));
    },
    resolve(id, ctx) {
      const notes = ctx.store.get("notes") || DEFAULT_NOTES;
      const note = notes.find((n) => n.id === id);
      return { text: note ? note.title + "\n\n" + note.body : "未找到笔记" };
    },
  },
};

export const triggers = {
  evaluateInterval(trigger, lastRunISO, nowISO) {
    const seconds = Math.max(10, Number(trigger.seconds || 60));
    const base = lastRunISO ? Date.parse(lastRunISO) : Date.parse(nowISO);
    return { next: new Date(base + seconds * 1000).toISOString() };
  },
};

export const channels = {
  labchat: {
    verifyWebhook(req, ctx) {
      const body = req.json || {};
      const secret = String(ctx.channel.app_secret || "");
      if (secret && String(body.secret || "") !== secret) {
        return { ok: false };
      }
      return {
        ok: true,
        chatId: String(body.chatId || "lab-chat"),
        fromUser: String(body.from || "lab-user"),
        text: String(body.text || ""),
        replyToken: String(body.replyToken || ""),
      };
    },
    sendMessage(target, markdown, ctx) {
      // 演示渠道只写日志，不真正外发；生产插件在这里调用 ctx.http。
      ctx.log.info("labchat send", target, markdown);
      return { ok: true, target };
    },
  },
};
