# Media Tool — Field Notes design system

Verified implementation decisions · 2026-10-12 · Vue 3 + Element Plus

## Product and direction
A media automation **workbench**, not a streaming-service mockup. Preserve real
provider data, read-only service overview, save-time validation, dangerous-action
confirmations, secret redaction, and the compulsory password-reset boundary.

The ui-ux-pro-max `media automation dashboard` query matched operations / dark
media UI, technical typography, status colors and the prohibition on fabricated
telemetry. Adapted after reviewing the existing product: **dark evergreen
navigation, neutral light task surfaces, restrained green accents**. Full OLED,
terminal typography and a marketing landing-page layout are not appropriate to
long forms, filesystem tables and Chinese operational text. This documented
choice supersedes the raw generator recommendation.

No new fonts, CDN dependencies, illustration packages or motion libraries.
System sans-serif supports Chinese; system monospace is reserved for identifiers,
paths, byte sizes and shortcut hints. Media artwork must come from the existing
authorized image endpoints; no fake posters or media statistics.

## Tokens (source: web/src/style.css)
| Role | Value |
|---|---|
| Canvas | #f5f6f8 |
| Surface | #ffffff |
| Text | #202c38 |
| Secondary text | #626d7b |
| Accent / primary action | #146e63, white text |
| Accent wash | #eaf5f1 |
| Navigation | #19282d, #b9c8cc text |
| Borders | #e1e5e9 / #ccd4db |
| Danger | #b63c43 |
| Caution | #8b5e15 |
| Focus | 3px #146e63; light ring on dark navigation |

Spacing: 4 / 8 / 16 / 24 / 32 px. Surface radius 12px; control radius 8px.
Base 16px; operational body 13–14px; secondary metadata 11–12px. Page heading
24–30px, editorial overview heading 28–40px. No decorative chart/metric.

## Information architecture
- 媒体浏览: 总览, 媒体库
- 资源获取: 搜索, 订阅, PT站点, 下载与传输
- 文件与入库: 文件管理, 云盘索引, STRM管理, 整理历史
- 系统运维: 任务, 通知, 服务连接, 插件, 日志, 数据库, 设置

All existing routes and ownership rules remain. One canonical navigation model
powers the desktop menu, mobile drawer, page location and quick navigation.
Sidebar groups are native-button disclosures, initially **all collapsed**, with
unique controlled panel IDs. Their temporary state is shared across desktop and
mobile; navigating keeps the state, reloading clears it. The current group is
marked but is **never automatically expanded**. Hidden children are removed from
keyboard order and the accessibility tree.

Quick navigation is **not media search**: name/keyword filtering only, Ctrl/⌘ K,
arrow selection, Enter navigation, Escape dismissal and explicit empty results.

## Components and behavior
- Title, compact category eyebrow, plain-language description, then actions.
- One visually dominant primary action per operation; dangerous actions use
  outlined danger buttons and the existing confirmation/preview flow.
- White panels, neutral table headers, consistent status badges. Partial results
  and retries use caution; they must not be styled as completed.
- Runtime overview metrics are only the actual returned snapshot, with query
  time and scope. Missing data displays `—`, not a fabricated zero.
- Provider errors, loading and genuinely empty results remain distinct.
- Form inputs use visible labels; save success and validation stay near the form.
- Read-only data/log details use bounded, wrapping code surfaces.

## Responsive and accessibility contract
- Desktop sidebar 244px. Below 900px use an Element Plus modal drawer; the
  hidden desktop navigation is excluded from the accessibility tree.
- Drawer traps focus, supports Escape, prevents background interaction and
  restores the opener. Closing on route selection prevents hidden navigation.
- Skip link to focusable main; meaningful document titles; visible keyboard focus.
- Touch controls 44px minimum; compact table buttons expand to 44px on mobile.
- Dialog width is bounded by viewport. Tables scroll **inside** their container;
  page-level horizontal overflow is not hidden to disguise layout defects.
- Images reserve aspect ratio and load lazily; image failures use honest fallback.
- Only 160ms color/background transitions; honor reduced-motion including
  pseudo-elements. Do not animate width/height or add decorative GSAP.

## Verification
Retain existing API/behavior E2E coverage, and add grouped-navigation model unit
tests; command keyboard and empty-state tests; mobile focus-trap/return tests;
375 / 768 / 1024 / 1440 px overflow checks. Inspect actual screenshots before
release. Deployment follows GitHub → Actions AMD64 → Docker Hub → owned server.

## Native media realm · 2026-10-14

Add 自研媒体库 / 媒体用户 to the canonical browse group; `/watch` is a
separate member surface without administrator navigation. Retain existing
tokens and avoid pretending to be a completed video player. Mark unsupported
playback and progress clearly. New-user grants default to none, destructive
identity changes require confirmation; password fields allow paste/managers,
visibility toggles and have real labels. Scoped ui-ux-pro-max authentication
search verified these recommendations; no new form library is needed for the
small forms, and password values deliberately do not use `.trim`.

## Player UA admission · 2026-10-14

播放器访问 is an explicit browse-group page with a default-disabled admission
policy, real aggregate observations and exact-UA approval. Desktop uses a
bounded table; mobile uses cards with44px actions. Full UA remains available in
a wrapping readonly detail field. Rule removal, admission changes and cleanup
require distinct confirmation. Counts say UA admission, never playback success.
Scoped ui-ux-pro-max table/confirmation guidance verified this choice.
