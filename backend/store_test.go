package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// Minimal database/sql driver exercises binding, RETURNING scans, and failures
// without adding a second third-party dependency to unit tests.
type testDriver struct{}
type testConn struct{ fail bool }
type testRows struct {
	values []driver.Value
	done   bool
}

func (testDriver) Open(name string) (driver.Conn, error) { return &testConn{fail: name == "fail"}, nil }
func (*testConn) Prepare(string) (driver.Stmt, error)    { return nil, errors.New("not supported") }
func (*testConn) Close() error                           { return nil }
func (*testConn) Begin() (driver.Tx, error)              { return nil, errors.New("not supported") }
func (c *testConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	if c.fail {
		return nil, errors.New("database down")
	}
	return driver.RowsAffected(0), nil
}
func (c *testConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if c.fail {
		return nil, errors.New("database down")
	}
	if strings.Contains(query, "mock_access_requests") {
		if len(args) != 1 || args[0].Value != "Jira" {
			return nil, errors.New("incorrect service binding")
		}
		return &testRows{values: []driver.Value{int64(7), "created"}}, nil
	}
	if len(args) != 2 || args[0].Value != "demo-001" || args[1].Value != "Review DEMO-42" {
		return nil, errors.New("incorrect message binding")
	}
	return &testRows{values: []driver.Value{int64(8), time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)}}, nil
}
func (r *testRows) Columns() []string { return []string{"id", "result"} }
func (*testRows) Close() error        { return nil }
func (r *testRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	copy(dest, r.values)
	r.done = true
	return nil
}
func init() { sql.Register("chugun-test", testDriver{}) }
func TestPostgresStore(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "ok"
		if fail {
			name = "fail"
		}
		db, err := sql.Open("chugun-test", name)
		if err != nil {
			t.Fatal(err)
		}
		store := PostgresStore{DB: db}
		ctx := context.Background()
		if err := store.Initialize(ctx); (err != nil) != fail {
			t.Fatalf("initialize: %v", err)
		}
		access, err := store.CreateAccess(ctx, "Jira")
		if (err != nil) != fail {
			t.Fatalf("access: %v", err)
		}
		if !fail && (access.TicketID != "DEMO-ACCESS-7" || access.Status != "created") {
			t.Fatalf("%+v", access)
		}
		message, err := store.SendMessage(ctx, "demo-001", "Review DEMO-42")
		if (err != nil) != fail {
			t.Fatalf("message: %v", err)
		}
		if !fail && (message.MessageID != "DEMO-MSG-8" || message.SentAt != "2026-11-12T00:00:00Z") {
			t.Fatalf("%+v", message)
		}
		_ = db.Close()
	}
}
