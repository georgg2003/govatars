package models

// AllObjectKeys returns original plus thumbnail keys for S3 cleanup.
func (a *Avatar) AllObjectKeys() []string {
	keys := []string{a.S3Key}
	if len(a.ThumbnailS3Keys) == 0 {
		return keys
	}
	for _, byFormat := range a.ThumbnailS3Keys {
		for _, k := range byFormat {
			if k != "" {
				keys = append(keys, k)
			}
		}
	}
	return keys
}
