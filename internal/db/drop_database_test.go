package db

import "testing"

func TestBuildDropDatabaseSQL(t *testing.T) {
	tests := []struct {
		driver Driver
		name   string
		want   string
		err    bool
	}{
		{DriverMySQL, "mydb", "DROP DATABASE `mydb`", false},
		{DriverMySQL, "", "", true},
		{DriverMySQL, "  ", "", true},
		{DriverMySQL, "mysql", "", true},
		{DriverMySQL, "information_schema", "", true},
		{DriverMySQL, "performance_schema", "", true},
		{DriverMySQL, "sys", "", true},
		{DriverMySQL, "MYSQL", "", true},
		{DriverSQLite, "mydb", "", true},
	}
	for _, tc := range tests {
		got, err := BuildDropDatabaseSQL(tc.driver, tc.name)
		if tc.err {
			if err == nil {
				t.Errorf("BuildDropDatabaseSQL(%q, %q) expected error, got %q", tc.driver, tc.name, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("BuildDropDatabaseSQL(%q, %q) unexpected error: %v", tc.driver, tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("BuildDropDatabaseSQL(%q, %q) = %q, want %q", tc.driver, tc.name, got, tc.want)
		}
	}
}
