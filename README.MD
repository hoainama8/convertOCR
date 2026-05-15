# PDFtoMD

## Giới thiệu
`PDFtoMD` là dự án chuyển đổi nội dung từ file PDF sang nhiều định dạng đầu ra khác nhau để dễ đọc, tái sử dụng dữ liệu và tích hợp vào các hệ thống khác.

Dự án có giao diện web đơn giản để tải lên một hoặc nhiều file PDF, chọn định dạng đầu ra, sau đó nhận file kết quả tự động.

## Công dụng chính
- Trích xuất nội dung văn bản từ PDF.
- Hỗ trợ nhận diện tài liệu scan thông qua OCR (khi môi trường có hỗ trợ phù hợp).
- Chuyển đổi nhanh sang nhiều định dạng phổ biến để phục vụ:
  - Lưu trữ tài liệu
  - Chia sẻ nội dung
  - Xử lý dữ liệu tự động
  - Tích hợp hệ thống

## Các định dạng có thể chuyển đổi
Dự án hiện hỗ trợ xuất PDF sang các định dạng sau:
- `Markdown (.md)`
- `JSON (.json)`
- `HTML (.html)`
- `Word (.docx)`
- `CSV (.csv)`

## Cách chạy nhanh
1. Chạy server:
```bash
go run ./cmd/server
```

2. Mở trình duyệt và truy cập địa chỉ mà server in ra (thường là `http://localhost:<port>`).

3. Tải lên file PDF, chọn định dạng đầu ra, nhấn chuyển đổi để tải file kết quả.


Lưu ý: một số tính năng OCR/phụ thuộc thư viện hệ thống có thể cần bản build `cgo` hoặc thiết lập môi trường phù hợp.

## Mời tôi ly cà phê
Nếu bạn thấy phần mềm hữu ích, hãy mời tôi ly cà phê. Cảm ơn nhiều!

![Mời tôi ly cà phê](IMG_0726.JPG)
