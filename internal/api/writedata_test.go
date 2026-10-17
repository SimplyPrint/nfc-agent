package api

import (
	"encoding/json"
	"testing"
)

func TestWriteDataUnmarshal(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{`{"data":"hello"}`, "hello", false},
		{`{"data":"{\"materialName\":\"PLA\"}"}`, `{"materialName":"PLA"}`, false}, // stringified JSON (existing callers)
		{`{"data":{"materialName":"PLA"}}`, `{"materialName":"PLA"}`, false},       // raw object
		{`{"data":[1,2]}`, `[1,2]`, false},
		{`{"data":null}`, "", false},
		{`{}`, "", false},
		{`{"data":42}`, "", true},
	}
	for _, tt := range tests {
		var req struct {
			Data writeData `json:"data"`
		}
		err := json.Unmarshal([]byte(tt.in), &req)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && string(req.Data) != tt.want {
			t.Errorf("%s: data = %q, want %q", tt.in, req.Data, tt.want)
		}
	}
}
