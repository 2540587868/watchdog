package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ysqss/watchdog/internal/target"
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		PRAGMA journal_mode=WAL;
		PRAGMA synchronous=NORMAL;
		PRAGMA busy_timeout=5000;

		CREATE TABLE IF NOT EXISTS targets (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			type        TEXT NOT NULL,
			url         TEXT NOT NULL,
			interval_ms INTEGER NOT NULL DEFAULT 30000,
			timeout_ms  INTEGER NOT NULL DEFAULT 5000,
			method      TEXT DEFAULT 'GET',
			headers     TEXT DEFAULT '{}',
			expect_status INTEGER DEFAULT 200,
			expect_body TEXT DEFAULT '',
			tls_skip_verify INTEGER DEFAULT 0,
			healthy_threshold   INTEGER DEFAULT 3,
			unhealthy_threshold INTEGER DEFAULT 3,
			degraded_threshold_ms INTEGER DEFAULT 0,
			alert_level TEXT DEFAULT 'critical',
			labels      TEXT DEFAULT '{}',
			silenced_until TEXT,
			created_at  TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE TABLE IF NOT EXISTS probe_history (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			target_id   TEXT NOT NULL,
			timestamp   TEXT NOT NULL,
			success     INTEGER NOT NULL,
			status_code INTEGER DEFAULT 0,
			latency_ms  INTEGER DEFAULT 0,
			error       TEXT DEFAULT '',
			FOREIGN KEY (target_id) REFERENCES targets(id)
		);

		CREATE INDEX IF NOT EXISTS idx_probe_target_time
			ON probe_history(target_id, timestamp DESC);

		CREATE TABLE IF NOT EXISTS events (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			target_id   TEXT NOT NULL,
			timestamp   TEXT NOT NULL,
			from_state  TEXT NOT NULL,
			to_state    TEXT NOT NULL,
			message     TEXT DEFAULT '',
			FOREIGN KEY (target_id) REFERENCES targets(id)
		);

		CREATE INDEX IF NOT EXISTS idx_events_target_time
			ON events(target_id, timestamp DESC);
	`)
	return err
}

type TargetRecord struct {
	ID                 string
	Name               string
	Type               target.ProbeType
	URL                string
	IntervalMs         int
	TimeoutMs          int
	Method             string
	Headers            map[string]string
	ExpectStatus       int
	ExpectBody         string
	TLSSkipVerify      bool
	HealthyThreshold   int
	UnhealthyThreshold int
	AlertLevel         target.AlertLevel
	Labels             map[string]string
	SilencedUntil      *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (s *Store) InsertTarget(t *target.Target) error {
	headersJSON, _ := json.Marshal(t.Headers)
	labelsJSON, _ := json.Marshal(t.Labels)
	var silencedUntil any
	if t.SilencedUntil != nil {
		silencedUntil = t.SilencedUntil.Format(time.RFC3339)
	}
	_, err := s.db.Exec(
		`INSERT INTO targets (id, name, type, url, interval_ms, timeout_ms, method, headers,
		 expect_status, expect_body, tls_skip_verify, healthy_threshold, unhealthy_threshold,
		 degraded_threshold_ms, alert_level, labels, silenced_until)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Name, string(t.Type), t.URL, t.Interval, t.Timeout,
		t.GetMethod(), string(headersJSON), t.GetExpectStatus(), t.ExpectBody,
		boolToInt(t.TLSSkipVerify), t.GetHealthyThreshold(), t.GetUnhealthyThreshold(),
		t.DegradedThresholdMs,
		string(t.GetAlertLevel()), string(labelsJSON), silencedUntil,
	)
	return err
}

func (s *Store) GetTargetByID(id string) (*target.Target, error) {
	row := s.db.QueryRow(
		`SELECT id, name, type, url, interval_ms, timeout_ms, method, headers,
		 expect_status, expect_body, tls_skip_verify, healthy_threshold, unhealthy_threshold,
		 degraded_threshold_ms, alert_level, labels, silenced_until, created_at, updated_at
		 FROM targets WHERE id = ?`, id,
	)
	return s.scanTarget(row)
}

