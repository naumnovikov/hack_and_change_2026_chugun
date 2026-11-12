package main

import "testing"

func TestOpenStoreErrors(t *testing.T) {
	for _, dsn := range []string{"", "://bad", "postgres://demo:demo@127.0.0.1:1/demo?sslmode=disable&connect_timeout=1"} {
		if _, err := openStore(dsn); err == nil {
			t.Fatal("expected startup failure")
		}
	}
}
