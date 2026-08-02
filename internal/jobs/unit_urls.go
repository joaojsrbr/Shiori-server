package jobs

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/net/html"

	"github.com/joaojsr/shiori-server/internal/extraction"
)

var unitNumberPattern = regexp.MustCompile(`(?i)\b(?:cap(?:(?:i|\x{00ED})tulo)?|chapter|ep(?:is(?:o|\x{00F3})dio)?|episode)\.?\s*#?\s*([0-9]+(?:[.,][0-9]+)?)\b`)
var routeURLPattern = regexp.MustCompile(`(?i)https?://[a-z0-9._~:/?#\[\]@!$&()*+,;=%-]+|/[a-z0-9._~:/?#\[\]@!$&()*+,;=%-]+`)
var routeNumberPattern = regexp.MustCompile(`[0-9]+(?:[.,-][0-9]+)?`)

func appendDiscoveredUnitURLs(content string, units []extractedUnit, target extraction.TargetType) string {
	if len(units) == 0 {
		return content
	}
	label := "chapter"
	if extractedUnitField(target) == "episodes" {
		label = "episode"
	}
	var routes strings.Builder
	routes.WriteString("\n\n# Verified ")
	routes.WriteString(label)
	routes.WriteString(" URL mapping\n\n")
	routes.WriteString("Extraction requirements:\n")
	routes.WriteString("- For every extracted number listed below, copy its exact verified URL into the `url` field.\n")
	routes.WriteString("- Do not return a null or empty `url` when that number exists in this mapping.\n")
	routes.WriteString("- These are content URLs, not `next_page_url` pagination links.\n")
	routes.WriteString("- Never invent a URL for a number absent from this mapping.\n\n")
	for _, unit := range units {
		fmt.Fprintf(&routes, "- %s %s: %s\n", label, unit.Number, unit.URL)
	}
	return content + routes.String()
}

// enrichExtractedUnitURLs repairs results from client-rendered lists. A route
// is derived only when links, data attributes, handlers or embedded scripts
// prove a reusable template.
func enrichExtractedUnitURLs(result map[string]interface{}, document, baseURL string, target extraction.TargetType) {
	field := extractedUnitField(target)
	discovered := discoverUnitURLs(document, baseURL)
	if len(discovered) == 0 {
		return
	}
	items, _ := result[field].([]interface{})
	byNumber := make(map[string]string, len(discovered))
	for _, unit := range discovered {
		byNumber[normalizeUnitNumber(unit.Number)] = unit.URL
	}
	seen := make(map[string]bool)
	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		number := normalizeUnitNumber(scalarString(item["number"]))
		if number == "" {
			continue
		}
		seen[number] = true
		if scalarString(item["url"]) == "" && byNumber[number] != "" {
			item["url"] = byNumber[number]
		}
	}
	for _, unit := range discovered {
		number := normalizeUnitNumber(unit.Number)
		if number == "" || seen[number] {
			continue
		}
		items = append(items, map[string]interface{}{
			"number": unit.Number, "title": unit.Title, "url": unit.URL, "date": "",
		})
		seen[number] = true
	}
	result[field] = items
}

func extractedUnitContainsURL(result map[string]interface{}, target extraction.TargetType, candidate, baseURL string) bool {
	wanted := resolveUnitURL(baseURL, candidate)
	items, _ := result[extractedUnitField(target)].([]interface{})
	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if ok && resolveUnitURL(baseURL, scalarString(item["url"])) == wanted {
			return true
		}
	}
	return false
}

func extractedUnitField(target extraction.TargetType) string {
	if target == extraction.TargetAnime || target == extraction.TargetAnimeEpisodes {
		return "episodes"
	}
	return "chapters"
}

