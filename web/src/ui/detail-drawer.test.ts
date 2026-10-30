import {readFileSync} from 'node:fs';
import { expect, it } from "vitest";
import { createSSRApp, defineComponent, h } from "vue";
import { renderToString } from "vue/server-renderer";
import DetailDrawer from "./DetailDrawer.vue";

const DialogStub = defineComponent({
  props: {
    title: String,
    width: String,
    closeOnClickModal: Boolean,
    closeOnPressEscape: Boolean,
    showClose: Boolean,
  },
  setup(props, { slots }) {
    return () =>
      h(
        "section",
        {
          class: "dialog-stub",
          "data-title": props.title,
          "data-width": props.width,
          "data-close-on-click-modal": String(props.closeOnClickModal),
          "data-close-on-press-escape": String(props.closeOnPressEscape),
          "data-show-close": String(props.showClose),
        },
        [
          h("header", props.title),
          h("main", slots.default?.()),
          slots.footer ? h("footer", slots.footer()) : null,
        ],
      );
  },
});
const ButtonStub = defineComponent({
  setup:
    (_, { slots, attrs }) =>
    () =>
      h("button", attrs, slots.default?.()),
});

async function render(
  props: Record<string, unknown> = {},
  slots: Record<string, () => unknown> = {},
) {
  const app = createSSRApp({
    render: () => h(DetailDrawer, { title: "详情", ...props }, slots),
  });
  app.component("el-dialog", DialogStub);
  app.component("el-button", ButtonStub);
  app.component(
    "el-option",
    defineComponent({ setup: () => () => h("option") }),
  );
  return renderToString(app);
}

it.each([
  ["small", "560px"],
  ["default", "800px"],
  ["large", "1000px"],
] as const)("uses the %s dialog width", async (size, width) => {
  const html = await render({ size });
  expect(html).toContain(`data-width="${width}"`);
});

it("keeps modal clicks inert, allows Escape, and has one dialog close affordance", async () => {
  const html = await render();
  expect(html).toContain('data-close-on-click-modal="false"');
  expect(html).toContain('data-close-on-press-escape="true"');
  expect(html).toContain('data-show-close="true"');
  expect(html).not.toContain(">关闭<");
  expect(html).not.toContain("<footer");
});

it("renders a supplied footer only when the caller supplies one", async () => {
  const html = await render({}, { footer: () => h("button", "保存") });
  expect(html).toContain("<footer");
  expect(html).toContain("保存");
});

it("keeps supplied footer actions anchored to the full footer row", () => {
  const source=readFileSync(new URL('./DetailDrawer.vue',import.meta.url),'utf8');
  expect(source).toMatch(/\.detail-dialog \.detail-footer\s*\{[^}]*width:\s*100%/);
});

it('renders business slots without a technical section or a forced minimum content height',async()=>{
 const html=await render({}, {default:()=>h('p','可读业务说明')});expect(html).toContain('可读业务说明');expect(html).not.toMatch(/technical|<pre/);
 const source=readFileSync(new URL('./DetailDrawer.vue',import.meta.url),'utf8');expect(source).not.toMatch(/technicalData|showTechnical|min-height: min\(360px/);
});
