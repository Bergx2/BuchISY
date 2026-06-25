package anthropic

import (
	"testing"
)

func TestParseLocateJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantFound bool
		wantPage  int
		wantX0    float64
		wantY0    float64
		wantX1    float64
		wantY1    float64
		wantErr   bool
	}{
		{
			name:      "valid found box",
			input:     `{"found":true,"page":1,"box":[0.1,0.2,0.3,0.25]}`,
			wantFound: true,
			wantPage:  1,
			wantX0:    0.1,
			wantY0:    0.2,
			wantX1:    0.3,
			wantY1:    0.25,
		},
		{
			name:      "fenced json block",
			input:     "```json\n{\"found\":true,\"page\":0,\"box\":[0.05,0.1,0.4,0.15]}\n```",
			wantFound: true,
			wantPage:  0,
			wantX0:    0.05,
			wantY0:    0.1,
			wantX1:    0.4,
			wantY1:    0.15,
		},
		{
			name:      "found false",
			input:     `{"found":false}`,
			wantFound: false,
		},
		{
			name:      "degenerate box x1 <= x0",
			input:     `{"found":true,"page":0,"box":[0.5,0.1,0.3,0.4]}`,
			wantFound: false,
		},
		{
			name:      "degenerate box y1 <= y0",
			input:     `{"found":true,"page":0,"box":[0.1,0.5,0.4,0.3]}`,
			wantFound: false,
		},
		{
			name:    "garbage input",
			input:   `not json at all`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			box, err := parseLocateJSON(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if box.Found != tc.wantFound {
				t.Errorf("Found: got %v, want %v", box.Found, tc.wantFound)
			}
			if !tc.wantFound {
				return
			}
			if box.Page != tc.wantPage {
				t.Errorf("Page: got %d, want %d", box.Page, tc.wantPage)
			}
			if box.X0 != tc.wantX0 {
				t.Errorf("X0: got %v, want %v", box.X0, tc.wantX0)
			}
			if box.Y0 != tc.wantY0 {
				t.Errorf("Y0: got %v, want %v", box.Y0, tc.wantY0)
			}
			if box.X1 != tc.wantX1 {
				t.Errorf("X1: got %v, want %v", box.X1, tc.wantX1)
			}
			if box.Y1 != tc.wantY1 {
				t.Errorf("Y1: got %v, want %v", box.Y1, tc.wantY1)
			}
		})
	}
}
