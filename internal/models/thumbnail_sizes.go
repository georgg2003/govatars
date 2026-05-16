package models

const (
	ThumbnailFormatJPEG = "jpeg"
	ThumbnailFormatPNG  = "png"
	ThumbnailFormatWEBP = "webp"
)

// ThumbnailFormats is the persisted order for thumbnail generation and lookup.
var ThumbnailFormats = []string{ThumbnailFormatJPEG, ThumbnailFormatPNG, ThumbnailFormatWEBP}

// ThumbnailContentTypeByFormat maps API format to storage content-type.
var ThumbnailContentTypeByFormat = map[string]string{
	ThumbnailFormatJPEG: "image/jpeg",
	ThumbnailFormatPNG:  "image/png",
	ThumbnailFormatWEBP: "image/webp",
}

// ThumbnailFileExtByFormat maps API format to file extension used in S3 object keys.
var ThumbnailFileExtByFormat = map[string]string{
	ThumbnailFormatJPEG: "jpg",
	ThumbnailFormatPNG:  "png",
	ThumbnailFormatWEBP: "webp",
}
