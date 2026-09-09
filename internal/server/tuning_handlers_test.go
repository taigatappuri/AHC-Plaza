package server

import (
	"bytes"
	"net/http/httptest"
	"testing"
)

func TestTuningRejectsCrossOriginMutation(t *testing.T) {
	s := &Server{}
	for _, origin := range []string{"https://evil.example", "null"} {
		r := httptest.NewRequest("POST", "http://127.0.0.1:8080/api/tuning/environment/setup", bytes.NewBufferString("{}"))
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		s.handleTuning(w, r)
		if w.Code != 403 {
			t.Fatal(w.Code)
		}
	}
}
