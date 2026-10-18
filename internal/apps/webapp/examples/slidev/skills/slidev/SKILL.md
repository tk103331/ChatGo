# Slidev 幻灯片编辑

本应用编辑 Slidev 风格的 Markdown 演示文稿：

- 用单独一行 `---` 分隔幻灯片；
- 文件开头的 `---...---` 是 frontmatter（主题、标题等），不是分隔符；
- 每页支持 Markdown 标题、列表、引用、代码块、链接与图片；
- 本应用只做轻量渲染，不支持 Slidev 的 Vue 组件、动画与布局指令。

Agent 可用的工具：`slidev_get_source`、`slidev_set_source`、`slidev_list_slides`、
`slidev_get_slide`、`slidev_add_slide`、`slidev_update_slide`、`slidev_delete_slide`、
`slidev_go_to`。页码从 1 开始；修改后页面会立即刷新并自动保存。
