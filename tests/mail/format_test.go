package mail_test

import (
	"encoding/json"
	"fmt"
	"testing"
)

func sprint(format string, v any) string { return fmt.Sprintf(format, v) }

func sprintJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
