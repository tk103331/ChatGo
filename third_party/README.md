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

### 重建副本

```sh
go get git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI@<version>
MOD=$(go env GOMODCACHE)/git.zacharyzhang.com/\!zachary\!zhang-\!n\!y/\!mujica\!u\!i@<version>
mkdir -p third_party/MujicaUI
cp -R "$MOD"/. third_party/MujicaUI/
chmod -R u+w third_party/MujicaUI
# 然后按上文重新应用 highlight.go 与 prompt_composer.go 补丁
```

完成后 `go build ./...` 即可。若上游修复了该 bug，删除 `replace`、副本，并把 import 改回直属依赖即可。
