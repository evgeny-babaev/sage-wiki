package prompts

import (
	"strings"
	"testing"
)

func TestRenderAppendsPurposeToAllPromptTypes(t *testing.T) {
	purpose := "Help the team make product and delivery decisions."
	tests := []struct {
		name string
		data any
	}{
		{"summarize_article", SummarizeData{SourcePath: "raw/a.md", SourceType: "article", MaxTokens: 1000}},
		{"extract_concepts", ExtractData{Summaries: "summary"}},
		{"write_article", WriteArticleData{ConceptName: "Test", ConceptID: "test", MaxTokens: 1000}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(tt.name, tt.data, "", purpose)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if !strings.Contains(got, purpose) {
				t.Fatalf("rendered prompt does not contain purpose:\n%s", got)
			}
			if !strings.Contains(got, "The wiki purpose guides selection; it is not source evidence.") {
				t.Fatalf("rendered prompt does not contain purpose contract:\n%s", got)
			}
		})
	}
}

func TestRenderWithoutPurposeIsBackwardCompatible(t *testing.T) {
	data := SummarizeData{SourcePath: "raw/a.md", SourceType: "article", MaxTokens: 1000}
	want, err := Render("summarize_article", data, "Russian")
	if err != nil {
		t.Fatalf("Render legacy call: %v", err)
	}
	got, err := Render("summarize_article", data, "Russian", "  ")
	if err != nil {
		t.Fatalf("Render empty purpose: %v", err)
	}
	if got != want {
		t.Fatalf("empty purpose changed prompt\nwant:\n%s\ngot:\n%s", want, got)
	}
}
