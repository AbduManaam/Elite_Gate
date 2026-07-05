package handler

import (
	"testing"

	"elitegate/internal/model"
)

func TestProjectHandler_ApplyRoleBasedFields(t *testing.T) {
	h := &ProjectHandler{}

	plan := "enterprise"
	projectID := "8b1e2b8a-2222-4b31-9a35-6b6b6a2f9c10"

	// 1. Test Owner view
	summaryOwner := &model.ProjectSummary{
		IsActive: true,
		Plan:     &plan,
	}
	h.applyRoleBasedFields(summaryOwner, "owner", projectID)

	if summaryOwner.Plan == nil || *summaryOwner.Plan != "enterprise" {
		t.Errorf("expected plan to be enterprise, got %v", summaryOwner.Plan)
	}
	if summaryOwner.Subscription == nil {
		t.Fatal("expected subscription to be populated for owner")
	}
	if summaryOwner.Subscription.Plan != "enterprise" {
		t.Errorf("expected subscription plan to be enterprise, got %s", summaryOwner.Subscription.Plan)
	}
	if summaryOwner.Subscription.Status != "active" {
		t.Errorf("expected subscription status to be active, got %s", summaryOwner.Subscription.Status)
	}

	// 2. Test Editor/Viewer view
	summaryEditor := &model.ProjectSummary{
		IsActive: true,
		Plan:     &plan,
	}
	h.applyRoleBasedFields(summaryEditor, "editor", projectID)

	if summaryEditor.Plan != nil {
		t.Errorf("expected plan to be nil for editor, got %v", summaryEditor.Plan)
	}
	if summaryEditor.Subscription != nil {
		t.Error("expected subscription to be nil for editor")
	}
}
