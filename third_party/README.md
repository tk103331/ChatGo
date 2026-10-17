# third_party

## MujicaUI（本地补丁副本）

`MujicaUI/` 是上游模块 `git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI` 的本地副本，
通过根目录 `go.mod` 的 `replace` 使用：

```
replace git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI => ./third_party/MujicaUI
```

该目录纳入版本控制，基于上游适配 MyGo v0.5.5 的最新代码，叠加以下补丁。

### 补丁

**`internal/highlight.go`** — 修复死循环。上游 `Highlight` 判定用 `rune(rest[0])`（单字节），
但多字节 UTF-8 的首字节（如全角标点 `：` 的首字节 `0xEF`）会被 `unicode.IsLetter` 误判为字母，
随后 `spanLen` 按真 rune 迭代立即返回 `0`，导致 `i += 0` 永不前进，主线程 100% CPU 卡死。
触发条件：Markdown 代码块中出现以全角标点开头的行。
补丁：循环开头用 `utf8.DecodeRuneInString` 取首个 rune 并记录其字节数，
所有分支改用该 rune 判定、按字节数推进，并保证每个分支至少前进 `size` 字节。

**`input/text_input.go` + `input/text_area.go`** — 新增 `Borderless` 支持。
`TextAreaOptions.Borderless` 置 true 时去掉字段自身的边框与背景，
好让调用方把输入框与其工具栏包进同一个框（避免"框内套框"）。

**`chat/prompt_composer.go`** — 新增 `PromptComposerOptions.Box`：置 true 时向
`TextArea` 传递 `Borderless`，效果同上。

**`navigation/tabs.go`** — 水平模式下 panel 也 `Grow(1).MinHeight(0)`。上游只在
`Vertical` 时给 panel `Grow(1)`，水平模式下 panel 高度由内容决定，而 ChatGo 的
设置页 panel 内是 `Scroll.Grow(1).MinHeight(0)`，父级无 Grow 时高度坍缩为 0，
导致整个 tab 内容区空白。

**`code/terminal_tabs.go`** — 新增 `TerminalTabsOptions.Actions`：置非 nil 时在标签条末尾
渲染额外控件，好让 ChatGo 把"新建终端/关闭终端"按钮放进标签条内。

**`icons/svg/chart-column.svg`** — 上游精简图标集时移除了此图标，ChatGo 统计页导航需要，予以恢复。

**paint 闭包内 Element 方法调用（跨 pass panic）** — MyGo 的 `ui.Element` 仅在创建它的
build pass 内有效；`DrawOver`/`Draw` 回调在 paint 阶段执行，且 exit transition 的 ghost
（`ghostOf` 浅拷贝保留 `paintAfterFn`）会带着上一帧的闭包进入当前帧的 paint。闭包里再调
`e.FocusVisible()` 等方法时 handle 已过期，开发模式直接 panic：
`ui: element from pass N used in pass M; keep a ui.Handle instead`。
补丁：将所有 paint 闭包内的 Element 方法调用（`FocusVisible`/`Focused`/`Hovered`/`IsDisabled`）
改为 build pass 读入局部变量，闭包只用变量。涉及 `internal/paint.go`（Ring）、
`input/`（checkbox、switch、text_input、radio_group、color_picker、resizable_panel_group、
slider、knob、vertical_slider、range_slider、tags_input、input_group、masked_input、mention_input）、
`layout/`（scroll_area、split_pane）、`navigation/`（tabs、navigation_menu）、
`code/`（terminal_tabs、flamegraph）、`media/video_scrubber`、`files/presence_avatars`、
`finance/leverage_slider`、`messaging/video_call_grid`。

### 重建副本

```sh
git clone --depth 1 --filter=blob:none --sparse https://git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI /tmp/mujica
cd /tmp/mujica && git sparse-checkout set --skip-checks --no-cone '/*' '!testdata/' '!examples/' '!site/'
cp -R /tmp/mujica/. third_party/MujicaUI/
rm -rf third_party/MujicaUI/.git
chmod -R u+w third_party/MujicaUI
# 然后按上文重新应用 highlight.go、text_input.go、text_area.go、prompt_composer.go、
# tabs.go、terminal_tabs.go 补丁，恢复 chart-column.svg，
# 并重做 paint 闭包 Element 方法调用补丁（见上文）
```

完成后 `go build ./...` 即可。若上游修复了上述问题，删除 `replace`、副本，
并把 import 改回直属依赖即可。

**`git/changes_list.go` + `git/branch_list.go` + `git/tag_list.go`** — 新增行级右键菜单。
`ChangesListOptions.RowMenu` / `BranchListOptions.RowMenu` / `TagListOptions.RowMenu`
接受 `func(c *ui.Context, item string)`，在行元素上挂 `overlay.ContextMenu`，
点击右键时回调当前行条目（文件路径 / 分支名 / 标签名）。
分支行与标签行改为在 `ui.Row` 中构建并返回该行元素，其余布局保持不变。
