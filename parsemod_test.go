package main

import (
	"reflect"
	"testing"
)

func TestParseGoMod(t *testing.T) {
	tests := []struct {
		name     string
		modPath  string
		expected []string
		wantErr  bool
	}{
		{
			name:    "parse test.mod with mixed format",
			modPath: "testdata/test.mod",
			expected: []string{
				"golang.org/x/sync",
				"golang.org/x/net",
				"github.com/pkg/errors",
				"github.com/google/uuid",
				"golang.org/x/crypto",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseGoMod(tt.modPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseGoMod() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("parseGoMod() = %v, want %v", got, tt.expected)
			}
		})
	}
}
