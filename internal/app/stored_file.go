package app

import "github.com/vrc/nimbus/internal/domain"

// StoredFile is an uploaded/imported file with an optional public delivery URL.
type StoredFile struct {
	domain.Node
	URL string `json:"url,omitempty"`
}
