package main

import (
	"testing"
)

func TestWriteCache(t *testing.T) {
	result := writeToCache("leader", "nodex")
	if result != true {
		t.Fatalf(`Failed to write to cache`)
	}
}
