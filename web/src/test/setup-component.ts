import { createRenderer, ssrContextKey, type Component } from 'vue'

// Exercise the real SFC setup/lifecycle without a browser or rendering unrelated children.
export function setupComponent(component: Component, props: Record<string, unknown> = {}) {
  const renderer = createRenderer<object, object>({
    createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
    insert() {}, remove() {}, setText() {}, setElementText() {}, patchProp() {},
    parentNode: () => null, nextSibling: () => null,
  })
  const app = renderer.createApp({ ...component, render: () => null }, props)
  app.provide(ssrContextKey, { modules: new Set() })
  const instance = app.mount({})
  return { state: (instance.$ as unknown as { setupState: Record<string, any> }).setupState, unmount: () => app.unmount() }
}

export function deferred<T = any>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

export async function flush() {
  for (let i = 0; i < 20; i++) await Promise.resolve()
}
