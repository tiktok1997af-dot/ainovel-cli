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

// GUI-02C.4 behavioral RED seam: these transitions are intentionally not
// implemented yet. The tests must reach assertions before production behavior
// is added.
export function beginProjectLifecycle(
  state: ProjectSessionState,
  _operation: ProjectLifecycleOperation,
): ProjectSessionState {
  return state
}

export function settleProjectLifecycle(
  state: ProjectSessionState,
  _operation: ProjectLifecycleOperation,
  _result: ProjectLifecycleResult,
): ProjectSessionState {
  return state
}
