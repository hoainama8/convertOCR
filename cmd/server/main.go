package main

import (
	"archive/zip"
	"bytes"
	"embed"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"opendataloader-go/internal/parser"
	"opendataloader-go/internal/serializer"
	"opendataloader-go/pkg/models"
)

//go:embed templates/index.html
var templatesFS embed.FS

var indexTemplate = template.Must(template.ParseFS(templatesFS, "templates/index.html"))

type converterApp struct {
	addr           string
	parserFactory  DocumentParserFactory
	formatResolver FormatResolver
	convertSlots   chan struct{}
}

type convertedFile struct {
	Filename string
	Data     []byte
}

type DocumentParser interface {
	Parse() (*models.Document, error)
}

type DocumentParserFactory interface {
	New(filePath string) DocumentParser
}

type PDFDocumentParserFactory struct{}

func (f *PDFDocumentParserFactory) New(filePath string) DocumentParser {
	return parser.NewPDFParser(filePath)
}

type FormatResolver interface {
	Resolve(format string) (serializer.Serializer, string, string, error)
}

type DefaultFormatResolver struct{}

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	maxConcurrent := flag.Int("max-concurrent", runtime.NumCPU(), "Maximum number of concurrent conversion jobs")
	flag.Parse()

	app := newConverterApp(*addr, *maxConcurrent)
	http.HandleFunc("/", app.indexHandler)
	http.HandleFunc("/convert", app.convertHandler)

	log.Printf("Starting converter server at http://localhost%s", app.addr)
	if err := http.ListenAndServe(app.addr, nil); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func newConverterApp(addr string, maxConcurrent int) *converterApp {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}

	return &converterApp{
		addr:           addr,
		parserFactory:  &PDFDocumentParserFactory{},
		formatResolver: &DefaultFormatResolver{},
		convertSlots:   make(chan struct{}, maxConcurrent),
	}
}

func (a *converterApp) indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	if err := indexTemplate.Execute(w, nil); err != nil {
		http.Error(w, "Unable to render page", http.StatusInternalServerError)
		log.Printf("template execute error: %v", err)
	}
}

func (a *converterApp) convertHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Allow multiple conversion jobs in parallel up to configured capacity.
	if !a.tryAcquireConvertSlot() {
		http.Error(w, "Converter is busy. Please try again in a moment.", http.StatusLocked)
		return
	}
	defer a.releaseConvertSlot()

	if err := r.ParseMultipartForm(200 << 20); err != nil {
		http.Error(w, "Failed to parse upload", http.StatusBadRequest)
		return
	}

	format := strings.ToLower(r.FormValue("format"))
	if format == "" {
		format = "md"
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		http.Error(w, "Please choose at least one PDF file", http.StatusBadRequest)
		return
	}

	s, extension, contentType, err := a.formatResolver.Resolve(format)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	results := make([]convertedFile, 0, len(files))
	for _, fh := range files {
		uploaded, err := saveUploadedFile(fh)
		if err != nil {
			http.Error(w, fmt.Sprintf("Cannot save uploaded file: %v", err), http.StatusInternalServerError)
			return
		}
		defer os.Remove(uploaded)

		p := a.parserFactory.New(uploaded)
		doc, err := p.Parse()
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to parse %s: %v", fh.Filename, err), http.StatusBadRequest)
			return
		}

		outputBytes, err := s.Serialize(doc)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to serialize %s: %v", fh.Filename, err), http.StatusInternalServerError)
			return
		}

		outName := strings.TrimSuffix(fh.Filename, filepath.Ext(fh.Filename)) + "." + extension
		results = append(results, convertedFile{Filename: outName, Data: outputBytes})
	}

	if len(results) == 1 {
		single := results[0]
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", single.Filename))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(single.Data)))
		_, _ = w.Write(single.Data)
		return
	}

	zipBytes, err := createZipArchive(results)
	if err != nil {
		http.Error(w, "Failed to create zip archive", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=converted-files.zip")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(zipBytes)))
	_, _ = w.Write(zipBytes)
}

func (a *converterApp) tryAcquireConvertSlot() bool {
	select {
	case a.convertSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (a *converterApp) releaseConvertSlot() {
	<-a.convertSlots
}

func saveUploadedFile(fileHeader *multipart.FileHeader) (string, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer file.Close()

	tmpFile, err := os.CreateTemp(os.TempDir(), "upload-*.pdf")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, file); err != nil {
		return "", err
	}

	return tmpFile.Name(), nil
}

func (r *DefaultFormatResolver) Resolve(format string) (serializer.Serializer, string, string, error) {
	var s serializer.Serializer
	var extension string
	var contentType string

	switch format {
	case "json":
		s = &serializer.JSONSerializer{}
		extension = "json"
		contentType = "application/json; charset=utf-8"
	case "html":
		s = &serializer.HTMLSerializer{}
		extension = "html"
		contentType = "text/html; charset=utf-8"
	case "docx":
		s = &serializer.DocxSerializer{}
		extension = "docx"
		contentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "csv":
		s = &serializer.CSVSerializer{}
		extension = "csv"
		contentType = "text/csv; charset=utf-8"
	case "md", "markdown":
		s = &serializer.MarkdownSerializer{}
		extension = "md"
		contentType = "text/markdown; charset=utf-8"
	default:
		return nil, "", "", fmt.Errorf("Unsupported format: %s", format)
	}

	return s, extension, contentType, nil
}

func createZipArchive(files []convertedFile) ([]byte, error) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for _, file := range files {
		entry, err := zw.Create(file.Filename)
		if err != nil {
			zw.Close()
			return nil, err
		}
		if _, err := entry.Write(file.Data); err != nil {
			zw.Close()
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
