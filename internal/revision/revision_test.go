package revision

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/voocel/agentcore"
	"github.com/voocel/agentcore/llm"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/llmcontract"
	"github.com/voocel/ainovel-cli/internal/store"
)

func TestAnalysisContractIsStrictReady(t *testing.T) {
	if err := llmcontract.ValidateStrictReady(analysisContract.Schema); err != nil {
		t.Fatal(err)
	}
}

func TestScanUsesAcceptedContentInsteadOfFileMetadata(t *testing.T) {
	st := newRevisionTestStore(t, 1)
	acceptTestChapter(t, st, 1, "第一段\n第二段", domain.ChapterFacts{Title: "第一章", Summary: "摘要", KeyEvents: []string{"事件"}})

	path := filepath.Join(st.Dir(), "chapters", "01.md")
	if err := os.WriteFile(path, []byte("第一段\r\n第二段"), 0o644); err != nil {
		t.Fatal(err)
	}
	changes, err := Scan(st)
	if err != nil || len(changes) != 0 {
		t.Fatalf("仅行尾变化不应产生修订: changes=%v err=%v", changes, err)
	}
	if err := os.WriteFile(path, []byte("第一段\r\n用户改写"), 0o644); err != nil {
		t.Fatal(err)
	}
	changes, err = Scan(st)
	if err != nil || len(changes) != 1 || changes[0].Before == changes[0].After {
		t.Fatalf("正文变化未被识别: changes=%+v err=%v", changes, err)
	}
}

