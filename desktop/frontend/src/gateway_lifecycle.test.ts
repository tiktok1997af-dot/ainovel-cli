import { describe, expect, it } from 'vitest'

import {
  CONTRACT_VERSION,
  createGatewayClient,
  type AppError,
  type DesktopEvent,
  type GatewaySnapshotResult,
  type QueryRequest,
  type QueryResult,
  type CommandRequest,
  type CommandResult,
  type WailsDesktopBridge,
} from './gateway'

type ProjectLifecycleRequest = {
  project_root: string
}

type ProjectLifecycleResult = {
  contract_version: string
  project_root?: string
  data?: unknown
  error?: AppError
}

type ExpectedLifecycleClient = {
  createProject: (projectRoot: string) => Promise<ProjectLifecycleResult>
  openProject: (projectRoot: string) => Promise<ProjectLifecycleResult>
  switchProject: (projectRoot: string) => Promise<ProjectLifecycleResult>
  closeProject: () => Promise<ProjectLifecycleResult>
}

type LifecycleBridge = WailsDesktopBridge & {
  createProject: (request: ProjectLifecycleRequest) => Promise<ProjectLifecycleResult>
  openProject: (request: ProjectLifecycleRequest) => Promise<ProjectLifecycleResult>
  switchProject: (request: ProjectLifecycleRequest) => Promise<ProjectLifecycleResult>
  closeProject: () => Promise<ProjectLifecycleResult>
}

function lifecycleBridge() {
  const calls = {
    create: [] as ProjectLifecycleRequest[],
    open: [] as ProjectLifecycleRequest[],
    switch: [] as ProjectLifecycleRequest[],
    close: 0,
  }
  const result = (projectRoot?: string): ProjectLifecycleResult => ({
    contract_version: CONTRACT_VERSION,
    ...(projectRoot ? { project_root: projectRoot } : {}),
  })

  const bridge: LifecycleBridge = {
    async snapshot(): Promise<GatewaySnapshotResult> {
      return { contract_version: CONTRACT_VERSION }
    },
    async query(request: QueryRequest): Promise<QueryResult> {
      return { contract_version: CONTRACT_VERSION, kind: request.kind }
    },
    async dispatch(command: CommandRequest): Promise<CommandResult> {
      return { contract_version: CONTRACT_VERSION, command_id: command.id, accepted: true }
    },
    eventsOn(_topic: string, _callback: (event: DesktopEvent) => void) {
      return () => undefined
    },
    eventsEmit() {},
    async createProject(request) {
      calls.create.push(request)
      return result(request.project_root)
    },
    async openProject(request) {
      calls.open.push(request)
      return result(request.project_root)
    },
    async switchProject(request) {
      calls.switch.push(request)
      return result(request.project_root)
    },
    async closeProject() {
      calls.close += 1
      return result()
    },
  }

  return { bridge, calls }
}

describe('GUI-02C.4 typed project lifecycle client', () => {
  it('routes Create/Open/Switch/Close only through typed lifecycle bridge methods', async () => {
    const { bridge, calls } = lifecycleBridge()
    const client = createGatewayClient(bridge) as typeof createGatewayClient extends (...args: never[]) => infer Result
      ? Result & ExpectedLifecycleClient
      : never

    await client.createProject('C:/novels/new-project')
    await client.openProject('C:/novels/existing-project')
    await client.switchProject('C:/novels/next-project')
    await client.closeProject()

    expect(calls.create).toEqual([{ project_root: 'C:/novels/new-project' }])
    expect(calls.open).toEqual([{ project_root: 'C:/novels/existing-project' }])
    expect(calls.switch).toEqual([{ project_root: 'C:/novels/next-project' }])
    expect(calls.close).toBe(1)
    expect('invoke' in client).toBe(false)
  })
})
