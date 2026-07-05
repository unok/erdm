package fixtures

import (
	"strings"
	"testing"
)

func TestSampleNames_AllLoad(t *testing.T) {
	names := NamesAll()
	if len(names) != len(SampleNames) {
		t.Fatalf("NamesAll length mismatch: %d vs %d", len(names), len(SampleNames))
	}
	for _, name := range names {
		data, err := LoadFixture(name)
		if err != nil {
			t.Fatalf("LoadFixture(%q): %v", name, err)
		}
		if len(data) == 0 {
			t.Fatalf("LoadFixture(%q) returned empty content", name)
		}
	}
}

func TestNamesAll_DefensiveCopy(t *testing.T) {
	got := NamesAll()
	if len(got) == 0 {
		t.Fatalf("NamesAll() should not be empty")
	}
	got[0] = "MUTATED"
	if SampleNames[0] == "MUTATED" {
		t.Fatalf("NamesAll() should return a defensive copy")
	}
}

func TestLoadFixture_NotFound(t *testing.T) {
	_, err := LoadFixture("does_not_exist_in_sample")
	if err == nil {
		t.Fatalf("LoadFixture should fail for missing fixture")
	}
}

func TestFixturePath_Absolute(t *testing.T) {
	path, err := FixturePath("test")
	if err != nil {
		t.Fatalf("FixturePath: %v", err)
	}
	if !strings.HasSuffix(path, "doc/sample/test.erdm") {
		t.Fatalf("FixturePath should end with doc/sample/test.erdm: %s", path)
	}
}
