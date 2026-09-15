# DESIGN.md — 应用区细节统一规范

适用于 `web/` 下除落地页（`features/home`，已按深色科技风单独设计）以外的所有界面：
控制台、模型广场、用量日志、钱包、个人设置、系统设置、管理端各表格页。

**范围**：保留现有亮/暗双主题与调色板（`:root` / `.dark` 两套 token 已在
`web/src/styles/theme.css` 定义），只统一细节。**不换色板、不改信息架构、不改功能与文案。**

---

## 1. 圆角（三档，不许再引入新值）

| 角色 | 用什么 | 典型场景 |
|---|---|---|
| 容器 | `rounded-xl` | 卡片、面板、弹窗、区块包装 |
| 控件 | `rounded-md` | 按钮、输入框、下拉、标签页 |
| 胶囊 | `rounded-full` | 状态点、徽标、头像、进度条 |

**要消除**：裸 `rounded`（99 处）、`rounded-sm`（35）、`rounded-lg`（279）、
`rounded-2xl`（28）、`rounded-3xl`。

映射规则：
- `rounded` / `rounded-sm` → 控件用 `rounded-md`；胶囊类用 `rounded-full`
- `rounded-lg` → 按钮/输入等控件→`rounded-md`；卡片/面板→`rounded-xl`
- `rounded-2xl` / `rounded-3xl` → `rounded-xl`

> 单边圆角（`rounded-t-*` 等）保留，但取值同样收敛到上面的档位。

## 2. 间距节奏

| 位置 | 值 |
|---|---|
| 页面左右留白 | `px-4 sm:px-6` |
| 页面顶部留白 | `py-4 sm:py-6` |
| 区块之间 | `space-y-4`（紧凑）/ `space-y-6`（宽松，仅一级分区） |
| 卡片内边距 | `p-4`（默认）/ `p-5`（内容型面板）/ `p-3`（紧凑列表项） |
| 表格单元格 | `px-4 py-3`（默认）/ `px-3 py-2`（紧凑模式） |
| 表单字段之间 | `space-y-4` |
| 图标与文字 | `gap-2`（默认）/ `gap-1.5`（紧凑） |

**要消除**：`p-8`、`p-6`（除非整页空状态容器）、`gap-3` 与 `gap-2` 混用于同一层级的
兄弟元素、同一列表里 `py-2.5`/`py-3`/`py-3.5` 混用。

## 3. 字阶（禁止任意 px）

| 角色 | 类 | 说明 |
|---|---|---|
| 页面标题 | `text-lg font-semibold` | 每页一个 |
| 区块标题 | `text-sm font-semibold` | |
| 正文 | `text-sm` | 默认 |
| 次要信息 | `text-xs text-muted-foreground` | |
| 数字/代码 | `text-xs font-mono tabular-nums` | 表格数字列 |

**要消除**：`text-[9px]`（4）、`text-[10px]`（83）、`text-[11px]`（89）、
`text-[13px]`（15）、`text-[15px]` 等一切任意 px 字号。

映射：`text-[13px]`/`text-[15px]` → `text-sm`；`text-[11px]`/`text-[10px]`/`text-[9px]` → `text-xs`。

> 允许例外：`[data-slot='table']` 现有的 13px 表格字号覆盖（见
> `usage-logs-table.tsx`），保持原样不动。

## 4. 状态与反馈

- **加载**：统一用现有骨架组件；不要在骨架外层再加 `animate-pulse` 或自定义 shimmer。
- **空状态**：统一形态 —— 图标（`size-4`，`text-muted-foreground`）+ 一行主文案（`text-sm`）
  + 可选一行次要说明（`text-xs text-muted-foreground`），垂直居中、`py-10`。
- **错误**：就地展示在出错的字段/区块旁，`text-xs text-destructive`，不要只用一个顶部 toast。
- **状态徽标**：统一 `rounded-full px-2 py-0.5 text-xs font-mono uppercase tracking-wide`；
  颜色只用语义 token（`--success` / `--warning` / `--destructive` / `--muted`），
  不许出现 `bg-green-500` 这类硬编码色。

## 5. 焦点与可访问性（硬性）

- 焦点环统一用 `focus-visible:ring-ring`，**不得**移除（`focus:outline-none` 后必须补 ring）。
- 图标按钮必须有可访问名：`aria-label` 或可见文字。
- 正文对比度 ≥ 4.5:1；`text-muted-foreground` 不要再用透明度继续削弱
  （禁止 `text-muted-foreground/55` 这类叠加）。
- 可点击目标 ≥ 36×36px（`size-9`），密集表格内的行操作菜单例外但不得小于 `size-7`。

## 6. 表格统一

- 表头：`text-xs font-medium text-muted-foreground`，背景透明，下边框 `border-b`。
- 行高：默认 `py-3`，紧凑模式 `py-2`；同一个表里不许两种。
- 悬停：`hover:bg-muted/40`。
- 数字列右对齐 + `tabular-nums`；时间列统一用同一个格式化函数。
- 分页与批量操作栏：沿用 `components/data-table/` 现有实现，只做上面的间距/字号收敛。

## 7. 暗色一致性

每一处新写的颜色都必须走语义 token。逐条检查：
- 硬编码 `bg-white` / `bg-black` / `text-white` → 改语义 token
- `dark:` 变体里出现硬编码色（如 `dark:bg-slate-900`）→ 改语义 token
- 半透明叠加（`/50`、`/20`）超过一层的，收敛到一层

---

## 不许做的事

- 不改任何文案、字段含义、请求参数、路由。
- 不删除功能，不为「更简洁」合并不同的交互。
- 不动 `features/home/**`（落地页自成一体）。
- 不动 `styles/theme.css` 与 `styles/theme-presets.css` 里的 token 定义
  （除非是补一个缺失的语义 token，且两个主题都补）。
- 不引入新的依赖或新的全局 CSS 类。
