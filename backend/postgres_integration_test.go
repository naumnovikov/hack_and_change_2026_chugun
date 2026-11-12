package main

import (
	"context"
	"os"
	"testing"
)

// Run against an isolated synthetic database: TEST_DATABASE_URL=... go test -run TestPostgresIntegration -v.
func TestPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required for real Postgres integration")
	}
	store, err := openStore(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	ctx := context.Background()
	access, err := store.CreateAccess(ctx, "DEMO-Jira")
	if err != nil {
		t.Fatal(err)
	}
	message, err := store.SendMessage(ctx, "demo-employee-001", "Review DEMO-42")
	if err != nil {
		t.Fatal(err)
	}
	// Query independently rather than relying on INSERT RETURNING alone.
	var service, status, text string
	if err = store.DB.QueryRowContext(ctx, "SELECT service_name,status FROM mock_access_requests WHERE id=$1", access.TicketID[len("DEMO-ACCESS-"):]).Scan(&service, &status); err != nil {
		t.Fatal(err)
	}
	if service != "DEMO-Jira" || status != access.Status {
		t.Fatal("persisted access differs")
	}
	if err = store.DB.QueryRowContext(ctx, "SELECT text FROM mock_messages WHERE id=$1", message.MessageID[len("DEMO-MSG-"):]).Scan(&text); err != nil {
		t.Fatal(err)
	}
	if text != "Review DEMO-42" {
		t.Fatal("persisted message differs")
	}
	defer store.DB.ExecContext(ctx, "DELETE FROM mock_access_requests WHERE id=$1", access.TicketID[len("DEMO-ACCESS-"):])
	defer store.DB.ExecContext(ctx, "DELETE FROM mock_messages WHERE id=$1", message.MessageID[len("DEMO-MSG-"):])
}
