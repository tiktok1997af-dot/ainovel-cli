import { CONTRACT_VERSION, type GatewayClient } from './gateway'
import type { ProjectSessionState } from './project_session'
import { ProjectSessionPanel } from './project_session_ui'

export type ProjectOverviewView = {
  formatVersion: number
  title: string
  synopsis: string
  premise: string
  phase: string
  flow: string
  layered: boolean
  currentChapter: number
  completedChapters: number
  totalWordCount: number
  currentVolume: number
  currentArc: number
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function stringField(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function numberField(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}

function booleanField(value: unknown): boolean {
  return value === true
}

export async function loadProjectOverview(client: GatewayClient): Promise<ProjectOverviewView> {
  const result = await client.query('project.overview')

  if (result.contract_version !== CONTRACT_VERSION) {
    throw new Error('Project overview contract version mismatch.')
  }

  if (result.error) {
    throw new Error(`${result.error.code}: ${result.error.message}`)
  }

  if (!isRecord(result.data)) {
    throw new Error('Project overview response is missing typed data.')
  }

  return {
    formatVersion: numberField(result.data.format_version),
    title: stringField(result.data.title),
    synopsis: stringField(result.data.synopsis),
    premise: stringField(result.data.premise),
    phase: stringField(result.data.phase),
    flow: stringField(result.data.flow),
    layered: booleanField(result.data.layered),
    currentChapter: numberField(result.data.current_chapter),
    completedChapters: numberField(result.data.completed_chapters),
    totalWordCount: numberField(result.data.total_word_count),
    currentVolume: numberField(result.data.current_volume),
    currentArc: numberField(result.data.current_arc),
  }
}

type ProjectHubPanelProps = {
  session: ProjectSessionState
  projectRootInput: string
  overview: ProjectOverviewView | null
  overviewError?: string
  onProjectRootInput: (value: string) => void
  onCreate: () => void
  onOpen: () => void
  onSwitch: () => void
  onClose: () => void
}

export function ProjectHubPanel({
  session,
  projectRootInput,
  overview,
  overviewError = '',
  onProjectRootInput,
  onCreate,
  onOpen,
  onSwitch,
  onClose,
}: ProjectHubPanelProps) {
  const hasProject = session.projectRoot.length > 0

  return (
    <section className="project-hub" aria-label="S01 Project Hub">
      <ProjectSessionPanel
        state={session}
        projectRootInput={projectRootInput}
        onProjectRootInput={onProjectRootInput}
        onCreate={onCreate}
        onOpen={onOpen}
        onSwitch={onSwitch}
        onClose={onClose}
      />

      {hasProject ? (
        <section className="panel runtime-card" aria-label="Active Project Summary">
          <div className="section-title">Active Project</div>

          {overview ? (
            <>
              <div className="status-row">
                <span>Title</span>
                <strong>{overview.title || 'Untitled project'}</strong>
              </div>

              <div className="status-row">
                <span>Phase</span>
                <span>{overview.phase || '—'}</span>
                <span className="muted">Flow: {overview.flow || '—'}</span>
              </div>

              <div className="status-row">
                <span>Progress</span>
                <span>Chapter {overview.currentChapter}</span>
                <span className="muted">
                  {overview.completedChapters} completed · {overview.totalWordCount} words
                </span>
              </div>

              {overview.synopsis ? (
                <div className="runtime-foot">{overview.synopsis}</div>
              ) : null}

              {overview.premise ? (
                <div className="runtime-foot muted">{overview.premise}</div>
              ) : null}
            </>
          ) : (
            <div className="runtime-foot muted">Loading project overview…</div>
          )}

          {overviewError ? (
            <div className="runtime-foot gateway-failure" role="alert">
              {overviewError}
            </div>
          ) : null}
        </section>
      ) : null}
    </section>
  )
}
