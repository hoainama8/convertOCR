//go:build cgo

package parser

import "github.com/otiai10/gosseract/v2"

func extractOCRText(imageBytes []byte) (string, error) {
	client := gosseract.NewClient()
	defer client.Close()

	client.SetLanguage("eng", "vie")
	if err := client.SetImageFromBytes(imageBytes); err != nil {
		return "", err
	}
	return client.Text()
}
