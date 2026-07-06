package store

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/dchooyc/book"
)

// ReadJSONFile decodes a JSON file into v.
func ReadJSONFile(path string, v interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("unmarshal %s: %w", path, err)
	}
	return nil
}

// WriteJSONFileAtomic writes v as indented JSON via a temp file and rename.
func WriteJSONFileAtomic(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename %s: %w", tmp, err)
	}
	return nil
}

// ReadBooksFile loads a book.Books JSON file (the crawler output format).
func ReadBooksFile(path string) (*book.Books, error) {
	var books book.Books
	if err := ReadJSONFile(path, &books); err != nil {
		return nil, err
	}
	return &books, nil
}
