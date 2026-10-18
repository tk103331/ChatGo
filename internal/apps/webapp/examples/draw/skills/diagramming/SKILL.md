---
name: diagramming
description: 在画图画布上组织架构图、流程图与思维导图
---

# 画图技能

调用 `draw_list_nodes` 先了解画布现状，再批量调用 `draw_add_node` 与 `draw_connect`。

- 节点 id 由画布生成，新增后用 `draw_list_nodes` 取回
- 先布局主干节点，再补分支和连线
- 完成后用 `draw_export` 取回 JSON 校验结构
