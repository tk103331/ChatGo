# third_party

## MujicaUI（本地补丁副本）

`MujicaUI/` 是上游模块 `git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI` 的本地副本，
通过根目录 `go.mod` 的 `replace` 使用：

```
replace git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI => ./third_party/MujicaUI
```

该目录未纳入版本控制（见 `.gitignore`），因为它是整模块副本（约 25 MB）。

### 为什么需要它

上游 `internal/highlight.go` 的 `Highlight` 存在**死循环**：判定用 `rune(rest[0])`（单字节），
但多字节 UTF-8 的首字节（如全角标点 `：` 的首字节 `0xEF`）会被 `unicode.IsLetter` 误判为字母，
随后 `spanLen` 按真 rune 迭代立即返回 `0`，导致 `i += 0` 永不前进，主线程 100% CPU 卡死。

触发条件：Markdown 代码块中出现**以全角标点开头的行**（中文冒号、逗号、句号等）。
表现：打开含此类消息的会话即卡死。

### 补丁

`internal/highlight.go` 中：循环开头用 `utf8.DecodeRuneInString` 取首个 rune 并记录其字节数，
所有分支改用该 rune 判定、按字节数推进，并保证每个分支至少前进 `size` 字节（`n <= 0` 时兜底为 `size`）。

`chat/prompt_composer.go` 中新增 `PromptComposerOptions.Box`：置 true 时去掉字段自身的
边框与背景（`area.Element.BorderWidth(0,0,0,0).Background(k.Surface)`），
好让调用方把输入框与其工具栏包进**同一个框**（ChatGo 的输入框用此选项，避免"框内套框"）。

`navigation/tabs.go` 中：水平模式下 panel 也 `Grow(1).MinHeight(0)`。上游只在 `Vertical` 时
给 panel `Grow(1)`，水平模式下 panel 高度由内容决定，而 ChatGo 的设置页 panel 内是
`Scroll.Grow(1).MinHeight(0)`，父级无 Grow 时高度坍缩为 0，导致整个 tab 内容区空白。

`code/terminal_tabs.go` 中新增 `TerminalTabsOptions.Actions`：置非 nil 时在标签条末尾
（`TabsBase.List` 之后）渲染额外控件，好让 ChatGo 把"新建终端/关闭终端"按钮放进标签条内，
而不是单独占一列被推到面板边缘。

### 重建副本

```sh
go get git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI@<version>
MOD=$(go env GOMODCACHE)/git.zacharyzhang.com/\!zachary\!zhang-\!n\!y/\!mujica\!u\!i@<version>
mkdir -p third_party/MujicaUI
cp -R "$MOD"/. third_party/MujicaUI/
chmod -R u+w third_party/MujicaUI
# 然后按上文重新应用 highlight.go、prompt_composer.go、tabs.go 与 terminal_tabs.go 补丁
```

完成后 `go build ./...` 即可。若上游修复了该 bug，删除 `replace`、副本，并把 import 改回直属依赖即可。
