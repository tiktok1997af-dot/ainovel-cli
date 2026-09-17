import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'

import { CONTRACT_VERSION, type AppError, type GatewayClient, type ProjectLifecycleResult } from './gateway'
import { initialProjectSessionState, type ProjectSessionState } from './project_session'
import { ProjectSessionPanel, runProjectLifecycle } from './project_session_ui'

const backendError: AppError = {
  code: 'project_open_failed',
  category: 'project',
  message: 'Backend refused the requested project path.',
  retryable: false,
}

function success(projectRoot: string): ProjectLifecycleResult {
  return {
    contract_version: CONTRACT_VERSION,
    project_root: projectRoot,
    data: { project_root: projectRoot },
  }
}

function client(overrides: Partial<GatewayClient> = {}): GatewayClient {
  const unavailable = async () => { throw new Error('unused in project session UI test') }
  return {
    snapshot: unavailable,
    query: unavailable,
    dispatch: unavailable,
    createProject: async (projectRoot) => success(projectRoot),
    openProject: async (projectRoot) => success(projectRoot),
    switchProject: async (projectRoot) => success(projectRoot),
    closeProject: async () => ({ contract_version: CONTRACT_VERSION }),
    subscribe: () => () => undefined,
    emitVerificationProbe: () => undefined,
    ...overrides,
  }
}

function render(state: ProjectSessionState, projectRootInput = 'C:/novels/demo') {
  return renderToStaticMarkup(
    <ProjectSessionPanel
      state={state}
      projectRootInput={projectRootInput}
      onProjectRootInput={() => undefined}
      onCreate={() => undefined}
      onOpen={() => undefined}
      onSwitch={() => undefined}
      onClose={() => undefined}
    />,
  )
}

describe('GUI-02C.4 Project Session UI contract', () => {
  it('renders project-root input, NO ACTIVE PROJECT, and only Create/Open before activation', () => {
    const html = render(initialProjectSessionState())
    expect(html).toContain('Project Session')
    expect(html).toContain('Project root')
    expect(html).toContain('NO ACTIVE PROJECT')
    expect(html).toContain('Create')
    expect(html).toContain('Open')
    expect(html).not.toContain('>Switch<')
    expect(html).not.toContain('>Close<')
  })

  it('renders ACTIVE with backend project root and exposes Switch/Close instead of Create/Open', () => {
    const html = render({
      status: 'active',
      projectRoot: 'C:/canonical/from-backend',
      pendingOperation: null,
      error: null,
    })
    expect(html).toContain('ACTIVE')
    expect(html).toContain('C:/canonical/from-backend')
    expect(html).toContain('>Switch<')
    expect(html).toContain('>Close<')
    expect(html).not.toContain('>Create<')
    expect(html).not.toContain('>Open<')
  })

  it('renders BUSY and disables lifecycle actions while an operation is pending', () => {
    const html = render({
      status: 'busy',
      projectRoot: 'C:/canonical/current',
      pendingOperation: 'switch',
      error: null,
    })
    expect(html).toContain('BUSY')
    expect(html).toContain('disabled=""')
  })

  it('renders typed backend ERROR without inventing an active project', () => {
    const html = render({
      status: 'error',
      projectRoot: '',
      pendingOperation: null,
      error: backendError,
    })
    expect(html).toContain('ERROR')
    expect(html).toContain('project_open_failed')
    expect(html).toContain('Backend refused the requested project path.')
    expect(html).not.toContain('ACTIVE')
  })

  it('uses the typed lifecycle client and trusts the successful backend project_root', async () => {
    const calls: string[] = []
    const next = await runProjectLifecycle(
      client({
        openProject: async (projectRoot) => {
          calls.push(projectRoot)
          return success('C:/canonical/backend-root')
        },
      }),
      initialProjectSessionState(),
      'open',
      'C:/user/input-root',
    )

    expect(calls).toEqual(['C:/user/input-root'])
    expect(next.status).toBe('active')
    expect(next.projectRoot).toBe('C:/canonical/backend-root')
  })

  it('preserves the current project root when a typed switch result returns an error', async () => {
    const active: ProjectSessionState = {
      status: 'active',
      projectRoot: 'C:/canonical/current',
      pendingOperation: null,
      error: null,
    }
    const next = await runProjectLifecycle(
      client({
        switchProject: async () => ({ contract_version: CONTRACT_VERSION, error: backendError }),
      }),
      active,
      'switch',
      'C:/user/replacement',
    )

    expect(next.status).toBe('error')
    expect(next.projectRoot).toBe('C:/canonical/current')
    expect(next.error).toEqual(backendError)
  })

  it('returns to no-project after a successful typed CloseProject result', async () => {
    const active: ProjectSessionState = {
      status: 'active',
      projectRoot: 'C:/canonical/current',
      pendingOperation: null,
      error: null,
    }
    const next = await runProjectLifecycle(client(), active, 'close', '')
    expect(next).toEqual(initialProjectSessionState())
  })
})
