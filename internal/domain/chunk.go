package domain

import "fmt"

// SplitChunks divides content into fixed-size parts. Last part may be shorter.
// Empty input yields one empty part so callers always have a stable part map.
func SplitChunks(data []byte, chunkSize int) [][]byte {
	if chunkSize <= 0 {
		panic("chunkSize must be positive")
	}
	if len(data) == 0 {
		return [][]byte{data}
	}
	n := (len(data) + chunkSize - 1) / chunkSize
	out := make([][]byte, 0, n)
	for i := 0; i < len(data); i += chunkSize {
		end := i + chunkSize
		if end > len(data) {
			end = len(data)
		}
		out = append(out, data[i:end])
	}
	return out
}

// PartRange maps an HTTP byte range onto part indices and offsets.
type PartRange struct {
	PartNo     int
	Skip       int64 // bytes to skip inside this part
	Take       int64 // bytes to read from this part
}

// MapRangeToParts converts [start, endInclusive] into per-part reads.
func MapRangeToParts(partSizes []int64, start, endInclusive int64) ([]PartRange, error) {
	if start < 0 || endInclusive < start {
		return nil, fmt.Errorf("%w: invalid range", ErrValidation)
	}
	var total int64
	for _, s := range partSizes {
		total += s
	}
	if endInclusive >= total {
		return nil, fmt.Errorf("%w: range past EOF", ErrValidation)
	}

	var out []PartRange
	var offset int64
	remainingStart := start
	remainingEnd := endInclusive

	for i, size := range partSizes {
		partStart := offset
		partEnd := offset + size - 1
		offset += size

		if remainingEnd < partStart || remainingStart > partEnd {
			continue
		}

		skip := int64(0)
		if remainingStart > partStart {
			skip = remainingStart - partStart
		}
		takeEnd := partEnd
		if remainingEnd < partEnd {
			takeEnd = remainingEnd
		}
		take := takeEnd - (partStart + skip) + 1
		out = append(out, PartRange{PartNo: i, Skip: skip, Take: take})
	}
	return out, nil
}