func (s *Store) ListTargets() ([]*target.Target, error) {
	rows, err := s.db.Query(
		`SELECT id, name, type, url, interval_ms, timeout_ms, method, headers,
		 expect_status, expect_body, tls_skip_verify, healthy_threshold, unhealthy_threshold,
		 degraded_threshold_ms, alert_level, labels, silenced_until, created_at, updated_at
		 FROM targets ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var targets []*target.Target
	for rows.Next() {
		t, err := s.scanTargetFromRows(rows)
		if err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

func (s *Store) DeleteTarget(id string) error {
	_, err := s.db.Exec(`DELETE FROM targets WHERE id = ?`, id)
	return err
}

func (s *Store) UpdateTarget(t *target.Target) error {
	headersJSON, _ := json.Marshal(t.Headers)
	labelsJSON, _ := json.Marshal(t.Labels)

	var silencedUntil any
	if t.SilencedUntil != nil {
		silencedUntil = t.SilencedUntil.Format(time.RFC3339)
	}

	_, err := s.db.Exec(
		`UPDATE targets SET name=?, type=?, url=?, interval_ms=?, timeout_ms=?, method=?,
		 headers=?, expect_status=?, expect_body=?, tls_skip_verify=?,
		 healthy_threshold=?, unhealthy_threshold=?, degraded_threshold_ms=?,
		 alert_level=?, labels=?, silenced_until=?, updated_at=?
		 WHERE id=?`,
		t.Name, string(t.Type), t.URL, t.Interval, t.Timeout,
		t.GetMethod(), string(headersJSON), t.GetExpectStatus(), t.ExpectBody,
		boolToInt(t.TLSSkipVerify), t.GetHealthyThreshold(), t.GetUnhealthyThreshold(),
		t.DegradedThresholdMs,
		string(t.GetAlertLevel()), string(labelsJSON), silencedUntil,
		t.UpdatedAt.Format(time.RFC3339), t.ID,
	)
	return err
}

func (s *Store) UpdateTargetSilence(id string, silencedUntil *time.Time) error {
	var val any
	if silencedUntil != nil {
		val = silencedUntil.Format(time.RFC3339)
	}
	_, err := s.db.Exec(
		`UPDATE targets SET silenced_until = ?, updated_at = ? WHERE id = ?`,
		val, time.Now().Format(time.RFC3339), id,
	)
	return err
}

type ProbeHistoryRecord struct {
	ID         int64
	TargetID   string
	Timestamp  time.Time
	Success    bool
	StatusCode int
	LatencyMs  int64
	Error      string
}

func (s *Store) InsertProbeResult(r *ProbeHistoryRecord) error {
	_, err := s.db.Exec(
		`INSERT INTO probe_history (target_id, timestamp, success, status_code, latency_ms, error)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		r.TargetID, r.Timestamp.Format(time.RFC3339), boolToInt(r.Success),
		r.StatusCode, r.LatencyMs, r.Error,
	)
	return err
}

