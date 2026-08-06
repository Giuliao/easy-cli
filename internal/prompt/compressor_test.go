package prompt

import "testing"

func TestCompressConvertsMarkdownTableAndDropsDuplicateTitle(t *testing.T) {
	input := "---\nname: demo\ndescription: A demo skill.\n---\n\n# demo\n\n| Component | Path |\n| --- | --- |\n| API | /srv/api |\n| UI | /srv/ui |\n"
	want := "---\nname: demo\ndescription: A demo skill.\n---\n- Component: API; Path: /srv/api\n- Component: UI; Path: /srv/ui\n"

	got, err := Compress(input)
	if err != nil {
		t.Fatalf("Compress() error = %v", err)
	}
	if got != want {
		t.Fatalf("Compress() = %q, want %q", got, want)
	}
}
