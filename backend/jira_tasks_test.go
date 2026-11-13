package main

import (
	"reflect"
	"testing"
)

func TestCompletedDemoJiraIssuesFiltersByAssigneeAndStatus(t *testing.T) {
	tests := []struct {
		name       string
		assigneeID string
		wantKeys   []string
	}{
		{
			name:       "demo intern gets only completed assigned tasks",
			assigneeID: "demo-intern-001",
			wantKeys:   []string{"DEMO-101", "DEMO-102", "DEMO-103"},
		},
		{
			name:       "other demo intern gets only their completed task",
			assigneeID: "demo-intern-002",
			wantKeys:   []string{"DEMO-105"},
		},
		{
			name:       "unknown user gets no tasks",
			assigneeID: "demo-intern-unknown",
			wantKeys:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues, err := completedDemoJiraIssues(tt.assigneeID)
			if err != nil {
				t.Fatal(err)
			}
			gotKeys := make([]string, len(issues))
			for i, issue := range issues {
				gotKeys[i] = issue.Key
				if issue.Status != "done" || issue.AssigneeID != tt.assigneeID {
					t.Errorf("returned unqualified issue: %+v", issue)
				}
			}
			if !reflect.DeepEqual(gotKeys, tt.wantKeys) {
				t.Fatalf("issue keys = %v, want %v", gotKeys, tt.wantKeys)
			}
		})
	}
}

func TestCompletedDemoJiraIssuesRequiresAssignee(t *testing.T) {
	if _, err := completedDemoJiraIssues(" "); err == nil {
		t.Fatal("expected an error for an empty assignee ID")
	}
}
