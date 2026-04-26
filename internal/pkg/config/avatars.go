package config

import (
	"errors"
	"fmt"
	"strings"
)

// ThumbnailPreset is one square thumbnail variant (label matches OpenAPI size enum values).
type ThumbnailPreset struct {
	Label  string `mapstructure:"label"`
	Pixels int    `mapstructure:"pixels"`
}

// Avatars configures upload limits and derived thumbnail generation.
type Avatars struct {
	MaxUploadBytes    int64             `mapstructure:"max_upload_bytes"`
	ImageCacheControl string            `mapstructure:"image_cache_control"` // Cache-Control for GET avatar image responses (e.g. max-age=86400)
	Thumbnails        []ThumbnailPreset `mapstructure:"thumbnails"`
}

// ThumbnailCatalog is resolved labels + lookup map for workers and HTTP metadata.
type ThumbnailCatalog struct {
	Labels []string
	Sides  map[string]int
}

// Catalog builds an ordered catalog from [Avatars.Thumbnails] after defaults are applied.
func (a Avatars) Catalog() (ThumbnailCatalog, error) {
	if len(a.Thumbnails) == 0 {
		return ThumbnailCatalog{}, errors.New("avatars: no thumbnails configured")
	}
	labels := make([]string, 0, len(a.Thumbnails))
	sides := make(map[string]int, len(a.Thumbnails))
	seen := make(map[string]struct{}, len(a.Thumbnails))
	for _, t := range a.Thumbnails {
		if t.Label == "" {
			return ThumbnailCatalog{}, errors.New("avatars: empty thumbnail label")
		}
		if t.Pixels <= 0 {
			return ThumbnailCatalog{}, fmt.Errorf("avatars: thumbnail %q must have pixels > 0", t.Label)
		}
		if _, ok := seen[t.Label]; ok {
			return ThumbnailCatalog{}, fmt.Errorf("avatars: duplicate thumbnail label %q", t.Label)
		}
		seen[t.Label] = struct{}{}
		labels = append(labels, t.Label)
		sides[t.Label] = t.Pixels
	}
	return ThumbnailCatalog{Labels: labels, Sides: sides}, nil
}

func defaultThumbnailPresets() []ThumbnailPreset {
	return []ThumbnailPreset{
		{Label: "64x64", Pixels: 64},
		{Label: "128x128", Pixels: 128},
		{Label: "256x256", Pixels: 256},
		{Label: "512x512", Pixels: 512},
		{Label: "100x100", Pixels: 100},
		{Label: "300x300", Pixels: 300},
	}
}

// Normalize fills derived defaults (upload cap, thumbnails, static dir).
func (cfg *App) Normalize() {
	if cfg.Avatars.MaxUploadBytes <= 0 {
		cfg.Avatars.MaxUploadBytes = 10 * 1024 * 1024
	}
	if len(cfg.Avatars.Thumbnails) == 0 {
		cfg.Avatars.Thumbnails = defaultThumbnailPresets()
	}
	if strings.TrimSpace(cfg.Avatars.ImageCacheControl) == "" {
		cfg.Avatars.ImageCacheControl = "max-age=86400"
	}
	if strings.TrimSpace(cfg.HTTP.StaticDir) == "" {
		cfg.HTTP.StaticDir = "web/static"
	}
}
