//go:build !cgo

package parser

import "fmt"

func extractOCRText(_ []byte) (string, error) {
	return "", fmt.Errorf("ocr requires cgo/tesseract and is not available in this build")
}
