package serializer

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"opendataloader-go/pkg/models"

	docxgo "github.com/mmonterroca/docxgo/v2"
)

// Serializer defines the interface for different output formats
type Serializer interface {
	Serialize(doc *models.Document) ([]byte, error)
}

// MarkdownSerializer converts Document to Markdown
type MarkdownSerializer struct{}

func (s *MarkdownSerializer) Serialize(doc *models.Document) ([]byte, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", doc.Filename))

	for _, page := range doc.Pages {
		for _, block := range page.Blocks {
			switch block.Type {
			case models.BlockTypeHeader:
				sb.WriteString(fmt.Sprintf("%s %s\n\n", strings.Repeat("#", block.Level+1), block.Content))
			case models.BlockTypeList:
				sb.WriteString(fmt.Sprintf("%s\n", block.Content))
			case models.BlockTypeText:
				sb.WriteString(fmt.Sprintf("%s\n\n", normalizeTextBlock(block.Content)))
			case models.BlockTypeTable:
				sb.WriteString(fmt.Sprintf("%s\n\n", tableToBulletList(block.Content)))
			}
		}
	}
	return []byte(sb.String()), nil
}

// JSONSerializer converts Document to JSON
type JSONSerializer struct{}

func (s *JSONSerializer) Serialize(doc *models.Document) ([]byte, error) {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return data, nil
}

// HTMLSerializer converts Document to semantic HTML
type HTMLSerializer struct{}

func (s *HTMLSerializer) Serialize(doc *models.Document) ([]byte, error) {
	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html>\n<html>\n<head>\n")
	sb.WriteString(fmt.Sprintf("<title>%s</title>\n", doc.Filename))
	sb.WriteString("</head>\n<body>\n")

	for _, page := range doc.Pages {
		sb.WriteString(fmt.Sprintf("<section class=\"page\" data-page=\"%d\">\n", page.Number))
		for _, block := range page.Blocks {
			switch block.Type {
			case models.BlockTypeHeader:
				sb.WriteString(fmt.Sprintf("<h%d>%s</h%d>\n", block.Level+1, block.Content, block.Level+1))
			case models.BlockTypeList:
				sb.WriteString(fmt.Sprintf("<li>%s</li>\n", block.Content))
			case models.BlockTypeText:
				sb.WriteString(fmt.Sprintf("<p>%s</p>\n", block.Content))
			case models.BlockTypeTable:
				sb.WriteString("<pre>")
				sb.WriteString(block.Content)
				sb.WriteString("</pre>\n")
			}
		}
		sb.WriteString("</section>\n")
	}

	sb.WriteString("</body>\n</html>")
	return []byte(sb.String()), nil
}

// DocxSerializer converts Document to Word (.docx)
type DocxSerializer struct{}

func (s *DocxSerializer) Serialize(doc *models.Document) ([]byte, error) {
	docxFile := docxgo.NewDocument()

	// Title
	para, _ := docxFile.AddParagraph()
	run, _ := para.AddRun()
	run.SetText(doc.Filename)
	run.SetBold(true)

	for _, page := range doc.Pages {
		for _, block := range page.Blocks {
			para, _ = docxFile.AddParagraph()
			switch block.Type {
			case models.BlockTypeHeader:
				run, _ = para.AddRun()
				run.SetText(block.Content)
				run.SetBold(true)
			case models.BlockTypeList:
				run, _ = para.AddRun()
				run.SetText("• " + block.Content)
			case models.BlockTypeText, models.BlockTypeTable:
				run, _ = para.AddRun()
				run.SetText(block.Content)
			}
		}
	}

	// Create a temporary file to save the docx content
	tmpFile, err := os.CreateTemp("", "output-*.docx")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close() // Close immediately as SaveAs will open it
	defer os.Remove(tmpPath)

	// Save to the temporary file
	if err := docxFile.SaveAs(tmpPath); err != nil {
		return nil, fmt.Errorf("failed to save docx: %w", err)
	}

	// Read the content back
	content, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read temp file: %w", err)
	}

	return content, nil
}

// CSVSerializer converts Document to CSV
type CSVSerializer struct{}

func (s *CSVSerializer) Serialize(doc *models.Document) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Header row
	if err := writer.Write([]string{"Page", "Type", "Content"}); err != nil {
		return nil, err
	}

	for _, page := range doc.Pages {
		pageStr := fmt.Sprintf("%d", page.Number)
		for _, block := range page.Blocks {
			// Basic row
			row := []string{pageStr, string(block.Type), block.Content}

			// Special handling for tables to split into columns if possible
			if block.Type == models.BlockTypeTable {
				tableRows := extractMarkdownTableRows(block.Content)
				for _, tr := range tableRows {
					tableRow := append([]string{pageStr, "table_row"}, tr...)
					if err := writer.Write(tableRow); err != nil {
						return nil, err
					}
				}
				continue
			}

			if err := writer.Write(row); err != nil {
				return nil, err
			}
		}
	}

	writer.Flush()
	return buf.Bytes(), nil
}

func extractMarkdownTableRows(content string) [][]string {
	lines := strings.Split(content, "\n")
	rows := make([][]string, 0)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || !strings.Contains(trimmed, "|") {
			continue
		}

		// Skip markdown separator line.
		if i > 0 && strings.Contains(trimmed, "---") && strings.Count(trimmed, "|") >= 2 {
			continue
		}

		row := parseMarkdownTableLine(trimmed)
		if len(row) > 0 {
			rows = append(rows, row)
		}
	}

	return rows
}

func parseMarkdownTableLine(line string) []string {
	parts := strings.Split(line, "|")
	cells := make([]string, 0, len(parts))
	for _, part := range parts {
		cell := strings.TrimSpace(part)
		if cell != "" {
			cells = append(cells, cell)
		}
	}
	return cells
}

func normalizeTextBlock(content string) string {
	lines := strings.Split(content, "\n")
	cleaned := make([]string, 0, len(lines))

	for _, l := range lines {
		line := strings.TrimSpace(l)
		if line == "" {
			continue
		}
		cleaned = append(cleaned, line)
	}

	if len(cleaned) == 0 {
		return ""
	}

	return strings.Join(cleaned, "\n")
}

func tableToBulletList(content string) string {
	rows := extractMarkdownTableRows(content)
	if len(rows) == 0 {
		return ""
	}

	out := make([]string, 0, len(rows))
	// Skip header row to avoid duplicated schema-like line in bullets.
	start := 1
	if len(rows) == 1 {
		start = 0
	}

	for i := start; i < len(rows); i++ {
		out = append(out, "- "+strings.Join(rows[i], " | "))
	}

	return strings.Join(out, "\n")
}
