package config

import "testing"

func TestLoadExtraBuckets(t *testing.T) {
	t.Setenv("S3_BUCKET", "default-bucket")
	t.Setenv("S3_BUCKET_DELIVERABLES", "private-bucket")
	t.Setenv("S3_BUCKET_ARCHIVE", "archive-bucket")
	t.Setenv("S3_BUCKET_EMPTY", "")

	got := loadExtraBuckets()

	if got["deliverables"] != "private-bucket" {
		t.Errorf("deliverables = %q, want private-bucket", got["deliverables"])
	}
	if got["archive"] != "archive-bucket" {
		t.Errorf("archive = %q, want archive-bucket", got["archive"])
	}
	// Plain S3_BUCKET is the default and must not appear as a logical bucket.
	if _, ok := got[""]; ok {
		t.Error("empty logical name must not be registered")
	}
	// Empty-valued vars are ignored.
	if _, ok := got["empty"]; ok {
		t.Error("S3_BUCKET_EMPTY with empty value must be ignored")
	}
}
