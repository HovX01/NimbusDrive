package app

import (
	"fmt"
	"strconv"
	"strings"
)

type Caption struct {
	Index     int     `json:"index"`
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
	Text      string  `json:"text"`
}

func ParseSRT(data string) ([]Caption, error) {
	data = strings.ReplaceAll(data, "\r\n", "\n")
	data = strings.ReplaceAll(data, "\r", "\n")
	blocks := strings.Split(data, "\n\n")
	captions := make([]Caption, 0, len(blocks))
	for _, block := range blocks {
		lines := compactLines(strings.Split(strings.TrimSpace(block), "\n"))
		if len(lines) == 0 {
			continue
		}
		index := len(captions) + 1
		timeLineAt := 0
		if n, err := strconv.Atoi(strings.TrimSpace(lines[0])); err == nil {
			index = n
			timeLineAt = 1
		}
		if len(lines) <= timeLineAt {
			continue
		}
		parts := strings.Split(lines[timeLineAt], "-->")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid SRT timestamp: %s", lines[timeLineAt])
		}
		start, err := parseSRTTime(parts[0])
		if err != nil {
			return nil, err
		}
		end, err := parseSRTTime(parts[1])
		if err != nil {
			return nil, err
		}
		if end <= start {
			return nil, fmt.Errorf("invalid SRT range: end before start")
		}
		text := strings.TrimSpace(strings.Join(lines[timeLineAt+1:], "\n"))
		if text == "" {
			continue
		}
		captions = append(captions, Caption{Index: index, StartTime: start, EndTime: end, Text: text})
	}
	return captions, nil
}

func GenerateSRT(captions []Caption) string {
	var b strings.Builder
	for i, caption := range captions {
		index := caption.Index
		if index <= 0 {
			index = i + 1
		}
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(strconv.Itoa(index))
		b.WriteString("\n")
		b.WriteString(formatSRTTime(caption.StartTime))
		b.WriteString(" --> ")
		b.WriteString(formatSRTTime(caption.EndTime))
		b.WriteString("\n")
		b.WriteString(strings.TrimSpace(caption.Text))
		b.WriteString("\n")
	}
	return b.String()
}

func compactLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func parseSRTTime(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.Split(s, " ")[0]
	mainParts := strings.Split(s, ",")
	if len(mainParts) != 2 {
		return 0, fmt.Errorf("invalid SRT time: %s", s)
	}
	hms := strings.Split(mainParts[0], ":")
	if len(hms) != 3 {
		return 0, fmt.Errorf("invalid SRT time: %s", s)
	}
	h, err := strconv.Atoi(hms[0])
	if err != nil {
		return 0, err
	}
	m, err := strconv.Atoi(hms[1])
	if err != nil {
		return 0, err
	}
	sec, err := strconv.Atoi(hms[2])
	if err != nil {
		return 0, err
	}
	ms, err := strconv.Atoi(padRight(mainParts[1], 3))
	if err != nil {
		return 0, err
	}
	return float64(h*3600+m*60+sec) + float64(ms)/1000, nil
}

func formatSRTTime(v float64) string {
	if v < 0 {
		v = 0
	}
	totalMS := int64(v*1000 + 0.5)
	ms := totalMS % 1000
	totalSec := totalMS / 1000
	sec := totalSec % 60
	totalMin := totalSec / 60
	min := totalMin % 60
	hour := totalMin / 60
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hour, min, sec, ms)
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s[:n]
	}
	return s + strings.Repeat("0", n-len(s))
}
