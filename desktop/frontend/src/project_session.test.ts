import { describe, expect, it } from 'vitest'

import { CONTRACT_VERSION, type AppError, type ProjectLifecycleResult } from './gateway'
import {
  beginProjectLifecycle,
  initialProjectSessionState,
  settleProjectLifecycle,
} from './project_session'

const backendError: AppError = {
  code: 'command_not_allowed',
  category: 'command',
  message: 'The requested project lifecycle operation is not allowed.',
  retryable: false,
}

function success(projectRoot: string): ProjectLifecycleResult {
  return { contract_version: CONTRACT_VERSION, project_root: projectRoot, data: { project_root: projectRoot } }
}

describe('GUI-02C.4 project session state', () => {
  it('moves no-project through busy to active using only the backend-returned project root', () => {
    const initial = initialProjectSessionState()
    expect(initial).toEqual({
      status: 'no-project',
      projectRoot: '',
      pendingOperation: null,
      error: null,
    })

    const busy = beginProjectLifecycle(initial, 'open')
    expect(busy).toEqual({
      status: 'busy',
      projectRoot: '',
      pendingOperation: 'open',
      error: null,
    })

    const active = settleProjectLifecycle(busy, 'open', success('C:/canonical/from-backend'))
    expect(active).toEqual({
      status: 'active',
      projectRoot: 'C:/canonical/from-backend',
      pendingOperation: null,
      error: null,
    })
  })

  it('surfaces the typed backend error and does not claim the session is active after a failed switch', () => {
    const active = settleProjectLifecycle(
      beginProjectLifecycle(initialProjectSessionState(), 'open'),
      'open',
      success('C:/canonical/current'),
    )

    const busy = beginProjectLifecycle(active, 'switch')
    const failed = settleProjectLifecycle(busy, 'switch', {
      contract_version: CONTRACT_VERSION,
      error: backendError,
    })

    expect(failed.status).toBe('error')
    expect(failed.projectRoot).toBe('C:/canonical/current')
    expect(failed.pendingOperation).toBe(null)
    expect(failed.error).toEqual(backendError)
  })

  it('clears the project root only after a successful CloseProject result', () => {
    const active = settleProjectLifecycle(
      beginProjectLifecycle(initialProjectSessionState(), 'create'),
      'create',
      success('C:/canonical/new-project'),
    )

    const busy = beginProjectLifecycle(active, 'close')
    expect(busy.status).toBe('busy')
    expect(busy.projectRoot).toBe('C:/canonical/new-project')

    const closed = settleProjectLifecycle(busy, 'close', { contract_version: CONTRACT_VERSION })
    expect(closed).toEqual({
      status: 'no-project',
      projectRoot: '',
      pendingOperation: null,
      error: null,
    })
  })
})
