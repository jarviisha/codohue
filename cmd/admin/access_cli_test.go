package main

import "testing"

func TestAccessCLIRejectsUnsafeInput(t *testing.T) {
	for _, args := range [][]string{nil, {"unknown"}, {"bootstrap", "--password", "secret"}, {"token", "--secret-file", "/missing/secret"}, {"recover", "unexpected"}} {
		if err := runAccessCLI(args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