func TestScanRejectsEmptyCompletedChapter(t *testing.T) {
	st := newRevisionTestStore(t, 1)
	facts := domain.ChapterFacts{Title: "第一章", Summary: "摘要", KeyEvents: []string{"事件"}}
	acceptTestChapter(t, st, 1, "系统正文", facts)
	if err := os.WriteFile(filepath.Join(st.Dir(), "chapters", "01.md"), []byte(" \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Scan(st); err == nil {
		t.Fatal("空终稿必须显式拒绝")
	}
}

func TestMigrateLegacyBaselineKeepsExternalChangeDirty(t *testing.T) {
	st := newRevisionTestStore(t, 1)
	facts := domain.ChapterFacts{
		Title: "第一章", Summary: "林墨离开村庄", Characters: []string{"林墨"}, KeyEvents: []string{"离村"},
		TimelineEvents: []domain.TimelineEvent{{Time: "清晨", Event: "林墨离村", Characters: []string{"林墨"}}},
		HookType:       "mystery", DominantStrand: "quest",
	}
	if err := st.Drafts.SaveDraft(1, "系统提交的正文"); err != nil {
		t.Fatal(err)
	}
	if err := st.Drafts.SaveFinalChapter(1, "用户后来修改的正文"); err != nil {
		t.Fatal(err)
	}
	if err := st.Summaries.SaveSummary(domain.ChapterSummary{
		Chapter: 1, Title: facts.Title, Summary: facts.Summary, Characters: facts.Characters, KeyEvents: facts.KeyEvents,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.Progress.StartChapter(1); err != nil {
		t.Fatal(err)
	}
	if err := st.Progress.MarkChapterComplete(1, 8, facts.HookType, facts.DominantStrand); err != nil {
		t.Fatal(err)
	}
	writeLegacyCommitSession(t, st.Dir(), 1, facts)

	if err := MigrateLegacyBaseline(st); err != nil {
		t.Fatal(err)
	}
	record, err := st.ChapterRecords.Load(1)
	if err != nil || record == nil {
		t.Fatalf("迁移后接纳记录缺失: record=%+v err=%v", record, err)
	}
	if record.Content != "系统提交的正文" {
		t.Fatalf("迁移错误接纳了当前工作区正文: %q", record.Content)
	}
	changes, err := Scan(st)
	if err != nil || len(changes) != 1 || changes[0].Chapter != 1 {
		t.Fatalf("迁移前的外部修改应保持待同步: changes=%+v err=%v", changes, err)
	}
	if err := MigrateLegacyBaseline(st); err != nil {
		t.Fatalf("重复迁移应幂等: %v", err)
	}
}

func TestMigrateLegacyBaselineFromImportArtifact(t *testing.T) {
	st := newRevisionTestStore(t, 1)
	facts := domain.ChapterFacts{
		Title: "第一章", Summary: "旧书导入", Characters: []string{"林墨"}, KeyEvents: []string{"进入旧城"},
		HookType: "mystery", DominantStrand: "quest",
	}
	if err := st.Drafts.SaveDraft(1, "导入正文"); err != nil {
		t.Fatal(err)
	}
	if err := st.Drafts.SaveFinalChapter(1, "导入正文"); err != nil {
		t.Fatal(err)
	}
	if err := st.Summaries.SaveSummary(domain.ChapterSummary{
		Chapter: 1, Title: facts.Title, Summary: facts.Summary, Characters: facts.Characters, KeyEvents: facts.KeyEvents,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.Progress.StartChapter(1); err != nil {
		t.Fatal(err)
	}
	if err := st.Progress.MarkChapterComplete(1, 4, facts.HookType, facts.DominantStrand); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Checkpoints.AppendArtifact(domain.ChapterScope(1), "commit", "chapters/01.md"); err != nil {
		t.Fatal(err)
	}
	writeLegacyImportArtifact(t, st.Dir(), 1, facts)

	if err := MigrateLegacyBaseline(st); err != nil {
		t.Fatal(err)
	}
	record, err := st.ChapterRecords.Load(1)
	if err != nil || record == nil || record.Facts.Summary != facts.Summary || record.Content != "导入正文" {
		t.Fatalf("导入书迁移结果错误: record=%+v err=%v", record, err)
	}
}

func TestChangedExcerptOmitsUnchangedPrefixAndSuffix(t *testing.T) {
	got := changedExcerpt("相同开头\n旧内容\n相同结尾", "相同开头\n新内容\n相同结尾")
	if got.Before != "旧内容" || got.After != "新内容" || got.BeforeStart != 2 || got.AfterStart != 2 {
		t.Fatalf("changed excerpt = %+v", got)
	}
}

func TestProjectorRebuildsWorldStateFromChapterRecords(t *testing.T) {
	st := newRevisionTestStore(t, 2)
	if err := st.World.SaveTimeline([]domain.TimelineEvent{{Chapter: 1, Time: "旧", Event: "应被删除"}}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	records := []domain.ChapterRecord{
		testRecord(1, "正文一", domain.ChapterFacts{
			Title: "第一章", Summary: "新摘要", Characters: []string{"林墨", "店主"}, KeyEvents: []string{"离城"},
			TimelineEvents:      []domain.TimelineEvent{{Time: "当夜", Event: "林墨离城", Characters: []string{"林墨"}}},
			ForeshadowUpdates:   []domain.ForeshadowUpdate{{ID: "信件", Action: "plant", Description: "未拆的信"}},
			RelationshipChanges: []domain.RelationshipEntry{{CharacterA: "林墨", CharacterB: "店主", Relation: "互相信任"}},
			StateChanges:        []domain.StateChange{{Entity: "林墨", Field: "location", NewValue: "城外"}},
			CastIntros:          []domain.CastIntro{{Name: "店主", BriefRole: "客栈店主"}}, HookType: "mystery", DominantStrand: "quest",
		}, domain.StyleDelta{Prose: []string{"减少解释性心理描写"}}, now),
		testRecord(2, "正文二", domain.ChapterFacts{
			Title: "第二章", Summary: "后续", Characters: []string{"林墨", "店主"}, KeyEvents: []string{"拆信"},
			ForeshadowUpdates:   []domain.ForeshadowUpdate{{ID: "信件", Action: "resolve"}},
			RelationshipChanges: []domain.RelationshipEntry{{CharacterA: "店主", CharacterB: "林墨", Relation: "决裂"}},
		}, domain.StyleDelta{}, now.Add(time.Minute)),
	}
	if err := NewProjector(st).Apply(records); err != nil {
		t.Fatal(err)
	}
	timeline, _ := st.World.LoadTimeline()
	if len(timeline) != 1 || timeline[0].Event != "林墨离城" || timeline[0].Chapter != 1 {
		t.Fatalf("时间线未按记录重建: %+v", timeline)
	}
	ledger, _ := st.World.LoadForeshadowLedger()
	if len(ledger) != 1 || ledger[0].Status != "resolved" || ledger[0].ResolvedAt != 2 {
		t.Fatalf("伏笔投影错误: %+v", ledger)
	}
	relationships, _ := st.World.LoadRelationships()
	if len(relationships) != 1 || relationships[0].Relation != "决裂" || relationships[0].Chapter != 2 {
		t.Fatalf("关系投影错误: %+v", relationships)
	}
	style, _ := st.World.LoadAuthorRevisionStyle()
	if style == nil || len(style.Prose) != 1 || style.Prose[0] != "减少解释性心理描写" {
		t.Fatalf("用户修订风格未投影: %+v", style)
	}
}

func TestServiceAcceptsRevisionAndRefreshesFacts(t *testing.T) {
	st := newRevisionTestStore(t, 1)
	acceptTestChapter(t, st, 1, "林墨留在城中。", domain.ChapterFacts{
		Title: "第一章", Summary: "林墨留在城中", Characters: []string{"林墨"}, KeyEvents: []string{"留城"},
	})
	if err := os.WriteFile(filepath.Join(st.Dir(), "chapters", "01.md"), []byte("林墨连夜离开城市。"), 0o644); err != nil {
		t.Fatal(err)
	}
	model := &revisionModel{response: `{
  "change_summary":"林墨由留城改为连夜离城",
  "story_changed":true,
  "facts":{
    "title":"第一章","summary":"林墨连夜离城","characters":["林墨"],"key_events":["林墨离城"],
    "timeline_events":[{"time":"当夜","event":"林墨离开城市","characters":["林墨"]}],
    "foreshadow_updates":[],"relationship_changes":[],
    "state_changes":[{"entity":"林墨","field":"location","old_value":"城中","new_value":"城外","reason":"主动离开"}],
    "cast_intros":[],"hook_type":null,"dominant_strand":null
  },
  "style_delta":{"prose":["动作表达直接，不补充解释"],"dialogue":[],"taboos":[]},
  "outline_impact":{"deviation":"主角已提前离城","suggestion":"后续从城外承接"},
  "downstream_issues":[]
}`}
	index := &recordingStyleIndex{}
	result, err := NewService(st, model, "分析用户修订", index).Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Applied) != 1 || result.Applied[0] != 1 {
		t.Fatalf("同步结果错误: %+v", result)
	}
	record, err := st.ChapterRecords.Load(1)
	if err != nil || record == nil || record.Origin != domain.ChapterOriginUser || record.Revision != 2 {
		t.Fatalf("接纳记录错误: record=%+v err=%v", record, err)
	}
	summary, _ := st.Summaries.LoadSummary(1)
	if summary == nil || summary.Summary != "林墨连夜离城" {
		t.Fatalf("摘要未刷新: %+v", summary)
	}
	changes, err := Scan(st)
	if err != nil || len(changes) != 0 {
		t.Fatalf("接纳后工作区仍为 dirty: changes=%v err=%v", changes, err)
	}
	if index.chapter != 1 || index.text != "林墨连夜离开城市。" {
		t.Fatalf("风格统计索引未刷新: %+v", index)
	}
	if cp := st.Checkpoints.LatestByStep(domain.ChapterScope(1), "revision_sync"); cp == nil {
		t.Fatal("缺少 revision_sync checkpoint")
	}
}

func TestServiceResumesProjectionWithoutCallingModel(t *testing.T) {
	st := newRevisionTestStore(t, 1)
	oldFacts := domain.ChapterFacts{Title: "第一章", Summary: "旧摘要", KeyEvents: []string{"旧事件"}}
	acceptTestChapter(t, st, 1, "旧正文", oldFacts)
	newFacts := domain.ChapterFacts{Title: "第一章", Summary: "新摘要", KeyEvents: []string{"新事件"}}
	record := testRecord(1, "用户正文", newFacts, domain.StyleDelta{}, time.Now())
	record.Revision = 2
	if err := st.Drafts.SaveFinalChapter(1, record.Content); err != nil {
		t.Fatal(err)
	}
	if err := st.ChapterRecords.Save(record); err != nil {
		t.Fatal(err)
	}
	pending := domain.PendingRevision{
		Stage:     domain.RevisionStageRecordsApplied,
		Items:     []domain.PendingRevisionItem{{Chapter: 1, Record: record, Analysis: domain.RevisionAnalysis{Facts: newFacts}}},
		StartedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := st.Revisions.SavePending(pending); err != nil {
		t.Fatal(err)
	}
	if _, err := NewService(st, nil, "", nil).Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	summary, _ := st.Summaries.LoadSummary(1)
	if summary == nil || summary.Summary != "新摘要" {
		t.Fatalf("恢复后摘要未投影: %+v", summary)
	}
	if pending, _ := st.Revisions.LoadPending(); pending != nil {
		t.Fatalf("恢复记录未清理: %+v", pending)
	}
}

func TestServiceResumesPreparedAfterRecordWasAlreadyWritten(t *testing.T) {
	st := newRevisionTestStore(t, 1)
	oldFacts := domain.ChapterFacts{Title: "第一章", Summary: "旧摘要", KeyEvents: []string{"旧事件"}}
	acceptTestChapter(t, st, 1, "旧正文", oldFacts)
	base, err := st.ChapterRecords.Load(1)
	if err != nil {
		t.Fatal(err)
	}
	newFacts := domain.ChapterFacts{Title: "第一章", Summary: "新摘要", KeyEvents: []string{"新事件"}}
	record := testRecord(1, "用户正文", newFacts, domain.StyleDelta{}, time.Now())
	record.Revision = base.Revision + 1
	if err := st.Drafts.SaveFinalChapter(1, record.Content); err != nil {
		t.Fatal(err)
	}
	if err := st.ChapterRecords.Save(record); err != nil {
		t.Fatal(err)
	}
	pending := domain.PendingRevision{
		Stage: domain.RevisionStagePrepared,
		Items: []domain.PendingRevisionItem{{
			Chapter: 1, BaseSHA256: base.ContentSHA256, CurrentSHA256: record.ContentSHA256,
			Record: record, Analysis: domain.RevisionAnalysis{Facts: newFacts},
		}},
		StartedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := st.Revisions.SavePending(pending); err != nil {
		t.Fatal(err)
	}
	if _, err := NewService(st, nil, "", nil).Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	summary, _ := st.Summaries.LoadSummary(1)
	if summary == nil || summary.Summary != "新摘要" {
		t.Fatalf("prepared 恢复未重建投影: %+v", summary)
	}
	if pending, _ := st.Revisions.LoadPending(); pending != nil {
		t.Fatalf("prepared 恢复记录未清理: %+v", pending)
	}
}

func TestServiceResumesPartiallyWrittenPreparedBatch(t *testing.T) {
	st := newRevisionTestStore(t, 2)
	oldFacts := func(chapter int) domain.ChapterFacts {
		return domain.ChapterFacts{Title: "旧标题", Summary: "旧摘要", KeyEvents: []string{"旧事件"}}
	}
	acceptTestChapter(t, st, 1, "旧正文一", oldFacts(1))
	acceptTestChapter(t, st, 2, "旧正文二", oldFacts(2))
	items := make([]domain.PendingRevisionItem, 0, 2)
	for chapter := 1; chapter <= 2; chapter++ {
		base, err := st.ChapterRecords.Load(chapter)
		if err != nil {
			t.Fatal(err)
		}
		facts := domain.ChapterFacts{Title: "新标题", Summary: fmt.Sprintf("新摘要%d", chapter), KeyEvents: []string{"新事件"}}
		content := fmt.Sprintf("用户正文%d", chapter)
		record := testRecord(chapter, content, facts, domain.StyleDelta{}, time.Now())
		record.Revision = base.Revision + 1
		if err := st.Drafts.SaveFinalChapter(chapter, content); err != nil {
			t.Fatal(err)
		}
		items = append(items, domain.PendingRevisionItem{
			Chapter: chapter, BaseSHA256: base.ContentSHA256, CurrentSHA256: record.ContentSHA256,
			Record: record, Analysis: domain.RevisionAnalysis{Facts: facts},
		})
	}
	if err := st.ChapterRecords.Save(items[0].Record); err != nil {
		t.Fatal(err)
	}
	pending := domain.PendingRevision{Stage: domain.RevisionStagePrepared, Items: items, StartedAt: time.Now(), UpdatedAt: time.Now()}
	if err := st.Revisions.SavePending(pending); err != nil {
		t.Fatal(err)
	}
	if _, err := NewService(st, nil, "", nil).Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	for chapter := 1; chapter <= 2; chapter++ {
		record, _ := st.ChapterRecords.Load(chapter)
		summary, _ := st.Summaries.LoadSummary(chapter)
		if record == nil || record.Revision != 2 || summary == nil || summary.Summary != fmt.Sprintf("新摘要%d", chapter) {
			t.Fatalf("chapter %d not recovered: record=%+v summary=%+v", chapter, record, summary)
		}
	}
}

func TestProjectorFillsCastRoleFromLaterChapter(t *testing.T) {
	st := newRevisionTestStore(t, 2)
	now := time.Now()
	records := []domain.ChapterRecord{
		testRecord(1, "正文一", domain.ChapterFacts{
			Title: "第一章", Summary: "初见店主", Characters: []string{"店主"}, KeyEvents: []string{"初见"},
		}, domain.StyleDelta{}, now),
		testRecord(2, "正文二", domain.ChapterFacts{
			Title: "第二章", Summary: "确认身份", Characters: []string{"店主"}, KeyEvents: []string{"确认身份"},
			CastIntros: []domain.CastIntro{{Name: "店主", BriefRole: "客栈店主"}},
		}, domain.StyleDelta{}, now.Add(time.Minute)),
	}
	if err := NewProjector(st).Apply(records); err != nil {
		t.Fatal(err)
	}
	cast, err := st.Cast.Load()
	if err != nil || len(cast) != 1 || cast[0].BriefRole != "客栈店主" {
		t.Fatalf("后续角色简介未补全: cast=%+v err=%v", cast, err)
	}
}

func TestServiceRejectsAndClearsStalePreparedAnalysis(t *testing.T) {
	st := newRevisionTestStore(t, 1)
	facts := domain.ChapterFacts{Title: "第一章", Summary: "摘要", KeyEvents: []string{"事件"}}
	acceptTestChapter(t, st, 1, "系统正文", facts)
	path := filepath.Join(st.Dir(), "chapters", "01.md")
	if err := os.WriteFile(path, []byte("第一次修改"), 0o644); err != nil {
		t.Fatal(err)
	}
	base, _ := st.ChapterRecords.Load(1)
	record := testRecord(1, "第一次修改", facts, domain.StyleDelta{}, time.Now())
	record.Revision = 2
	pending := domain.PendingRevision{
		Stage: domain.RevisionStagePrepared,
		Items: []domain.PendingRevisionItem{{
			Chapter: 1, BaseSHA256: base.ContentSHA256,
			CurrentSHA256: domain.ChapterContentSHA256("第一次修改"), Record: record,
		}},
		StartedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := st.Revisions.SavePending(pending); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("第二次修改"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewService(st, nil, "", nil).Sync(context.Background()); err == nil {
		t.Fatal("分析后再次修改正文应拒绝应用")
	}
	if pending, _ := st.Revisions.LoadPending(); pending != nil {
		t.Fatalf("过期 prepared 记录应被清理: %+v", pending)
	}
}

type revisionModel struct{ response string }

func (m *revisionModel) Capabilities() llm.Capabilities {
	return llm.Capabilities{Structured: llm.StructuredCapabilities{JSONSchema: llm.SupportYes, Strict: llm.SupportYes}}
}

func (m *revisionModel) Generate(context.Context, []agentcore.Message, []agentcore.ToolSpec, ...agentcore.CallOption) (*agentcore.LLMResponse, error) {
	return &agentcore.LLMResponse{Message: agentcore.Message{
		Role: agentcore.RoleAssistant, Content: []agentcore.ContentBlock{agentcore.TextBlock(m.response)}, StopReason: agentcore.StopReasonStop,
	}}, nil
}

func (m *revisionModel) GenerateStream(context.Context, []agentcore.Message, []agentcore.ToolSpec, ...agentcore.CallOption) (<-chan agentcore.StreamEvent, error) {
	return nil, nil
}

func (m *revisionModel) SupportsTools() bool { return true }

type recordingStyleIndex struct {
	chapter int
	text    string
}

func (i *recordingStyleIndex) ChapterCommitted(chapter int, text string) {
	i.chapter, i.text = chapter, text
}

func newRevisionTestStore(t *testing.T, total int) *store.Store {
	t.Helper()
	st := store.NewStore(t.TempDir())
	if err := st.Init(); err != nil {
		t.Fatal(err)
	}
	if err := st.Progress.Init(total); err != nil {
		t.Fatal(err)
	}
	if err := st.Progress.UpdatePhase(domain.PhaseWriting); err != nil {
		t.Fatal(err)
	}
	return st
}

func acceptTestChapter(t *testing.T, st *store.Store, chapter int, content string, facts domain.ChapterFacts) {
	t.Helper()
	if err := st.Drafts.SaveFinalChapter(chapter, content); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ChapterRecords.Accept(chapter, domain.ChapterOriginGenerated, content, facts, domain.StyleDelta{}); err != nil {
		t.Fatal(err)
	}
	if err := st.Progress.StartChapter(chapter); err != nil {
		t.Fatal(err)
	}
	if err := st.Progress.MarkChapterComplete(chapter, len([]rune(content)), facts.HookType, facts.DominantStrand); err != nil {
		t.Fatal(err)
	}
	if err := st.Summaries.SaveSummary(domain.ChapterSummary{
		Chapter: chapter, Title: facts.Title, Summary: facts.Summary, Characters: facts.Characters, KeyEvents: facts.KeyEvents,
	}); err != nil {
		t.Fatal(err)
	}
}

func testRecord(chapter int, content string, facts domain.ChapterFacts, style domain.StyleDelta, acceptedAt time.Time) domain.ChapterRecord {
	return domain.ChapterRecord{
		Version: domain.ChapterRecordVersion, Chapter: chapter, Revision: 1, Origin: domain.ChapterOriginUser,
		Content: content, ContentSHA256: domain.ChapterContentSHA256(content), Facts: facts, StyleDelta: style, AcceptedAt: acceptedAt,
	}
}

func writeLegacyCommitSession(t *testing.T, dir string, chapter int, facts domain.ChapterFacts) {
	t.Helper()
	args, err := json.Marshal(legacyCommitArgs{Chapter: chapter, ChapterFacts: facts})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	messages := []agentcore.Message{
		{
			Role: agentcore.RoleAssistant, Timestamp: now.Add(-time.Second),
			Content: []agentcore.ContentBlock{agentcore.ToolCallBlock(agentcore.ToolCall{
				ID: "commit-1", Name: "commit_chapter", Args: args,
			})},
		},
		{
			Role: agentcore.RoleTool, Timestamp: now,
			Content:  []agentcore.ContentBlock{agentcore.TextBlock(`{"committed":true}`)},
			Metadata: map[string]any{"tool_call_id": "commit-1", "tool_name": "commit_chapter", "is_error": false},
		},
	}
	var data []byte
	for _, message := range messages {
		line, err := json.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		data = append(data, line...)
		data = append(data, '\n')
	}
	path := filepath.Join(dir, "meta", "sessions", "agents", fmt.Sprintf("writer-ch%02d.jsonl", chapter))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeLegacyImportArtifact(t *testing.T, dir string, chapter int, facts domain.ChapterFacts) {
	t.Helper()
	artifact := struct {
		Payload struct {
			Facts legacyCommitArgs `json:"facts"`
		} `json:"payload"`
	}{}
	artifact.Payload.Facts = legacyCommitArgs{Chapter: chapter, ChapterFacts: facts}
	data, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "meta", "import", "analyses", fmt.Sprintf("%06d.json", chapter))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
