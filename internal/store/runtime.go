package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/voocel/ainovel-cli/internal/domain"
)

const (
	runtimeQueuePath = "meta/runtime/queue.jsonl"
	runtimeRunsRoot  = "meta/runtime/runs"
)

// RuntimeStore 管理统一运行时队列和每任务日志。
type RuntimeStore struct {
	io *IO

	mu         sync.Mutex
	seqLoaded  bool
	nextSeqNum int64
}

func NewRuntimeStore(io *IO) *RuntimeStore {
	return &RuntimeStore{io: io}
}

// AppendQueue 追加一条运行时队列记录，并自动分配递增序号。
func (s *RuntimeStore) AppendQueue(item domain.RuntimeQueueItem) (domain.RuntimeQueueItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureSeqLoadedLocked(); err != nil {
		return item, err
	}
	s.nextSeqNum++
	item.Seq = s.nextSeqNum
	if item.Time.IsZero() {
		item.Time = time.Now()
	}
	if err := s.appendJSONLine(runtimeQueuePath, item); err != nil {
		return item, err
	}
	return item, nil
}

// LoadQueue 读取当前持久化的全部运行时队列项。
func (s *RuntimeStore) LoadQueue() ([]domain.RuntimeQueueItem, error) {
	return loadJSONLines[domain.RuntimeQueueItem](s.io, runtimeQueuePath)
}

// LoadQueueAfter 返回指定序号之后的队列项。
func (s *RuntimeStore) LoadQueueAfter(afterSeq int64) ([]domain.RuntimeQueueItem, error) {
	items, err := s.LoadQueue()
	if err != nil || afterSeq <= 0 {
		return items, err
	}
	filtered := items[:0]
	for _, item := range items {
		if item.Seq > afterSeq {
			filtered = append(filtered, item)
		}
	}
	return append([]domain.RuntimeQueueItem(nil), filtered...), nil
}

// AppendTaskLog 追加某个任务的运行日志。
func (s *RuntimeStore) AppendTaskLog(taskID string, entry domain.RuntimeTaskLogEntry) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil
	}
	if entry.Time.IsZero() {
		entry.Time = time.Now()
	}
	if entry.TaskID == "" {
		entry.TaskID = taskID
	}
	return s.appendJSONLine(taskLogPath(taskID), entry)
}

// LoadTaskLog 读取某个任务的全部运行日志。
func (s *RuntimeStore) LoadTaskLog(taskID string) ([]domain.RuntimeTaskLogEntry, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, nil
	}
	return loadJSONLines[domain.RuntimeTaskLogEntry](s.io, taskLogPath(taskID))
}

func taskLogPath(taskID string) string {
	return filepath.Join("meta", "runtime", "tasks", taskID+".log")
}

// SaveRun atomically creates or updates one additive per-run registry fact.
// G05.3 persists facts only; scheduler/priority/lock/lane semantics remain closed.
func (s *RuntimeStore) SaveRun(record domain.RunRegistryRecord) (domain.RunRegistryRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if record.Version == 0 {
		record.Version = domain.CurrentRunRegistryRecordVersion
	}
	if record.Source == "" {
		record.Source = domain.RunRecordSourceRegistry
	}
	if record.Source != domain.RunRecordSourceRegistry {
		return record, fmt.Errorf("legacy run projection is read-only")
	}
	if err := (domain.RunIdentity{RunID: record.RunID, TaskID: record.TaskID}).Validate(); err != nil {
		return record, err
	}
	if record.RunID == domain.LegacySingleRunID {
		return record, fmt.Errorf("run_id %q is reserved for legacy projection", record.RunID)
	}

	existing, err := s.loadRunRecord(record.RunID)
	if err != nil {
		return record, err
	}
	now := time.Now().UTC()
	if existing != nil && record.CreatedAt.IsZero() {
		record.CreatedAt = existing.CreatedAt
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = now
	}
	if err := record.Validate(); err != nil {
		return record, err
	}
	if err := s.io.WriteJSON(runRecordPath(record.RunID), record); err != nil {
		return record, err
	}
	return record, nil
}

