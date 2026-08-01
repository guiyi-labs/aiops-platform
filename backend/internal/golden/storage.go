package golden

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ReportStorage persists and retrieves quality reports. The default
// implementation writes JSON files to a bounded directory under
// .artifacts/quality-report/.
type ReportStorage interface {
	// Save writes the report to persistent storage.
	Save(report QualityReport) error
	// LoadLatest returns the most recently saved report, or ErrNoReport
	// if none exists.
	LoadLatest() (QualityReport, error)
}

// ErrNoReport is returned by LoadLatest when no quality report has been
// saved yet.
var ErrNoReport = fmt.Errorf("no quality report available")

// FileReportStorage writes quality reports as timestamped JSON files in
// a fixed directory. The directory is created on first Save. Files are
// named <timestamp>-<dataset-version>.json and sorted by name (which is
// chronological due to the timestamp prefix) to find the latest.
type FileReportStorage struct {
	dir string
}

// NewFileReportStorage returns a FileReportStorage rooted at the given
// directory (typically .artifacts/quality-report/).
func NewFileReportStorage(dir string) *FileReportStorage {
	return &FileReportStorage{dir: dir}
}

// Save writes the report as a JSON file. The directory is created if it
// does not exist. Returns an error if the write fails.
func (s *FileReportStorage) Save(report QualityReport) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create quality report directory: %w", err)
	}
	ts := report.GeneratedAt.UTC().Format("20060102-150405")
	name := fmt.Sprintf("%s-%s.json", ts, report.DatasetVersionAfter)
	if report.DatasetVersionAfter == "" {
		name = fmt.Sprintf("%s-unknown.json", ts)
	}
	path := filepath.Join(s.dir, name)
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal quality report: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write quality report file: %w", err)
	}
	return nil
}

// LoadLatest returns the most recently saved report. Files are listed
// and sorted by name descending; the first readable JSON file is returned.
// Returns ErrNoReport if the directory does not exist or contains no
// .json files.
func (s *FileReportStorage) LoadLatest() (QualityReport, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return QualityReport{}, ErrNoReport
		}
		return QualityReport{}, fmt.Errorf("read quality report directory: %w", err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return QualityReport{}, ErrNoReport
	}

	sort.Sort(sort.Reverse(sort.StringSlice(names)))

	path := filepath.Join(s.dir, names[0])
	data, err := os.ReadFile(path)
	if err != nil {
		return QualityReport{}, fmt.Errorf("read quality report file %s: %w", names[0], err)
	}

	var report QualityReport
	if err := json.Unmarshal(data, &report); err != nil {
		return QualityReport{}, fmt.Errorf("unmarshal quality report %s: %w", names[0], err)
	}
	return report, nil
}

// NopReportStorage is a no-op ReportStorage for testing. Save is a no-op;
// LoadLatest always returns ErrNoReport.
type NopReportStorage struct{}

func (NopReportStorage) Save(QualityReport) error           { return nil }
func (NopReportStorage) LoadLatest() (QualityReport, error) { return QualityReport{}, ErrNoReport }

// NowFunc is the clock used for report generation timestamps. Tests can
// override it; production code uses time.Now.
var NowFunc = func() time.Time { return time.Now().UTC() }
