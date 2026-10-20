package db

import "testing"

func TestConnectRequiresDatabaseURL(t *testing.T) {
	if _, err := Connect("  "); err == nil {
		t.Fatal("Connect accepted a blank database URL")
	}
}
