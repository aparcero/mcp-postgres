package confirm

import (
	"testing"
	"time"
)

func TestManagerIssueAndConsume(t *testing.T) {
	manager := NewManager(2 * time.Minute)
	request := Request{
		ToolName: "postgres.exec_admin",
		Database: "app_db",
		Mode:     "admin",
		SQLHash:  "abc123",
	}

	issued, err := manager.Issue(request)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if issued.Token == "" {
		t.Fatal("Issue() token is empty")
	}

	if err := manager.Consume(issued.Token, request); err != nil {
		t.Fatalf("Consume() error = %v", err)
	}

	if err := manager.Consume(issued.Token, request); err == nil {
		t.Fatal("Consume() second use error = nil, want non-nil")
	}
}

func TestManagerRejectsMismatchedRequest(t *testing.T) {
	manager := NewManager(2 * time.Minute)
	issued, err := manager.Issue(Request{
		ToolName: "postgres.exec_admin",
		Database: "app_db",
		Mode:     "admin",
		SQLHash:  "abc123",
	})
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	err = manager.Consume(issued.Token, Request{
		ToolName: "postgres.exec_admin",
		Database: "other_db",
		Mode:     "admin",
		SQLHash:  "abc123",
	})
	if err == nil {
		t.Fatal("Consume() error = nil, want non-nil")
	}
}

func TestManagerExpiresTokens(t *testing.T) {
	manager := NewManager(5 * time.Millisecond)
	issued, err := manager.Issue(Request{
		ToolName: "postgres.exec_admin",
		Database: "app_db",
		Mode:     "admin",
		SQLHash:  "abc123",
	})
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	if err := manager.Consume(issued.Token, Request{
		ToolName: "postgres.exec_admin",
		Database: "app_db",
		Mode:     "admin",
		SQLHash:  "abc123",
	}); err == nil {
		t.Fatal("Consume() error = nil, want non-nil for expired token")
	}
}
