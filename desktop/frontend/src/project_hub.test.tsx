import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'

import App from './App'
import {
  CONTRACT_VERSION,
  type GatewayClient,
  type QueryResult,
} from './gateway'
import { initialProjectSessionState, type ProjectSessionState } from './project_session'
import {
  ProjectHubPanel,
  loadProjectOverview,
  type ProjectOverviewView,
} from './project_hub'

function gateway(overrides: Partial<GatewayClient> = {}): GatewayClient {
  const unavailable = async () => {
    throw new Error('unused S01 test gateway method')
  }

  return {
    snapshot: unavailable,
    query: unavailable,
    dispatch: unavailable,
    createProject: unavailable,
    openProject: unavailable,
    switchProject: unavailable,
    closeProject: unavailable,
    subscribe: () => () => undefined,
    emitVerificationProbe: () => undefined,
    ...overrides,
  }
}

const overview: ProjectOverviewView = {
  formatVersion: 2,
  title: 'Demo Project',
  synopsis: 'A bounded project overview.',
  premise: 'The canonical premise.',
  phase: 'planning',
  flow: 'drafting',
  layered: true,
  currentChapter: 12,
  completedChapters: 9,
  totalWordCount: 54321,
  currentVolume: 2,
  currentArc: 4,
}

function renderHub(
  session: ProjectSessionState,
  currentOverview: ProjectOverviewView | null,
) {
  return renderToStaticMarkup(
    <ProjectHubPanel
      session={session}
      projectRootInput="C:/user/input"
      overview={currentOverview}
      onProjectRootInput={() => undefined}
      onCreate={() => undefined}
      onOpen={() => undefined}
      onSwitch={() => undefined}
      onClose={() => undefined}
    />,
  )
}

describe('S01 Project Hub contract', () => {
  it('loads project.overview only through the typed Gateway query seam', async () => {
    const calls: Array<{ kind: string; payload?: unknown }> = []

    const client = gateway({
      query: async (kind, payload): Promise<QueryResult> => {
        calls.push({ kind, payload })
        return {
          contract_version: CONTRACT_VERSION,
          kind,
          data: {
            format_version: 2,
            title: 'Demo Project',
            synopsis: 'A bounded project overview.',
            premise: 'The canonical premise.',
            phase: 'planning',
            flow: 'drafting',
            layered: true,
            current_chapter: 12,
            completed_chapters: 9,
            total_word_count: 54321,
            current_volume: 2,
            current_arc: 4,
          },
        }
      },
    })

    const result = await loadProjectOverview(client)

    expect(calls).toEqual([{ kind: 'project.overview', payload: undefined }])
    expect(result).toEqual(overview)
  })

  it('renders bounded Create/Open state when there is no active project', () => {
    const html = renderHub(initialProjectSessionState(), null)

    expect(html).toContain('NO ACTIVE PROJECT')
    expect(html).toContain('Create')
    expect(html).toContain('Open')
    expect(html).not.toContain('>Switch<')
    expect(html).not.toContain('>Close<')
  })

  it('renders backend-authoritative active project metadata and Switch/Close', () => {
    const html = renderHub(
      {
        status: 'active',
        projectRoot: 'C:/canonical/project',
        pendingOperation: null,
        error: null,
      },
      overview,
    )

    expect(html).toContain('ACTIVE')
    expect(html).toContain('C:/canonical/project')
    expect(html).toContain('Demo Project')
    expect(html).toContain('planning')
    expect(html).toContain('drafting')
    expect(html).toContain('>Switch<')
    expect(html).toContain('>Close<')
    expect(html).not.toContain('>Create<')
    expect(html).not.toContain('>Open<')
  })

  it('does not ship the GUI-02A fake Recent Projects as S01 business data', () => {
    const html = renderToStaticMarkup(<App />)

    expect(html).not.toContain('Hứa An 2026')
    expect(html).not.toContain('Linh Dị 2026')
    expect(html).not.toContain('Zombie 2026')
  })
})
