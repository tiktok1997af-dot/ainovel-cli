import { describe, expect, it } from 'vitest'

import {
  CONTRACT_VERSION,
  DESKTOP_EVENT_TOPIC,
  createGatewayClient,
  verifyGatewayRoundTrip,
  type DesktopEvent,
  type WailsDesktopBridge,
} from './gateway'

function runtimeUnavailable() {
  return {
    code: 'runtime_unavailable',
    category: 'runtime',
    message: 'The application runtime is unavailable.',
    retryable: true,
  }
}

function fakeBridge(): WailsDesktopBridge & {
  calls: { snapshot: number; query: unknown[]; dispatch: unknown[]; eventTopics: string[]; emittedTopics: string[] }
} {
  let listener: ((event: DesktopEvent) => void) | undefined
  const calls = { snapshot: 0, query: [] as unknown[], dispatch: [] as unknown[], eventTopics: [] as string[], emittedTopics: [] as string[] }

  return {
    calls,
    async snapshot() {
      calls.snapshot += 1
      return { contract_version: CONTRACT_VERSION, error: runtimeUnavailable() }
    },
    async query(request) {
      calls.query.push(request)
      return { contract_version: CONTRACT_VERSION, kind: request.kind, error: runtimeUnavailable() }
    },
    async dispatch(command) {
      calls.dispatch.push(command)
      return {
        contract_version: CONTRACT_VERSION,
        command_id: command.id,
        accepted: false,
        error: runtimeUnavailable(),
      }
    },
    eventsOn(topic, callback) {
      calls.eventTopics.push(topic)
      listener = callback
      return () => {
        listener = undefined
      }
    },
    eventsEmit(topic, event) {
      calls.emittedTopics.push(topic)
      queueMicrotask(() => listener?.(event))
    },
  }
}

describe('GUI-02B.3 frontend gateway client', () => {
  it('injects the locked contract into Query and Dispatch without a generic invoke escape hatch', async () => {
    const bridge = fakeBridge()
    const client = createGatewayClient(bridge)

    await client.query('project.overview')
    await client.dispatch({ id: 'cmd-probe', kind: 'gui.verify.noop' })

    expect(bridge.calls.query).toEqual([
      { contract_version: CONTRACT_VERSION, kind: 'project.overview' },
    ])
    expect(bridge.calls.dispatch).toEqual([
      { contract_version: CONTRACT_VERSION, id: 'cmd-probe', kind: 'gui.verify.noop' },
    ])
    expect('invoke' in client).toBe(false)
  })

  it('uses exactly ainovel:desktop:event and returns a disposer', () => {
    const bridge = fakeBridge()
    const client = createGatewayClient(bridge)
    const dispose = client.subscribe(() => undefined)

    expect(bridge.calls.eventTopics).toEqual([DESKTOP_EVENT_TOPIC])
    expect(typeof dispose).toBe('function')
  })

  it('proves Snapshot, Query, Dispatch, and the Wails event bus in one bounded diagnostic round-trip', async () => {
    const bridge = fakeBridge()
    const client = createGatewayClient(bridge)

    const report = await verifyGatewayRoundTrip(client, { timeoutMs: 250 })

    expect(report.ok).toBe(true)
    expect(report.snapshot.contract_version).toBe(CONTRACT_VERSION)
    expect(report.query.contract_version).toBe(CONTRACT_VERSION)
    expect(report.dispatch.contract_version).toBe(CONTRACT_VERSION)
    expect(report.eventReceived).toBe(true)
    expect(bridge.calls.snapshot).toBe(1)
    expect(bridge.calls.eventTopics).toEqual([DESKTOP_EVENT_TOPIC])
    expect(bridge.calls.emittedTopics).toEqual([DESKTOP_EVENT_TOPIC])
  })
})
