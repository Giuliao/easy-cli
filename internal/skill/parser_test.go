package skill

import "testing"

func TestParseReadsMetadataAndBody(t *testing.T) {
	source := "---\nname: demo\ndescription: A demo skill.\n---\n\n# Demo\n\nFollow the rule.\n"

	got, err := Parse(source)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Name != "demo" {
		t.Fatalf("Name = %q, want %q", got.Name, "demo")
	}
	if got.Description != "A demo skill." {
		t.Fatalf("Description = %q, want %q", got.Description, "A demo skill.")
	}
	if got.Source != source {
		t.Fatalf("Source changed during parsing")
	}
	if got.Body != "# Demo\n\nFollow the rule.\n" {
		t.Fatalf("Body = %q, want %q", got.Body, "# Demo\n\nFollow the rule.\n")
	}
}

func TestParseRejectsMissingDescription(t *testing.T) {
	_, err := Parse("---\nname: demo\n---\nbody\n")
	if err == nil {
		t.Fatal("Parse() error = nil, want missing description error")
	}
}
