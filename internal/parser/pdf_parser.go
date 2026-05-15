package parser

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"regexp"
	"strconv"
	"strings"

	"opendataloader-go/pkg/models"

	"github.com/gen2brain/go-fitz"
)

var multiSpaceRe = regexp.MustCompile(`\s{2,}`)

// PDFParser handles the extraction of text from PDF files
type PDFParser struct {
	FilePath string
}

type blockExtractor struct {
	lines            []string
	blocks           []models.ContentBlock
	currentTextBlock strings.Builder
	leftColumn       []string
	rightColumn      []string
}

func NewPDFParser(filePath string) *PDFParser {
	return &PDFParser{FilePath: filePath}
}

// Parse extracts content from the PDF and returns a Document model
func (p *PDFParser) Parse() (*models.Document, error) {
	// Open document with go-fitz (primary engine)
	doc, err := fitz.New(p.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF with fitz: %w", err)
	}
	defer doc.Close()

	document := &models.Document{
		Filename: p.FilePath,
		Pages:    make([]models.Page, 0),
	}

	totalPage := doc.NumPage()
	for pageNum := 0; pageNum < totalPage; pageNum++ {
		text, err := doc.Text(pageNum)

		// If text is empty or extraction failed, it might be a scanned document, try OCR
		if err != nil || strings.TrimSpace(text) == "" {
			fmt.Printf("Page %d seems to be an image or empty, performing OCR...\n", pageNum+1)
			ocrText, ocrErr := p.performOCR(doc, pageNum)
			if ocrErr == nil {
				text = ocrText
			} else {
				fmt.Fprintf(os.Stderr, "OCR failed for page %d: %v\n", pageNum+1, ocrErr)
			}
		}

		pModel := models.Page{
			Number: pageNum + 1,
			Blocks: p.extractBlocks(text),
		}
		document.Pages = append(document.Pages, pModel)
	}

	return document, nil
}

func (p *PDFParser) performOCR(doc *fitz.Document, pageNum int) (string, error) {
	// Render page to image (PNG)
	img, err := doc.Image(pageNum)
	if err != nil {
		return "", err
	}

	// Encode image to bytes
	buf := new(bytes.Buffer)
	if err := png.Encode(buf, img); err != nil {
		return "", err
	}

	return extractOCRText(buf.Bytes())
}

// extractBlocks is a basic heuristic-based layout analyzer
func (p *PDFParser) extractBlocks(text string) []models.ContentBlock {
	return newBlockExtractor(text).Extract()
}

func newBlockExtractor(text string) *blockExtractor {
	return &blockExtractor{
		lines:  strings.Split(text, "\n"),
		blocks: make([]models.ContentBlock, 0),
	}
}

func (e *blockExtractor) Extract() []models.ContentBlock {
	for i := 0; i < len(e.lines); i++ {
		line := e.lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			e.flushText()
			e.flushColumns()
			continue
		}

		if rows, ok := detectTable(e.lines, i); ok {
			e.flushText()
			e.flushColumns()
			e.blocks = append(e.blocks, models.ContentBlock{
				Type:    models.BlockTypeTable,
				Content: renderTable(rows),
				Metadata: map[string]string{
					"table_cols": strconv.Itoa(len(rows[0])),
					"table_rows": strconv.Itoa(len(rows)),
				},
			})
			i += len(rows) - 1
			continue
		}

		if l, r, ok := splitTwoColumns(line); ok {
			e.flushText()
			e.leftColumn = append(e.leftColumn, l)
			e.rightColumn = append(e.rightColumn, r)
			continue
		}
		e.flushColumns()

		if e.isHeader(trimmed) {
			e.flushText()
			e.blocks = append(e.blocks, models.ContentBlock{
				Type:    models.BlockTypeHeader,
				Content: trimmed,
				Level:   e.headerLevel(trimmed),
			})
			continue
		}

		if e.isListItem(trimmed) {
			e.flushText()
			e.blocks = append(e.blocks, models.ContentBlock{
				Type:    models.BlockTypeList,
				Content: trimmed,
			})
			continue
		}

		e.appendParagraphLine(trimmed)
	}

	e.flushText()
	e.flushColumns()
	return e.blocks
}

