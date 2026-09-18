import type { AppError, ProjectLifecycleResult } from './gateway'

export type ProjectLifecycleOperation = 'create' | 'open' | 'switch' | 'close'
export type ProjectSessionStatus = 'no-project' | 'active' | 'busy' | 'error'

export type ProjectSessionState = {
  status: ProjectSessionStatus
  projectRoot: string
  pendingOperation: ProjectLifecycleOperation | null
  error: AppError | null
}

export function initialProjectSessionState(): ProjectSessionState {
  return {
    status: 'no-project',
    projectRoot: '',
    pendingOperation: null,
    error: null,
  }
}

export function beginProjectLifecycle(
  state: ProjectSessionState,
  operation: ProjectLifecycleOperation,
): ProjectSessionState {
  return {
    status: 'busy',
    projectRoot: state.projectRoot,
    pendingOperation: operation,
    error: null,
  }
}

export function settleProjectLifecycle(
  state: ProjectSessionState,
  operation: ProjectLifecycleOperation,
  result: ProjectLifecycleResult,
): ProjectSessionState {
  if (result.error) {
    return {
      status: 'error',
      projectRoot: state.projectRoot,
      pendingOperation: null,
      error: result.error,
    }
  }

  if (operation === 'close') {
    return initialProjectSessionState()
  }

  return {
    status: 'active',
    projectRoot: result.project_root ?? '',
    pendingOperation: null,
    error: null,
  }
}
