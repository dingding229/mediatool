<script setup lang="ts">
defineProps<{
  title: string;
  description: string;
}>();
</script>
<template>
  <main class="auth-shell">
    <aside class="auth-story" aria-label="媒体工作台">
      <RouterLink class="auth-brand" to="/login">
        <span aria-hidden="true">M</span>
        Media Tool
      </RouterLink>
      <div class="auth-statement">
        <p class="auth-eyebrow">你的媒体，各归其位</p>
        <h1>
          从发现好作品，
          <br>
          到随时点开看。
        </h1>
        <p>
          把搜索、订阅、入库和播放放在一处，
          <br>
          每一步都清楚，每一次都可追踪。
        </p>
        <ol class="auth-path" aria-label="媒体流程">
          <li>发现</li>
          <li>订阅</li>
          <li>入库</li>
          <li>播放</li>
        </ol>
      </div>
      <small>媒体自动化 · 由你掌控</small>
    </aside>
    <section class="auth-entry">
      <div class="auth-form">

        <h2>{{title}}</h2>
        <p class="auth-description">{{description}}</p>
        <slot/>
      </div>
    </section>
  </main>
</template>
<style scoped>
.auth-shell{min-height:100dvh;display:grid;grid-template-columns:minmax(0,1.05fr) minmax(0,1fr);background:var(--mt-bg-surface);color:var(--mt-text-primary)}.auth-story{display:flex;flex-direction:column;justify-content:space-between;padding:48px 64px;background:var(--mt-color-primary-light);border-right:1px solid var(--mt-border-color);gap:48px}.auth-brand{color:var(--mt-text-primary);font-weight:700;text-decoration:none;display:flex;align-items:center;gap:12px}.auth-brand span{display:grid;place-items:center;width:40px;height:40px;border:1px solid var(--mt-color-primary);border-radius:12px;color:var(--mt-color-primary);font-size:22px}.auth-eyebrow{font-size:12px;letter-spacing:.08em;color:var(--mt-color-primary);font-weight:600;margin:0 0 16px}.auth-statement h1{font-size:clamp(30px,3.6vw,52px);line-height:1.4;letter-spacing:-.04em;margin:0 0 24px}.auth-statement>p:not(.auth-eyebrow){color:var(--mt-text-muted);line-height:1.9;font-size:14px}.auth-path{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));list-style:none;padding:0;margin:40px 0 0;gap:10px}.auth-path li{border-top:2px solid var(--mt-color-primary);padding-top:12px;color:var(--mt-text-muted);font-size:13px}.auth-story>small{color:var(--mt-text-muted);font-size:12px}.auth-entry{display:grid;align-items:center;padding:56px 48px;min-width:0}.auth-form{width:100%;max-width:420px;margin:auto;min-width:0}.auth-form h2{font-size:28px;margin:0 0 16px;letter-spacing:-.02em}.auth-description{color:var(--mt-text-muted);line-height:1.8;font-size:14px;margin-bottom:32px}.auth-form :deep(form){display:grid;gap:16px}.auth-form :deep(label){font-size:14px;display:block;margin-bottom:8px}.auth-form :deep(.el-input__wrapper){min-height:48px;box-sizing:border-box}.auth-form :deep(.el-button){min-height:44px;white-space:normal;height:auto;padding:12px}.auth-form :deep(form>.el-button){width:100%;margin:8px 0 0}.auth-form :deep(.field-hint),.auth-form :deep(small){font-size:12px;color:var(--mt-text-muted);line-height:1.8;margin:8px 0 0}.auth-form :deep(.auth-note){border-top:1px solid var(--mt-border-color);padding-top:20px;margin-top:28px;font-size:12px;color:var(--mt-text-muted);line-height:1.8}.auth-form :deep(.el-alert){margin-bottom:20px}.auth-form :deep(.auth-error){color:var(--mt-color-danger)}
@media(max-width:760px){.auth-shell{grid-template-columns:minmax(0,1fr)}.auth-story{padding:24px;border-right:0;border-bottom:1px solid var(--mt-border-color);gap:24px}.auth-statement h1{font-size:27px;margin-bottom:12px}.auth-statement>p:not(.auth-eyebrow),.auth-path,.auth-story>small{display:none}.auth-entry{padding:32px 24px 48px}.auth-form h2{font-size:24px}.auth-description{margin-bottom:24px}.auth-statement .auth-eyebrow{margin-bottom:8px}}
</style>
