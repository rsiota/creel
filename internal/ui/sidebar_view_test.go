package ui

import "testing"

// TestSidebarItemsBadgeViews verifies sidebarItems flags view entries via
// isView (sourced from the cached m.views set), while base tables stay false.
func TestSidebarItemsBadgeViews(t *testing.T) {
	m := Model{}
	m.tables = []string{"users", "active_users", "orders"}
	m.views = map[string]bool{"active_users": true}

	items := m.sidebarItems()
	got := map[string]bool{}
	for _, it := range items {
		if it.isColumn {
			continue
		}
		got[it.text] = it.isView
	}
	if !got["active_users"] {
		t.Error("view 'active_users' should be badged isView=true")
	}
	if got["users"] || got["orders"] {
		t.Error("base tables should not be badged")
	}
}

// TestSidebarItemsNoViewSetIsSafe confirms a nil view set (e.g. Views() failed)
// does not panic and leaves everything as a base table.
func TestSidebarItemsNoViewSetIsSafe(t *testing.T) {
	m := Model{tables: []string{"a", "b"}, views: nil}
	items := m.sidebarItems()
	for _, it := range items {
		if it.isView {
			t.Errorf("%q unexpectedly badged as view", it.text)
		}
	}
}
