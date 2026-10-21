package delivery

import (
	"strings"
	"unicode"

	"github.com/stevedotmt/url2supernote/app/internal/article"
)

const (
	maxTitleRunes = 100
	maxSiteRunes  = 40
	extension     = ".epub"
)

// Filename names the file an Article is delivered as: "<Title> — <site>.epub".
func Filename(a article.Article) string {
	title := clean(a.Title, maxTitleRunes)
	site := clean(a.Site(), maxSiteRunes)
	switch {
	case title == "" && site == "":
		return "Article" + extension
	case title == "":
		return site + extension
	case site == "":
		return title + extension
	default:
		return title + " — " + site + extension
	}
}

func clean(s string, maxRunes int) string {
	mapped := strings.Map(func(r rune) rune {
		switch {
		case strings.ContainsRune(`/\:*?"<>|`, r), unicode.IsSpace(r):
			return ' '
		case unicode.IsControl(r), r == unicode.ReplacementChar:
			return -1
		default:
			return r
		}
	}, s)
	collapsed := strings.Join(strings.Fields(mapped), " ")
	runes := []rune(collapsed)
	if len(runes) > maxRunes {
		collapsed = string(runes[:maxRunes])
	}
	return strings.Trim(collapsed, " .")
}
