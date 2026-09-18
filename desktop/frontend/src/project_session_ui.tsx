import { useCallback, useState } from 'react'

import { createBrowserGatewayClient, type AppError, type GatewayClient } from './gateway'
import {
  beginProjectLifecycle,
  initialProjectSessionState,
  settleProjectLifecycle,
  type ProjectLifecycleOperation,
  type ProjectSessionState,
} from './project_session'

const STATUS_LABEL: Record<ProjectSessionState['status'], string> = {
  'no-project': 'NO ACTIVE PROJECT',
  active: 'ACTIVE',
  busy: 'BUSY',
  error: 'ERROR',
}

export async function runProjectLifecycle(
  client: GatewayClient,
  state: ProjectSessionState,
  operation: ProjectLifecycleOperation,
  projectRootInput: string,
  onBusy?: (state: ProjectSessionState) => void,
): Promise<ProjectSessionState> {
  if (state.status === 'busy') return state

  const busy = beginProjectLifecycle(state, operation)
  onBusy?.(busy)

  try {
    const result = operation === 'create'
      ? await client.createProject(projectRootInput)
      : operation === 'open'
        ? await client.openProject(projectRootInput)
        : operation === 'switch'
          ? await client.switchProject(projectRootInput)
          : await client.closeProject()

    return settleProjectLifecycle(busy, operation, result)
  } catch (error) {
    const typedError: AppError = {
      code: 'gateway_unavailable',
      category: 'runtime',
      message: error instanceof Error ? error.message : 'Project lifecycle request failed.',
      retryable: true,
    }
    return {
      status: 'error',
      projectRoot: state.projectRoot,
      pendingOperation: null,
      error: typedError,
    }
  }
}

type ProjectSessionPanelProps = {
  state: ProjectSessionState
  projectRootInput: string
  onProjectRootInput: (value: string) => void
  onCreate: () => void
  onOpen: () => void
  onSwitch: () => void
  onClose: () => void
}

export function ProjectSessionPanel({
  state,
  projectRootInput,
  onProjectRootInput,
  onCreate,
  onOpen,
  onSwitch,
  onClose,
}: ProjectSessionPanelProps) {
  const busy = state.status === 'busy'
  const hasProject = state.projectRoot.length > 0

  return (
    <section className="panel runtime-card project-session" aria-label="Project Session">
      <div className="section-title">Project Session</div>
      <div className="status-row">
        <span>Session status</span>
        <span className="badge badge-idle">{STATUS_LABEL[state.status]}</span>
        <span className="muted">backend-authoritative lifecycle</span>
      </div>

      <label className="project-session-input">
        <span>Project root</span>
        <input
          aria-label="Project root"
          value={projectRootInput}
          onChange={(event) => onProjectRootInput(event.currentTarget.value)}
          disabled={busy}
          placeholder="C:/path/to/project"
        />
      </label>

      {hasProject ? <div className="runtime-foot gateway-proof">Active root: {state.projectRoot}</div> : null}
      {state.error ? (
        <div className="runtime-foot gateway-failure" role="alert">
          {state.error.code}: {state.error.message}
        </div>
      ) : null}

      <div className="project-session-actions">
        {hasProject ? (
          <>
            <button onClick={onSwitch} disabled={busy}>Switch</button>
            <button onClick={onClose} disabled={busy}>Close</button>
          </>
        ) : (
          <>
            <button onClick={onCreate} disabled={busy}>Create</button>
            <button onClick={onOpen} disabled={busy}>Open</button>
          </>
        )}
      </div>
    </section>
  )
}

export type ProjectSessionController = {
  state: ProjectSessionState
  projectRootInput: string
  setProjectRootInput: (value: string) => void
  run: (operation: ProjectLifecycleOperation) => Promise<void>
}

export function useProjectSessionController(): ProjectSessionController {
  const [state, setState] = useState<ProjectSessionState>(() => initialProjectSessionState())
  const [projectRootInput, setProjectRootInput] = useState('')

  const run = useCallback(async (operation: ProjectLifecycleOperation) => {
    if (state.status === 'busy') return

    const client = createBrowserGatewayClient()
    const next = await runProjectLifecycle(
      client,
      state,
      operation,
      projectRootInput,
      setState,
    )

    setState(next)
    setProjectRootInput(next.projectRoot)
  }, [projectRootInput, state])

  return {
    state,
    projectRootInput,
    setProjectRootInput,
    run,
  }
}

export function ProjectSession() {
  const controller = useProjectSessionController()

  return (
    <ProjectSessionPanel
      state={controller.state}
      projectRootInput={controller.projectRootInput}
      onProjectRootInput={controller.setProjectRootInput}
      onCreate={() => void controller.run('create')}
      onOpen={() => void controller.run('open')}
      onSwitch={() => void controller.run('switch')}
      onClose={() => void controller.run('close')}
    />
  )
}
