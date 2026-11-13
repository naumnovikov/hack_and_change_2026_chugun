package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

//go:embed testdata/jira_issues.json
var demoJiraIssuesJSON []byte

type demoJiraIssue struct {
	Key         string `json:"key"`
	AssigneeID  string `json:"assignee_id"`
	Status      string `json:"status"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
}

func completedDemoJiraIssues(assigneeID string) ([]demoJiraIssue, error) {
	assigneeID = strings.TrimSpace(assigneeID)
	if assigneeID == "" {
		return nil, errors.New("demo Jira assignee ID is required")
	}

	decoder := json.NewDecoder(bytes.NewReader(demoJiraIssuesJSON))
	decoder.DisallowUnknownFields()
	var issues []demoJiraIssue
	if err := decoder.Decode(&issues); err != nil {
		return nil, fmt.Errorf("decode demo Jira fixture: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		if err == nil {
			return nil, errors.New("demo Jira fixture contains trailing JSON")
		}
		return nil, fmt.Errorf("decode trailing demo Jira fixture data: %w", err)
	}

	completed := make([]demoJiraIssue, 0, len(issues))
	for _, issue := range issues {
		if issue.Key == "" || issue.AssigneeID == "" || issue.Status == "" ||
			issue.Summary == "" || issue.Description == "" {
			return nil, fmt.Errorf("demo Jira issue %q is incomplete", issue.Key)
		}
		if issue.AssigneeID == assigneeID && issue.Status == "done" {
			completed = append(completed, issue)
		}
	}
	return completed, nil
}
