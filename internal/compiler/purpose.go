package compiler

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const PurposeFilename = "purpose.md"

var purposeCommentRE = regexp.MustCompile(`(?s)<!--.*?-->`)

// Purpose is the optional compilation objective loaded from purpose.md.
// Missing, empty, and comment-only files all disable purpose-aware behavior.
type Purpose struct {
	Text string
	Hash string
}

func (p Purpose) Enabled() bool {
	return p.Text != ""
}

// LoadPurpose reads and normalizes the optional project-level purpose.md.
func LoadPurpose(projectDir string) (Purpose, error) {
	data, err := os.ReadFile(filepath.Join(projectDir, PurposeFilename))
	if err != nil {
		if os.IsNotExist(err) {
			return Purpose{}, nil
		}
		return Purpose{}, fmt.Errorf("load purpose: %w", err)
	}

	text := strings.TrimSpace(purposeCommentRE.ReplaceAllString(string(data), ""))
	if text == "" {
		return Purpose{}, nil
	}
	sum := sha256.Sum256([]byte(text))
	return Purpose{Text: text, Hash: fmt.Sprintf("sha256:%x", sum[:])}, nil
}
