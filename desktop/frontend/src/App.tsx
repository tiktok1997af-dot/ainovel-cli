import { useCallback, useEffect, useMemo, useState } from 'react'

import {
  createBrowserGatewayClient,
  setDesktopWindowTitle,
  verifyGatewayRoundTrip,
  type GatewayRoundTripReport,
} from './gateway'
import {
  ProjectSessionPanel,
  useProjectSessionController,
  type ProjectSessionController,
} from './project_session_ui'
import {
  ProjectHubPanel,
  loadProjectOverview,
  type ProjectOverviewView,
} from './project_hub'

type Screen = {
  id: string
  label: string
  route: string
  hint: string
}

const screens: Screen[] = [
  { id: 'S01', label: 'Projects', route: '/projects', hint: 'Project catalog and selection' },
  { id: 'S02', label: 'Project Home', route: '/project', hint: 'Project overview and progress' },
  { id: 'S03', label: 'Story Bible', route: '/story-bible', hint: 'Characters, world, canon and references' },
  { id: 'S04', label: 'Architecture', route: '/architecture', hint: 'Book, saga, arc and chapter architecture' },
  { id: 'S05', label: 'Chapter Studio', route: '/chapter-studio', hint: 'Primary writing workspace' },
  { id: 'S06', label: 'Review & Repair', route: '/review-repair', hint: '12-gate review and bounded repair' },
  { id: 'S07', label: 'Continuity Center', route: '/continuity', hint: 'Continuity evidence and canon locks' },
  { id: 'S08', label: 'Run Control', route: '/run-control', hint: 'Queue, runs, checkpoints and lifecycle' },
  { id: 'S09', label: 'AI Models', route: '/ai-models', hint: 'Web model readiness and role projection' },
  { id: 'S10', label: 'Settings', route: '/settings', hint: 'Runtime and desktop settings' },
]

const chapterCopy = [
  'Trăng treo cao trên rặng núi, ánh bạc phủ qua mặt hồ và những mái nhà im lặng. Hứa An đứng bên cửa sổ, mắt dõi về phía rừng tối ở ngoài trấn.',
  'Một tiếng động khẽ vang lên rồi tắt. Không đủ lớn để đánh thức cả căn phòng, nhưng đủ khiến cậu ngừng tay và lắng nghe.',
  'Những manh mối cũ vẫn chưa khép lại. Chương tiếp theo sẽ phải nối đúng continuity, canon locks và checkpoint đã được xác nhận trước đó.',
]

function Badge({ children, tone = 'neutral' }: { children: React.ReactNode; tone?: 'neutral' | 'ready' | 'idle' }) {
  return <span className={`badge badge-${tone}`}>{children}</span>
}

function gatewayCode(result: { error?: { code: string } }) {
  return result.error?.code ?? 'ok'
}

function GatewayDiagnostics() {
  const [state, setState] = useState<'idle' | 'running' | 'pass' | 'fail'>('idle')
  const [report, setReport] = useState<GatewayRoundTripReport | null>(null)
  const [failure, setFailure] = useState('')

  const runVerification = useCallback(async () => {
    if (state === 'running') return
    setState('running')
    setFailure('')
    setReport(null)
    setDesktopWindowTitle('AINOVEL Desktop · GATEWAY CHECK')

    try {
      const client = createBrowserGatewayClient()
      const nextReport = await verifyGatewayRoundTrip(client)
      setReport(nextReport)
      if (!nextReport.ok) {
        throw new Error('Typed gateway round-trip did not satisfy all verification seams.')
      }
      setState('pass')
      setDesktopWindowTitle('AINOVEL Desktop · GATEWAY VERIFIED')
    } catch (error) {
      setState('fail')
      setFailure(error instanceof Error ? error.message : 'Gateway verification failed.')
      setDesktopWindowTitle('AINOVEL Desktop · GATEWAY FAILED')
    }
  }, [state])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.ctrlKey && event.shiftKey && event.key.toLowerCase() === 'g') {
        event.preventDefault()
        void runVerification()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [runVerification])

  const label = state === 'pass' ? 'VERIFIED' : state === 'running' ? 'CHECKING' : state === 'fail' ? 'FAILED' : 'READY TO VERIFY'
  const tone = state === 'pass' ? 'ready' : 'idle'

  return (
    <div className="gateway-diagnostic" aria-label="GUI-02B.3 gateway diagnostic">
      <div className="status-row">
        <span>Wails Gateway</span>
        <Badge tone={tone}>{label}</Badge>
        <button onClick={() => void runVerification()} disabled={state === 'running'}>Verify Gateway</button>
      </div>
      {report ? (
        <div className="runtime-foot gateway-proof">
          Snapshot {gatewayCode(report.snapshot)} · Query {gatewayCode(report.query)} · Dispatch {gatewayCode(report.dispatch)} · Event {report.eventReceived ? 'PASS' : 'FAIL'}
        </div>
      ) : (
        <div className="runtime-foot muted">Ctrl+Shift+G · typed gateway remains the only frontend authority seam</div>
      )}
      {failure ? <div className="runtime-foot gateway-failure">{failure}</div> : null}
    </div>
  )
}

