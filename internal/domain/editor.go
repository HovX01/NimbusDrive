package domain

import "time"

// EditProject stores a persistent video editor project.
type EditProject struct {
	ID           string    `json:"id"`
	SourceNodeID string    `json:"source_node_id"`
	Name         string    `json:"name"`
	TimelineJSON string    `json:"timeline_json"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