// LoadRun loads one persisted G05.3 registry record. The synthetic legacy
// projection is intentionally available only through ListRunsWithLegacyProjection.
func (s *RuntimeStore) LoadRun(runID domain.RunID) (*domain.RunRegistryRecord, error) {
	if err := (domain.RunIdentity{RunID: runID}).Validate(); err != nil {
		return nil, err
	}
	if runID == domain.LegacySingleRunID {
		return nil, fmt.Errorf("run_id %q is reserved for legacy projection", runID)
	}
	return s.loadRunRecord(runID)
}

func (s *RuntimeStore) loadRunRecord(runID domain.RunID) (*domain.RunRegistryRecord, error) {
	var record domain.RunRegistryRecord
	if err := s.io.ReadJSON(runRecordPath(runID), &record); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if record.RunID != runID {
		return nil, fmt.Errorf("run registry identity mismatch: path=%q record=%q", runID, record.RunID)
	}
	if record.Source != domain.RunRecordSourceRegistry {
		return nil, fmt.Errorf("persisted run %q has non-registry source %q", runID, record.Source)
	}
	if err := record.Validate(); err != nil {
		return nil, fmt.Errorf("validate run %q: %w", runID, err)
	}
	return &record, nil
}

// ListRuns returns persisted G05.3 records sorted by opaque run ID.
func (s *RuntimeStore) ListRuns() ([]domain.RunRegistryRecord, error) {
	root := filepath.Join(s.io.dir, filepath.FromSlash(runtimeRunsRoot))
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	runs := make([]domain.RunRegistryRecord, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			return nil, fmt.Errorf("unexpected file under %s: %s", runtimeRunsRoot, entry.Name())
		}
		runID := domain.RunID(entry.Name())
		if err := (domain.RunIdentity{RunID: runID}).Validate(); err != nil {
			return nil, fmt.Errorf("invalid run registry directory %q: %w", entry.Name(), err)
		}
		if runID == domain.LegacySingleRunID {
			return nil, fmt.Errorf("reserved legacy projection directory must not be persisted")
		}
		record, err := s.loadRunRecord(runID)
		if err != nil {
			return nil, err
		}
		if record == nil {
			return nil, fmt.Errorf("run registry directory %q has no run.json", entry.Name())
		}
		runs = append(runs, *record)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].RunID < runs[j].RunID })
	return runs, nil
}

// ListRunsWithLegacyProjection provides a read-only compatibility projection
// only while no additive G05.3 registry records exist.
func (s *RuntimeStore) ListRunsWithLegacyProjection() ([]domain.RunRegistryRecord, error) {
	runs, err := s.ListRuns()
	if err != nil || len(runs) > 0 {
		return runs, err
	}
	legacy, err := s.projectLegacySingleRun()
	if err != nil || legacy == nil {
		return nil, err
	}
	return []domain.RunRegistryRecord{*legacy}, nil
}

func (s *RuntimeStore) projectLegacySingleRun() (*domain.RunRegistryRecord, error) {
	var meta domain.RunMeta
	metaFound := true
	if err := s.io.ReadJSON("meta/run.json", &meta); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read legacy meta/run.json: %w", err)
		}
		metaFound = false
	}

	var progress domain.Progress
	progressFound := true
	if err := s.io.ReadJSON("meta/progress.json", &progress); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read legacy meta/progress.json: %w", err)
		}
		progressFound = false
	}

	if !metaFound && !progressFound {
		return nil, nil
	}
	record := domain.RunRegistryRecord{
		Version: domain.CurrentRunRegistryRecordVersion,
		RunID:   domain.LegacySingleRunID,
		Source:  domain.RunRecordSourceLegacySingleRun,
	}
	if progressFound {
		record.LegacyPhase = progress.Phase
	}
	if metaFound && strings.TrimSpace(meta.StartedAt) != "" {
		if started, err := time.Parse(time.RFC3339, meta.StartedAt); err == nil {
			record.StartedAt = started
		}
	}
	if err := record.Validate(); err != nil {
		return nil, fmt.Errorf("project legacy single run: %w", err)
	}
	return &record, nil
}