func (e *blockExtractor) flushText() {
	if e.currentTextBlock.Len() > 0 {
		e.blocks = append(e.blocks, models.ContentBlock{
			Type:    models.BlockTypeText,
			Content: strings.TrimSpace(e.currentTextBlock.String()),
		})
		e.currentTextBlock.Reset()
	}
}

func (e *blockExtractor) flushColumns() {
	if len(e.leftColumn) == 0 && len(e.rightColumn) == 0 {
		return
	}

	if len(e.leftColumn) > 0 {
		e.blocks = append(e.blocks, models.ContentBlock{
			Type:    models.BlockTypeText,
			Content: strings.Join(e.leftColumn, "\n"),
			Metadata: map[string]string{
				"source": "column-left",
			},
		})
	}
	if len(e.rightColumn) > 0 {
		e.blocks = append(e.blocks, models.ContentBlock{
			Type:    models.BlockTypeText,
			Content: strings.Join(e.rightColumn, "\n"),
			Metadata: map[string]string{
				"source": "column-right",
			},
		})
	}

	e.leftColumn = nil
	e.rightColumn = nil
}

func (e *blockExtractor) isHeader(line string) bool {
	isAllCaps := line == strings.ToUpper(line) && len(line) > 3
	return len(line) < 60 && isAllCaps
}

func (e *blockExtractor) headerLevel(line string) int {
	if len(line) < 30 {
		return 2
	}
	return 1
}

func (e *blockExtractor) isListItem(line string) bool {
	return strings.HasPrefix(line, "- ") ||
		strings.HasPrefix(line, "* ") ||
		(len(line) > 2 && line[1] == '.' && line[0] >= '0' && line[0] <= '9')
}

func (e *blockExtractor) appendParagraphLine(line string) {
	e.currentTextBlock.WriteString(line)
	if !strings.HasSuffix(line, ".") && !strings.HasSuffix(line, ":") && !strings.HasSuffix(line, "!") && !strings.HasSuffix(line, "?") {
		e.currentTextBlock.WriteString(" ")
		return
	}
	e.flushText()
}

func splitTwoColumns(line string) (string, string, bool) {
	parts := multiSpaceRe.Split(strings.TrimSpace(line), -1)
	if len(parts) != 2 {
		return "", "", false
	}

	left := strings.TrimSpace(parts[0])
	right := strings.TrimSpace(parts[1])

	if len(left) < 12 || len(right) < 12 {
		return "", "", false
	}

	return left, right, true
}

func detectTable(lines []string, start int) ([][]string, bool) {
	rows := make([][]string, 0)
	width := 0

	for i := start; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			break
		}

		cells := splitTableCells(trimmed)
		if len(cells) < 2 {
			break
		}

		if width == 0 {
			width = len(cells)
		}

		// Keep table rows consistent; stop on layout drift.
		if len(cells) != width {
			break
		}

		rows = append(rows, cells)
	}

	if len(rows) < 2 {
		return nil, false
	}

	return rows, true
}

func splitTableCells(line string) []string {
	parts := multiSpaceRe.Split(line, -1)
	cells := make([]string, 0, len(parts))
	for _, p := range parts {
		clean := strings.TrimSpace(p)
		if clean != "" {
			cells = append(cells, clean)
		}
	}
	return cells
}

func renderTable(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}

	var b strings.Builder
	// Use first row as header to preserve a deterministic table shape in markdown.
	b.WriteString("| " + strings.Join(rows[0], " | ") + " |\n")

	sep := make([]string, len(rows[0]))
	for i := range sep {
		sep[i] = "---"
	}
	b.WriteString("| " + strings.Join(sep, " | ") + " |\n")

	for i := 1; i < len(rows); i++ {
		b.WriteString("| " + strings.Join(rows[i], " | ") + " |\n")
	}

	return strings.TrimRight(b.String(), "\n")
}
