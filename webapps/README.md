# WebApp 框架

一个 WebApp 是一个文件夹：一份清单 + 一个 SPA + 可选的能力资源。
SPA 在 WebView 中运行；清单里声明的工具由页面注册，Agent 调用时通过桥接转发到页面执行。

## 目录结构

```
myapp/
  webapp.yaml          # 清单（必需）
  index.html           # 入口，默认 index.html
  skills/<name>/SKILL.md
  <任意静态资源>
```

## 清单字段

| 字段 | 说明 |
|------|------|
| `id` | 唯一标识，小写字母/数字/连字符 |
| `name` | 显示名称 |
| `description` | 应用描述，用于卡片与系统提示 |
| `icon` | 图标名，默认 `globe` |
| `entry` | 入口 HTML，默认 `index.html` |
| `system` | 覆盖自动生成的系统提示 |
| `tools` | 工具列表：`name` / `description` / `params[]` |
| `webmcp` | MCP 端点：`name` / `url` / `transport` |
| `skills` | 内置技能：`name` / `path` |

## 页面桥接

框架会在入口页面注入 `/__chatgo.js`，暴露 `window.chatgo`：

```js
chatgo.registerTool("draw_add_node", (args) => ({ id: "n1", label: args.label }));
chatgo.registerTools({ a() {}, b() {} });
chatgo.tools();        // 已注册工具名
chatgo.handle(n, a);   // 框架调用入口
chatgo.notify({ ... }); // 通知宿主（工具栏显示最后一条）
chatgo.saveState(obj);  // sessionStorage 持久化
```

## 工具调用流程

1. Agent 调用清单中的工具
2. 框架在页面中执行 `chatgo.handle(name, args)`
3. 页面返回 Promise，结果回传到 `/__chatgo/result`
4. 结果作为工具输出返回给 Agent

约定：页面返回字符串或可 JSON 序列化对象；抛错会作为工具错误返回。

## WebMCP

`webmcp` 声明应用自带的 MCP 端点，会和配置里的 MCP 服务器一起注册：

```yaml
webmcp:
  - name: canvas
    url: /mcp              # 相对路径解析为 webapp-<id>://localhost/mcp
    transport: streamable_http   # 或 sse
```

- 相对 `url` 走应用自己的协议，绝对 `url`（`https://...`）原样使用
- 注册名为 `webapp:<id>/<name>`，与普通 MCP 服务器同处一个注册表
- 端点由应用自己实现：Go 侧用 `Client.Mount("/mcp", handler)`，
  或由浏览器内的服务端代码提供

`draw` 示例内置了一个演示端点，`tools/list` 来自清单，`tools/call` 转发到页面。

## Skill 注入

`skills` 里的 `SKILL.md` 会自动读入该应用会话的系统提示：

```
## Skill: diagramming

（SKILL.md 正文）
```

## 发现顺序

1. `<项目>/.chatgo/webapps/`
2. `<项目>/webapps/`
3. `~/.chatgo/webapps/`
4. 内置示例（编译进二进制）

同一 `id` 先出现者生效。

## 独立运行

```bash
./chatgo --app webapp:draw
```

## 示例

`draw/` 是一个画布应用：拖动节点、连线，并向 Agent 导出 4 个工具
（`draw_list_nodes`、`draw_add_node`、`draw_connect`、`draw_export`）。

`slidev/` 是一个 [Slidev](https://cn.sli.dev) 风格的幻灯片编辑器：左侧写
Markdown、右侧实时预览并翻页（单独一行 `---` 分页，开头的 `---...---` 为
frontmatter），向 Agent 导出 8 个工具（`slidev_get_source`、`slidev_set_source`、
`slidev_list_slides`、`slidev_get_slide`、`slidev_add_slide`、`slidev_update_slide`、
`slidev_delete_slide`、`slidev_go_to`）。内容存在浏览器 localStorage 中。