function ChapterStudio({ controller }: { controller: ProjectSessionController }) {
  const { state, projectRootInput, setProjectRootInput, run } = controller

  return (
    <>
      <ProjectSessionPanel
        state={state}
        projectRootInput={projectRootInput}
        onProjectRootInput={setProjectRootInput}
        onCreate={() => void run('create')}
        onOpen={() => void run('open')}
        onSwitch={() => void run('switch')}
        onClose={() => void run('close')}
      />

      <section className="workspace-grid" aria-label="Chapter Studio shell">
        <article className="panel editor-panel">
          <header className="panel-header editor-heading">
            <div>
              <div className="eyebrow">Chapter Studio</div>
              <h2>Chương 63: Bóng tối dưới trăng</h2>
            </div>
            <div className="header-meta">
              <span className="saved-dot" /> Đã lưu
            </div>
          </header>

          <div className="editor-toolbar" aria-label="Editor toolbar shell">
            <button>Paragraph</button>
            <span className="toolbar-separator" />
            <button aria-label="Bold">B</button>
            <button aria-label="Italic">I</button>
            <button aria-label="Underline">U</button>
            <span className="toolbar-separator" />
            <button>• List</button>
            <button>1. List</button>
            <span className="toolbar-grow" />
            <span className="word-count">1,247 từ</span>
          </div>

          <div className="editor-surface">
            <h1>Chương 63: Bóng tối dưới trăng</h1>
            {chapterCopy.map((paragraph) => (
              <p key={paragraph}>{paragraph}</p>
            ))}
            <div className="editor-caret" aria-hidden="true" />
          </div>
        </article>

        <aside className="panel context-panel">
          <div className="tab-row" role="tablist" aria-label="Context tabs">
            <button className="tab active">Ngữ cảnh</button>
            <button className="tab">Ghi chú</button>
            <button className="tab">Tài liệu</button>
            <button className="tab">Công cụ</button>
          </div>

          <div className="context-section">
            <div className="section-title">Thông tin chương</div>
            <dl className="property-grid">
              <dt>Số chương</dt><dd>63</dd>
              <dt>Trạng thái</dt><dd><Badge tone="idle">Đang viết</Badge></dd>
              <dt>Số từ</dt><dd>1,247</dd>
              <dt>Cập nhật</dt><dd>GUI-02C.4 Project Session lifecycle</dd>
            </dl>
          </div>

          <div className="context-section">
            <div className="section-title">Continuity</div>
            <ul>
              <li>Giữ đúng mạch manh mối từ checkpoint gần nhất.</li>
              <li>Không thay đổi canon đã khóa.</li>
              <li>S02–S10 business implementation vẫn CLOSED.</li>
            </ul>
          </div>

          <div className="context-section">
            <div className="section-title">Canon Locks</div>
            <ul>
              <li>Canonical truth vẫn thuộc backend/domain hiện tại.</li>
              <li>Frontend không sở hữu Store, Host hoặc WebAI.</li>
            </ul>
          </div>

          <div className="context-section ai-actions">
            <div className="section-title">Hỗ trợ AI</div>
            <div className="button-grid">
              <button>Gợi ý nội dung</button>
              <button>Mở rộng đoạn văn</button>
              <button>Viết lại</button>
              <button>Tóm tắt chương</button>
            </div>
          </div>
        </aside>
      </section>

      <section className="runtime-grid" aria-label="Runtime shell">
        <article className="panel runtime-card">
          <div className="section-title">AI Bridge Status</div>
          <GatewayDiagnostics />
          <div className="status-row"><span>Gemini Web</span><Badge tone="ready">READY</Badge><span className="muted">business wiring staged later</span></div>
          <div className="status-row"><span>ChatGPT Web</span><Badge tone="ready">READY</Badge><span className="muted">business wiring staged later</span></div>
        </article>

        <article className="panel runtime-card run-card">
          <div className="section-title">Hàng đợi / Run Control</div>
          <div className="run-layout">
            <div><span className="muted">Trạng thái</span><div className="run-value"><Badge tone="idle">IDLE</Badge></div></div>
            <div><span className="muted">Tiếp theo</span><div className="run-value">Chương 64: Dấu vết cũ</div></div>
            <div><span className="muted">Checkpoint</span><div className="run-value">Chương 62</div></div>
          </div>
        </article>
      </section>

      <footer className="action-bar">
        <span className="shell-note">S01 · shared Project Session projection · S02–S10 business remains CLOSED</span>
        <div className="action-buttons">
          <button className="primary">Generate Draft</button>
          <button>Review</button>
          <button>Repair</button>
          <button>Commit</button>
          <button>Export</button>
        </div>
      </footer>
    </>
  )
}

