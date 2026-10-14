package keys

import "testing"

func TestParseLinesKeepsOpaqueCredentials(t *testing.T) {
	items := ParseLines("not-a-provider-key\nAK|SK|Region\nhttps://example.test|secret")
	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(items))
	}
	wants := []string{"not-a-provider-key", "AK|SK|Region", "https://example.test|secret"}
	for index, want := range wants {
		if items[index].Error != "" {
			t.Fatalf("item %d unexpected error: %s", index, items[index].Error)
		}
		if items[index].Normalized != want {
			t.Fatalf("item %d normalized = %q, want %q", index, items[index].Normalized, want)
		}
		if items[index].Fingerprint == "" {
			t.Fatalf("item %d fingerprint should be set", index)
		}
	}
}

func TestParseLinesSkipsBlankAndMarksDuplicate(t *testing.T) {
	items := ParseLines(" raw-value \n\nraw-value\r\n")
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if !items[1].Duplicate {
		t.Fatal("second item should be duplicate")
	}
}