// AppendRunHistory persists one sanitized per-run activity fact and assigns a
// monotonically increasing sequence local to that run.
func (s *RuntimeStore) AppendRunHistory(entry domain.RunHistoryRecord) (domain.RunHistoryRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := (domain.RunIdentity{RunID: entry.RunID, TaskID: entry.TaskID}).Validate(); err != nil {
		return entry, err
	}
	if entry.RunID == domain.LegacySingleRunID {
		return entry, fmt.Errorf("legacy projection history is read-only")
	}
	record, err := s.loadRunRecord(entry.RunID)
	if err != nil {
		return entry, err
	}
	if record == nil {
		return entry, fmt.Errorf("run %q is not registered", entry.RunID)
	}
	items, err := loadJSONLines[domain.RunHistoryRecord](s.io, runHistoryPath(entry.RunID))
	if err != nil {
		return entry, err
	}
	if entry.Seq != 0 {
		return entry, fmt.Errorf("history seq is store-assigned")
	}
	entry.Seq = 1
	if len(items) > 0 {
		entry.Seq = items[len(items)-1].Seq + 1
	}
	if entry.Time.IsZero() {
		entry.Time = time.Now().UTC()
	}
	if err := entry.Validate(); err != nil {
		return entry, err
	}
	if err := s.appendJSONLine(runHistoryPath(entry.RunID), entry); err != nil {
		return entry, err
	}
	return entry, nil
}

// LoadRunHistory reads the append-only history for one registered run.
func (s *RuntimeStore) LoadRunHistory(runID domain.RunID) ([]domain.RunHistoryRecord, error) {
	if err := (domain.RunIdentity{RunID: runID}).Validate(); err != nil {
		return nil, err
	}
	if runID == domain.LegacySingleRunID {
		return nil, nil
	}
	return loadJSONLines[domain.RunHistoryRecord](s.io, runHistoryPath(runID))
}

// LoadRunHistoryAfter returns per-run history after the requested sequence.
func (s *RuntimeStore) LoadRunHistoryAfter(runID domain.RunID, afterSeq int64) ([]domain.RunHistoryRecord, error) {
	items, err := s.LoadRunHistory(runID)
	if err != nil || afterSeq <= 0 {
		return items, err
	}
	filtered := items[:0]
	for _, item := range items {
		if item.Seq > afterSeq {
			filtered = append(filtered, item)
		}
	}
	return append([]domain.RunHistoryRecord(nil), filtered...), nil
}

func runRecordPath(runID domain.RunID) string {
	return filepath.Join(runtimeRunsRoot, string(runID), "run.json")
}

func runHistoryPath(runID domain.RunID) string {
	return filepath.Join(runtimeRunsRoot, string(runID), "history.jsonl")
}

// Reset 清空运行时队列和任务日志。
func (s *RuntimeStore) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seqLoaded = false
	s.nextSeqNum = 0

	var errs []string
	if err := os.Remove(filepath.Join(s.io.dir, runtimeQueuePath)); err != nil && !os.IsNotExist(err) {
		errs = append(errs, err.Error())
	}
	if err := os.RemoveAll(filepath.Join(s.io.dir, "meta", "runtime", "tasks")); err != nil {
		errs = append(errs, err.Error())
	}
	if err := os.MkdirAll(filepath.Join(s.io.dir, "meta", "runtime", "tasks"), 0o755); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		sort.Strings(errs)
		return fmt.Errorf("reset runtime store: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (s *RuntimeStore) ensureSeqLoadedLocked() error {
	if s.seqLoaded {
		return nil
	}
	items, err := loadJSONLines[domain.RuntimeQueueItem](s.io, runtimeQueuePath)
	if err != nil {
		return err
	}
	if len(items) > 0 {
		s.nextSeqNum = items[len(items)-1].Seq
	}
	s.seqLoaded = true
	return nil
}

func (s *RuntimeStore) appendJSONLine(rel string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return s.io.AppendLine(rel, data)
}

func loadJSONLines[T any](io *IO, rel string) ([]T, error) {
	data, err := io.ReadFile(rel)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 8*1024*1024)
	var out []T
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item T
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return nil, fmt.Errorf("parse %s: %w", rel, err)
		}
		out = append(out, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