func (s *Store) ListProbeHistory(targetID string, limit int) ([]*ProbeHistoryRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(
		`SELECT id, target_id, timestamp, success, status_code, latency_ms, error
		 FROM probe_history WHERE target_id = ? ORDER BY timestamp DESC LIMIT ?`,
		targetID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*ProbeHistoryRecord
	for rows.Next() {
		var r ProbeHistoryRecord
		var success int
		var ts string
		err := rows.Scan(&r.ID, &r.TargetID, &ts, &success, &r.StatusCode, &r.LatencyMs, &r.Error)
		if err != nil {
			return nil, err
		}
		r.Success = success == 1
		r.Timestamp, _ = time.Parse(time.RFC3339, ts)
		records = append(records, &r)
	}
	return records, rows.Err()
}

type EventRecord struct {
	ID        int64
	TargetID  string
	Timestamp time.Time
	FromState target.TargetState
	ToState   target.TargetState
	Message   string
}

func (s *Store) InsertEvent(e *EventRecord) error {
	_, err := s.db.Exec(
		`INSERT INTO events (target_id, timestamp, from_state, to_state, message)
		 VALUES (?, ?, ?, ?, ?)`,
		e.TargetID, e.Timestamp.Format(time.RFC3339),
		e.FromState.String(), e.ToState.String(), e.Message,
	)
	return err
}

func (s *Store) ListEvents(targetID string, limit int) ([]*EventRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT id, target_id, timestamp, from_state, to_state, message
		 FROM events WHERE target_id = ? ORDER BY timestamp DESC LIMIT ?`,
		targetID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*EventRecord
	for rows.Next() {
		var e EventRecord
		var ts, fromState, toState string
		err := rows.Scan(&e.ID, &e.TargetID, &ts, &fromState, &toState, &e.Message)
		if err != nil {
			return nil, err
		}
		e.Timestamp, _ = time.Parse(time.RFC3339, ts)
		e.FromState = target.ParseState(fromState)
		e.ToState = target.ParseState(toState)
		events = append(events, &e)
	}
	return events, rows.Err()
}

func (s *Store) GetTargetStats(targetID string) (map[string]any, error) {
	var total, success int64
	s.db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN success=1 THEN 1 ELSE 0 END), 0) FROM probe_history WHERE target_id = ?`,
		targetID,
	).Scan(&total, &success)

	var avgLatency float64
	s.db.QueryRow(
		`SELECT COALESCE(AVG(latency_ms), 0) FROM probe_history WHERE target_id = ? AND success = 1`,
		targetID,
	).Scan(&avgLatency)

	uptime := 0.0
	if total > 0 {
		uptime = float64(success) / float64(total) * 100.0
	}

	return map[string]any{
		"total_probes":   total,
		"success_probes": success,
		"uptime_pct":     uptime,
		"avg_latency_ms": avgLatency,
	}, nil
}

func (s *Store) Cleanup(probeDays, eventDays int) error {
	if probeDays <= 0 {
		probeDays = 30
	}
	if eventDays <= 0 {
		eventDays = 90
	}
	_, err := s.db.Exec(
		`DELETE FROM probe_history WHERE timestamp < datetime('now', ?)`,
		fmt.Sprintf("-%d days", probeDays),
	)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`DELETE FROM events WHERE timestamp < datetime('now', ?)`,
		fmt.Sprintf("-%d days", eventDays),
	)
	return err
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Ping() error {
	var n int
	return s.db.QueryRow("SELECT 1").Scan(&n)
}

type scanner interface {
	Scan(dest ...any) error
}

func (s *Store) scanTarget(row *sql.Row) (*target.Target, error) {
	return s.scanTargetFromScanner(row)
}

func (s *Store) scanTargetFromRows(rows *sql.Rows) (*target.Target, error) {
	return s.scanTargetFromScanner(rows)
}

func (s *Store) scanTargetFromScanner(sc scanner) (*target.Target, error) {
	var t target.Target
	var headersJSON, labelsJSON, createdAt, updatedAt string
	var tlsSkip int
	var silencedUntil sql.NullString
	err := sc.Scan(
		&t.ID, &t.Name, &t.Type, &t.URL, &t.Interval, &t.Timeout,
		&t.Method, &headersJSON, &t.ExpectStatus, &t.ExpectBody,
		&tlsSkip, &t.HealthyThreshold, &t.UnhealthyThreshold,
		&t.DegradedThresholdMs, &t.AlertLevel, &labelsJSON, &silencedUntil,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(headersJSON), &t.Headers)
	json.Unmarshal([]byte(labelsJSON), &t.Labels)
	t.TLSSkipVerify = tlsSkip == 1
	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if silencedUntil.Valid {
		st, err := time.Parse(time.RFC3339, silencedUntil.String)
		if err == nil {
			t.SilencedUntil = &st
		}
	}
	return &t, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