func discoverUnitURLs(document, baseURL string) []extractedUnit {
	doc, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return nil
	}
	explicit := make(map[string]extractedUnit)
	visibleNumbers := make(map[string]string)
	candidates := make([]string, 0)

	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			text := strings.TrimSpace(nodeText(node))
			number := chapterNumberFromText(text)
			interactive := node.Data == "a" || node.Data == "button"
			var directURL string
			for _, attr := range node.Attr {
				key := strings.ToLower(attr.Key)
				value := strings.TrimSpace(attr.Val)
				if key == "role" && strings.EqualFold(value, "button") {
					interactive = true
				}
				if isRouteAttribute(key) {
					matches := routeURLPattern.FindAllString(strings.ReplaceAll(value, `\/`, `/`), -1)
					if len(matches) == 0 && key != "onclick" && value != "" && !strings.HasPrefix(strings.ToLower(value), "javascript:") {
						matches = []string{value}
					}
					candidates = append(candidates, matches...)
					if directURL == "" && len(matches) > 0 {
						directURL = matches[0]
					}
				}
			}
			if interactive && number != "" {
				visibleNumbers[normalizeUnitNumber(number)] = number
			}
			if number != "" && directURL != "" {
				if absolute := resolveUnitURL(baseURL, directURL); absolute != "" {
					explicit[normalizeUnitNumber(number)] = extractedUnit{Number: number, Title: text, URL: absolute}
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)

	// Next.js, Nuxt and similar frameworks serialize route information in
	// script payloads. JSON-LD urlTemplate values are covered by this scan too.
	decodedDocument := strings.NewReplacer(`\/`, `/`, `\u002F`, "/", `\u002f`, "/").Replace(document)
	candidates = append(candidates, routeURLPattern.FindAllString(decodedDocument, -1)...)

	if template, ok := selectRouteTemplate(candidates, baseURL); ok {
		for normalized, number := range visibleNumbers {
			if _, exists := explicit[normalized]; !exists {
				explicit[normalized] = extractedUnit{
					Number: number,
					Title:  "CAP. " + number,
					URL:    template.prefix + url.PathEscape(number) + template.suffix,
				}
			}
		}
	}

	keys := make([]string, 0, len(explicit))
	for key := range explicit {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool { return numericUnitValue(keys[i]) > numericUnitValue(keys[j]) })
	units := make([]extractedUnit, 0, len(keys))
	for _, key := range keys {
		units = append(units, explicit[key])
	}
	return units
}

func isRouteAttribute(key string) bool {
	switch key {
	case "href", "data-href", "data-url", "data-link", "data-path", "data-route", "onclick":
		return true
	default:
		return false
	}
}

type unitRouteTemplate struct {
	prefix string
	suffix string
	score  int
	count  int
}

func selectRouteTemplate(rawCandidates []string, baseURL string) (unitRouteTemplate, bool) {
	base, err := url.Parse(baseURL)
	if err != nil || base.Hostname() == "" {
		return unitRouteTemplate{}, false
	}
	basePath := strings.TrimSuffix(base.Path, "/")
	templates := make(map[string]unitRouteTemplate)
	for _, raw := range rawCandidates {
		absolute := resolveUnitURL(baseURL, strings.Trim(raw, "\"'\\"))
		candidate, parseErr := url.Parse(absolute)
		if parseErr != nil || !strings.EqualFold(candidate.Hostname(), base.Hostname()) {
			continue
		}
		full := candidate.String()
		lower := strings.ToLower(full)
		baseDescendant := basePath != "" && strings.HasPrefix(candidate.Path, basePath+"/")
		hasUnitWord := strings.Contains(lower, "chapter") || strings.Contains(lower, "capitulo") || strings.Contains(lower, "episode") || strings.Contains(lower, "episodio")
		if !baseDescendant && !hasUnitWord {
			continue
		}
		matches := routeNumberPattern.FindAllStringIndex(full, -1)
		if len(matches) == 0 {
			continue
		}
		match := matches[len(matches)-1]
		if baseDescendant {
			baseAbsolute := strings.TrimSuffix(base.String(), "/")
			if match[0] < len(baseAbsolute) {
				continue
			}
		}
		key := full[:match[0]] + "{number}" + full[match[1]:]
		entry := templates[key]
		entry.prefix, entry.suffix = full[:match[0]], full[match[1]:]
		entry.count++
		if baseDescendant {
			entry.score = 3
		} else {
			entry.score = 2
		}
		templates[key] = entry
	}

	var best unitRouteTemplate
	found, tied := false, false
	for _, candidate := range templates {
		if !found || candidate.score > best.score || (candidate.score == best.score && candidate.count > best.count) {
			best, found, tied = candidate, true, false
		} else if candidate.score == best.score && candidate.count == best.count && (candidate.prefix != best.prefix || candidate.suffix != best.suffix) {
			tied = true
		}
	}
	return best, found && !tied
}

func chapterNumberFromText(text string) string {
	match := unitNumberPattern.FindStringSubmatch(text)
	if len(match) < 2 {
		return ""
	}
	return strings.ReplaceAll(match[1], ",", ".")
}

func normalizeUnitNumber(number string) string {
	number = strings.TrimSpace(strings.ReplaceAll(number, ",", "."))
	trimmed := strings.TrimLeft(number, "0")
	if trimmed == "" || strings.HasPrefix(trimmed, ".") {
		return "0" + trimmed
	}
	return trimmed
}

func numericUnitValue(number string) float64 {
	var value float64
	_, _ = fmt.Sscanf(strings.ReplaceAll(number, "-", "."), "%f", &value)
	return value
}

func resolveUnitURL(baseURL, raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	ref, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	if base, baseErr := url.Parse(baseURL); baseErr == nil {
		ref = base.ResolveReference(ref)
	}
	if ref.Scheme != "http" && ref.Scheme != "https" {
		return ""
	}
	return ref.String()
}
