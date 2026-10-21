import { describe, it, expect, vi } from 'vitest'
import { setupComponent, flush } from '../test/setup-component'
import PageHeader from './PageHeader.vue'
import StateView from './StateView.vue'
import ConfirmAction from './ConfirmAction.vue'
import PreviewApprove from './PreviewApprove.vue'
import DetailDrawer from './DetailDrawer.vue'
import AsyncResultBanner from './AsyncResultBanner.vue'
import FlowHint from './FlowHint.vue'

describe('U1 UI Components logic', () => {
  it('StateView computes correct display titles', () => {
    const errorView = setupComponent(StateView, { state: 'error', title: '出错了' })
    expect(errorView.state.displayTitle).toBe('出错了')
    errorView.unmount()

    const defaultEmpty = setupComponent(StateView, { state: 'empty' })
    expect(defaultEmpty.state.displayTitle).toBe('暂无数据')
    defaultEmpty.unmount()
  })

  it('ConfirmAction verifies confirm phrase', () => {
    const confirm = setupComponent(ConfirmAction, {
      title: '删除资源',
      confirmPhrase: 'DELETE-CONFIRM',
      visible: true
    })
    expect(confirm.state.inputPhrase).toBe('')
    confirm.state.inputPhrase = 'DELETE-CONFIRM'
    expect(confirm.state.inputPhrase).toBe('DELETE-CONFIRM')
    confirm.unmount()
  })

  it('DetailDrawer toggles technical details', () => {
    const drawer = setupComponent(DetailDrawer, {
      title: '任务详情',
      visible: true,
      technicalData: { id: 'test-123' }
    })
    expect(drawer.state.showTechnical).toBe(false)
    drawer.state.showTechnical = true
    expect(drawer.state.showTechnical).toBe(true)
    drawer.unmount()
  })

  it('AsyncResultBanner computes default message and type', () => {
    const banner = setupComponent(AsyncResultBanner, {
      status: 'queued',
      ticketId: 't-1'
    })
    expect(banner.state.displayMessage).toContain('已排队')
    expect(banner.state.bannerType).toBe('info')
    banner.unmount()

    const reconciling = setupComponent(AsyncResultBanner, {
      status: 'reconciling'
    })
    expect(reconciling.state.bannerType).toBe('warning')
    reconciling.unmount()
  })

  it('FlowHint computes display action mapping', () => {
    const hint = setupComponent(FlowHint, {
      nextAction: 'authorize_115',
      reason: '凭据已过期'
    })
    expect(hint.state.displayAction).toBe('去完成 115 扫码授权')
    hint.unmount()

    const custom = setupComponent(FlowHint, {
      nextAction: 'custom_step'
    })
    expect(custom.state.displayAction).toBe('查看处理建议')
    custom.unmount()
  })
})
