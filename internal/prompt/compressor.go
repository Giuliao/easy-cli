package prompt

import (
	"fmt"
	"strings"
)

// Compress makes a Markdown prompt structurally compact without rewriting its
// sentences or inferring new requirements.
func Compress(markdown string) (string, error) {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	name, bodyStart := frontMatterName(lines)

	prefix := make([]string, 0, bodyStart)
	if bodyStart > 0 {
		for _, line := range lines[:bodyStart] {
			prefix = append(prefix, strings.TrimRight(line, " \t"))
		}
	}

	body := compactBody(lines[bodyStart:], name)
	if len(prefix) == 0 {
		return strings.Join(body, "\n") + "\n", nil
	}
	if len(body) == 0 {
		return strings.Join(prefix, "\n") + "\n", nil
	}
	return strings.Join(prefix, "\n") + "\n" + strings.Join(body, "\n") + "\n", nil
}

func frontMatterName(lines []string) (string, int) {
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", 0
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "---" {
			continue
		}
		name := ""
		for _, line := range lines[1:i] {
			key, value, ok := strings.Cut(line, ":")
			if ok && strings.TrimSpace(key) == "name" {
				name = strings.TrimSpace(value)
				break
			}
		}
		return name, i + 1
	}
	return "", 0
}

func compactBody(lines []string, name string) []string {
	result := make([]string, 0, len(lines))
	inFence := false
	blankPending := false
	for i := 0; i < len(lines); {
		line := strings.TrimRight(lines[i], " \t")
		trimmed := strings.TrimSpace(line)

		if isFence(trimmed) {
			if blankPending && len(result) > 0 {
				result = append(result, "")
				blankPending = false
			}
			result = append(result, line)
			inFence = !inFence
			i++
			continue
		}

		if inFence {
			result = append(result, line)
			i++
			continue
		}

		if trimmed == "" {
			blankPending = len(result) > 0
			i++
			continue
		}
		if name != "" && trimmed == "# "+name {
			i++
			continue
		}

		if headers, next, ok := tableAt(lines, i); ok {
			if blankPending && len(result) > 0 {
				result = append(result, "")
				blankPending = false
			}
			for _, row := range lines[i+2 : next] {
				cells := splitTableRow(row)
				parts := make([]string, 0, len(cells))
				for column, value := range cells {
					if column >= len(headers) || headers[column] == "" || value == "" {
						continue
					}
					parts = append(parts, fmt.Sprintf("%s: %s", headers[column], value))
				}
				if len(parts) > 0 {
					result = append(result, "- "+strings.Join(parts, "; "))
				}
			}
			i = next
			continue
		}

		if blankPending && len(result) > 0 {
			result = append(result, "")
			blankPending = false
		}
		if len(result) == 0 || result[len(result)-1] != line {
			result = append(result, line)
		}
		i++
	}

	for len(result) > 0 && result[len(result)-1] == "" {
		result = result[:len(result)-1]
	}
	return result
}

func tableAt(lines []string, start int) ([]string, int, bool) {
	if start+1 >= len(lines) || !strings.Contains(lines[start], "|") || !isTableDelimiter(lines[start+1]) {
		return nil, 0, false
	}
	headers := splitTableRow(lines[start])
	if len(headers) == 0 {
		return nil, 0, false
	}
	next := start + 2
	for next < len(lines) && strings.Contains(lines[next], "|") && strings.TrimSpace(lines[next]) != "" {
		next++
	}
	return headers, next, true
}

func isTableDelimiter(line string) bool {
	cells := splitTableRow(line)
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		cell = strings.Trim(cell, " :")
		if cell == "" {
			return false
		}
		for _, char := range cell {
			if char != '-' {
				return false
			}
		}
	}
	return true
}

func splitTableRow(line string) []string {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "|")
	trimmed = strings.TrimSuffix(trimmed, "|")
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func isFence(line string) bool {
	return strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~")
}
