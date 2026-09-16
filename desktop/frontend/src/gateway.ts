export const CONTRACT_VERSION = 'ainovel.desktop.v1'
export const DESKTOP_EVENT_TOPIC = 'ainovel:desktop:event'

export type AppError = {
  code: string
  category: string
  message: string
  retryable?: boolean
}

export type GatewaySnapshotResult = {
  contract_version: string
  data?: unknown
  error?: AppError
}

export type QueryRequest = {
  contract_version?: string
  kind: string
  payload?: unknown
}

export type QueryResult = {
  contract_version: string
  kind: string
  data?: unknown
  error?: AppError
}

export type CommandRequest = {
  contract_version?: string
  id: string
  kind: string
  run_id?: string
  task_id?: string
  resource?: string
  payload?: unknown
}

export type CommandResult = {
  contract_version: string
  command_id: string
  accepted: boolean
  run_id?: string
  task_id?: string
  resource?: string
  status?: string
  data?: unknown
  error?: AppError
}

export type DesktopEvent = {
  contract_version: string
  seq?: number
  time: string
  category: string
  type: string
  level?: string
  run_id?: string
  task_id?: string
  summary?: string
  payload?: unknown
  error?: AppError
}

export type WailsDesktopBridge = {
  snapshot: () => Promise<GatewaySnapshotResult>
  query: (request: QueryRequest) => Promise<QueryResult>
  dispatch: (command: CommandRequest) => Promise<CommandResult>
  eventsOn: (topic: string, callback: (event: DesktopEvent) => void) => () => void
  eventsEmit: (topic: string, event: DesktopEvent) => void
}

export type GatewayClient = {
  snapshot: () => Promise<GatewaySnapshotResult>
  query: (kind: string, payload?: unknown) => Promise<QueryResult>
  dispatch: (command: Omit<CommandRequest, 'contract_version'>) => Promise<CommandResult>
  subscribe: (listener: (event: DesktopEvent) => void) => () => void
  emitVerificationProbe: (event: DesktopEvent) => void
}

export type GatewayRoundTripReport = {
  ok: boolean
  snapshot: GatewaySnapshotResult
  query: QueryResult
  dispatch: CommandResult
  eventReceived: boolean
  eventTopic: typeof DESKTOP_EVENT_TOPIC
}

type WailsWindow = Window & {
  go?: {
    main?: {
      Gateway?: {
        Snapshot: () => Promise<GatewaySnapshotResult>
        Query: (request: QueryRequest) => Promise<QueryResult>
        Dispatch: (command: CommandRequest) => Promise<CommandResult>
      }
    }
  }
  runtime?: {
    EventsOn: (topic: string, callback: (event: DesktopEvent) => void) => () => void
    EventsEmit: (topic: string, event: DesktopEvent) => void
  }
}

export function createGatewayClient(bridge: WailsDesktopBridge): GatewayClient {
  return {
    snapshot: () => bridge.snapshot(),
    query: (kind, payload) => {
      const request: QueryRequest = { contract_version: CONTRACT_VERSION, kind }
      if (payload !== undefined) request.payload = payload
      return bridge.query(request)
    },
    dispatch: (command) => bridge.dispatch({ contract_version: CONTRACT_VERSION, ...command }),
    subscribe: (listener) => bridge.eventsOn(DESKTOP_EVENT_TOPIC, listener),
    emitVerificationProbe: (event) => {
      if (
        event.contract_version !== CONTRACT_VERSION ||
        event.category !== 'DIAGNOSTIC' ||
        event.type !== 'gateway_verification_probe'
      ) {
        throw new Error('Only the bounded GUI-02B.3 verification event may be emitted by the frontend gateway client.')
      }
      bridge.eventsEmit(DESKTOP_EVENT_TOPIC, event)
    },
  }
}

export function createBrowserGatewayClient(): GatewayClient {
  const host = window as WailsWindow
  const gateway = host.go?.main?.Gateway
  const runtime = host.runtime
  if (!gateway || !runtime?.EventsOn || !runtime?.EventsEmit) {
    throw new Error('Wails gateway bridge is unavailable.')
  }

  return createGatewayClient({
    snapshot: () => gateway.Snapshot(),
    query: (request) => gateway.Query(request),
    dispatch: (command) => gateway.Dispatch(command),
    eventsOn: (topic, callback) => runtime.EventsOn(topic, callback),
    eventsEmit: (topic, event) => runtime.EventsEmit(topic, event),
  })
}

function hasTypedEnvelope(result: { contract_version: string; error?: AppError }): boolean {
  if (result.contract_version !== CONTRACT_VERSION) return false
  if (!result.error) return true
  return Boolean(result.error.code && result.error.category && result.error.message)
}

export async function verifyGatewayRoundTrip(
  client: GatewayClient,
  options: { timeoutMs?: number } = {},
): Promise<GatewayRoundTripReport> {
  const timeoutMs = options.timeoutMs ?? 1500
  const probeID = `gui02b3-${Date.now()}-${Math.random().toString(16).slice(2)}`
  let resolveEvent: ((received: boolean) => void) | undefined
  const eventPromise = new Promise<boolean>((resolve) => {
    resolveEvent = resolve
  })

  const dispose = client.subscribe((event) => {
    if (
      event.contract_version === CONTRACT_VERSION &&
      event.category === 'DIAGNOSTIC' &&
      event.type === 'gateway_verification_probe' &&
      event.summary === probeID
    ) {
      resolveEvent?.(true)
    }
  })

  try {
    const snapshot = await client.snapshot()
    const query = await client.query('project.overview')
    const dispatch = await client.dispatch({ id: probeID, kind: 'gui.verify.noop' })

    client.emitVerificationProbe({
      contract_version: CONTRACT_VERSION,
      time: new Date().toISOString(),
      category: 'DIAGNOSTIC',
      type: 'gateway_verification_probe',
      level: 'info',
      summary: probeID,
    })

    let timeoutHandle: ReturnType<typeof setTimeout> | undefined
    const eventReceived = await Promise.race([
      eventPromise,
      new Promise<boolean>((resolve) => {
        timeoutHandle = setTimeout(() => resolve(false), timeoutMs)
      }),
    ])
    if (timeoutHandle) clearTimeout(timeoutHandle)

    const ok =
      hasTypedEnvelope(snapshot) &&
      hasTypedEnvelope(query) &&
      query.kind === 'project.overview' &&
      hasTypedEnvelope(dispatch) &&
      dispatch.command_id === probeID &&
      eventReceived

    return {
      ok,
      snapshot,
      query,
      dispatch,
      eventReceived,
      eventTopic: DESKTOP_EVENT_TOPIC,
    }
  } finally {
    dispose()
  }
}