function Placeholder({ screen }: { screen: Screen }) {
  return (
    <section className="panel placeholder-panel">
      <div className="placeholder-id">{screen.id}</div>
      <h2>{screen.label}</h2>
      <p>{screen.hint}</p>
      <div className="placeholder-route">Canonical shell route: {screen.route}</div>
      <p className="muted">S01 is open. This screen's business behavior remains CLOSED until its own AUTHOR gate opens.</p>
    </section>
  )
}

export default function App() {
  const [activeId, setActiveId] = useState('S01')
  const activeScreen = useMemo(() => screens.find((screen) => screen.id === activeId) ?? screens[0], [activeId])

  const projectSession = useProjectSessionController()
  const [projectOverview, setProjectOverview] = useState<ProjectOverviewView | null>(null)
  const [projectOverviewError, setProjectOverviewError] = useState('')

  useEffect(() => {
    let cancelled = false
    const projectRoot = projectSession.state.projectRoot

    if (!projectRoot) {
      setProjectOverview(null)
      setProjectOverviewError('')
      return () => {
        cancelled = true
      }
    }

    if (projectSession.state.status !== 'active') {
      return () => {
        cancelled = true
      }
    }

    const refresh = async () => {
      try {
        const next = await loadProjectOverview(createBrowserGatewayClient())
        if (!cancelled) {
          setProjectOverview(next)
          setProjectOverviewError('')
        }
      } catch (error) {
        if (!cancelled) {
          setProjectOverviewError(
            error instanceof Error ? error.message : 'Project overview failed to load.',
          )
        }
      }
    }

    void refresh()

    return () => {
      cancelled = true
    }
  }, [projectSession.state.projectRoot, projectSession.state.status])

  return (
    <div className="app-shell">
      <header className="titlebar">
        <div className="titlebar-brand"><span className="brand-mark small">A</span><strong>AINOVEL Desktop</strong><span className="version">v0.2.0 · S01 GREEN-1</span></div>
        <div className="window-dots" aria-hidden="true"><span>—</span><span>□</span><span>×</span></div>
      </header>

      <div className="app-body">
        <aside className="sidebar">
          <div className="brand-block">
            <span className="brand-mark">A</span>
            <div><div className="brand-name">AINOVEL</div><div className="brand-subtitle">Desktop Shell</div></div>
          </div>

          <nav className="nav-list" aria-label="S01 to S10">
            {screens.map((screen) => (
              <button key={screen.id} className={`nav-item ${activeId === screen.id ? 'active' : ''}`} onClick={() => setActiveId(screen.id)}>
                <span className="nav-id">{screen.id.slice(1)}</span>
                <span>{screen.label}</span>
              </button>
            ))}
          </nav>

          <div className="recent-projects">
            <div className="sidebar-label">Project Session</div>
            <div className="recent-project active">
              {projectOverview?.title || projectSession.state.projectRoot || 'No active project'}
              <span>{projectSession.state.status.toUpperCase()}</span>
            </div>
          </div>
        </aside>

        <main className="main-area">
          <header className="project-header">
            <div className="project-heading">
              <div className="project-icon">▣</div>
              <div>
                <div className="project-title-row"><h1>AINOVEL Project Session</h1><Badge tone="ready">LIFECYCLE</Badge><Badge tone="ready">S01 OPEN</Badge></div>
                <div className="project-subtitle">S01 Project Hub · shared App-level session projection over locked GUI-02C lifecycle</div>
              </div>
            </div>
            <div className="search-box">⌕ <span>Tìm trong dự án...</span><kbd>Ctrl + K</kbd></div>
          </header>

          <div className="breadcrumb-row">
            <strong>{activeScreen.label}</strong>
            <span>›</span>
            <span>{activeScreen.id} · {activeScreen.route}</span>
          </div>

          <div className="content-scroll">
            {activeId === 'S01' ? (
              <ProjectHubPanel
                session={projectSession.state}
                projectRootInput={projectSession.projectRootInput}
                overview={projectOverview}
                overviewError={projectOverviewError}
                onProjectRootInput={projectSession.setProjectRootInput}
                onCreate={() => void projectSession.run('create')}
                onOpen={() => void projectSession.run('open')}
                onSwitch={() => void projectSession.run('switch')}
                onClose={() => void projectSession.run('close')}
              />
            ) : activeId === 'S05' ? (
              <ChapterStudio controller={projectSession} />
            ) : (
              <Placeholder screen={activeScreen} />
            )}
          </div>
        </main>
      </div>
    </div>
  )
}
