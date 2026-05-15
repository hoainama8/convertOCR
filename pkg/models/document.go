package models

import "fmt"

// BlockType defines the type of a content block
type BlockType string

const (
	BlockTypeText    BlockType = "text"
	BlockTypeHeader  BlockType = "header"
	BlockTypeList    BlockType = "list"
	BlockTypeTable   BlockType = "table"
	BlockTypeImage   BlockType = "image"
)

// ContentBlock represents a structural element of a page
type ContentBlock struct {
	Type     BlockType         `json:"type"`
	Content  string            `json:"content"`
	Level    int               `json:"level,omitempty"`    // For headers
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Page represents a single page of a PDF
type Page struct {
	Number int            `json:"number"`
	Blocks []ContentBlock `json:"blocks"`
}

// Document represents the full processed PDF
type Document struct {
	Filename string `json:"filename"`
	Pages    []Page `json:"pages"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

func (d *Document) Summary() string {
	return fmt.Sprintf("Document: %s, Pages: %d", d.Filename, len(d.Pages))
}
