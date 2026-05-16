package models

// PlaceholderKeyPrefix is the S3 prefix for default placeholder objects (one set per process, idempotent).
const PlaceholderKeyPrefix = "placeholders"

// PlaceholderOriginalKey returns the S3 object key for the placeholder original.
// ext is the lowercase file extension without leading dot (e.g. "png", "jpg", "webp").
// Returns "" when ext is empty so callers can skip uploads when the placeholder is unconfigured.
func PlaceholderOriginalKey(ext string) string {
	if ext == "" {
		return ""
	}
	return PlaceholderKeyPrefix + "/original." + ext
}

// PlaceholderThumbnailKey returns the S3 object key for a placeholder thumbnail variant.
// Returns "" when format is not one of [ThumbnailFormats]; callers must skip empty keys.
func PlaceholderThumbnailKey(label, format string) string {
	ext, ok := ThumbnailFileExtByFormat[format]
	if !ok {
		return ""
	}
	return PlaceholderKeyPrefix + "/" + label + "." + ext
}
