package dto

import "testing"

func TestNewProblem(t *testing.T) {
	p := NewProblem(404, "Not Found", "order not found", "/orders/123")

	if p.Type != "about:blank" {
		t.Errorf("Type = %q, want %q", p.Type, "about:blank")
	}
	if p.Title != "Not Found" {
		t.Errorf("Title = %q, want %q", p.Title, "Not Found")
	}
	if p.Status != 404 {
		t.Errorf("Status = %d, want 404", p.Status)
	}
	if p.Detail != "order not found" {
		t.Errorf("Detail = %q, want %q", p.Detail, "order not found")
	}
	if p.Instance != "/orders/123" {
		t.Errorf("Instance = %q, want %q", p.Instance, "/orders/123")
	}
}
